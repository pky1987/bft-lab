# `types`: what validators agree on

> **One sentence:** `types` defines the three things consensus is about: **who** votes (the validator
> set), **what** they vote on (blocks), and **how** a vote is proven (signed votes).

This is where everything from `crypto` comes together:

```
crypto.Hash, HashFields ──► Header.Hash()        (block identity, hash chain via PrevHash)
crypto.MerkleRoot ────────► Header.TxRoot        (commits to every transaction)
                            ValidatorSet.Hash()  (commits to every validator + power)
crypto.Sign / Verify ─────► Vote.Sign / Verify   (who voted, and for what)
crypto.Address ───────────► Validator.Address, Vote.Validator, Header.Proposer
```

---

## 1. Why this package exists

Consensus (topics 12–16) needs to answer questions like:
- *"Is this vote from a real validator?"* → `ValidatorSet.GetByAddress` + `Vote.Verify`
- *"Do the votes for block H add up to more than 2/3 of the voting power?"* → `Validator.Power`, `ValidatorSet.TotalPower`
- *"Is this block well-formed, and does it link to the previous one?"* → `Block.ValidateBasic`, `Header.PrevHash`

Without shared, well-defined types, every node could interpret the same data differently, and that's
exactly how chains split.

---

## 2. Theory: validators and voting power

A **validator** is a node allowed to vote. Each one has:

| Field | Meaning |
|---|---|
| `PubKey` | used to verify its signatures |
| `Address` | `SHA-256(PubKey)[:20]`, its short ID. **Always derived** by `NewValidator`, so address and key can never disagree |
| `Power` | how much its vote **counts**. In Cosmos this comes from **staked tokens**: more stake, more power |

**Why voting power instead of "one validator, one vote"?** In proof-of-stake, an attacker could create
thousands of fake validators for free (a **Sybil attack**). Weighting votes by stake means attacking the chain
requires owning a large share of the tokens, which is expensive to get and loses value if you attack.

### The 2/3 rule (preview of topic 12)

Tendermint needs votes worth **more than 2/3 of the total power** to commit a block. With 4 validators of
equal power 10 (total 40), that's more than 26.67, so **3 validators** (30). The validator set's
`TotalPower()` is the number every node divides by.

### Determinism: the set must be identical on every node

Validators may learn about each other in **different orders** (network timing). If the set kept the input
order, two honest nodes could build sets with different `Hash()` values, disagree on `ValidatorsHash` in the
header, and the chain would split.

**Fix:** `NewValidatorSet` **sorts by address**. Same validators in any order → identical set → identical hash.
It's the same rule as "Go randomises map order" from `ledger`: **never let order depend on chance**.

**Verified mutation:** disabling the sort makes `TestValidatorSetSortedDeterministic` fail with *"order-dependent"*.

### Validation rules

| Rule | Why | Error |
|---|---|---|
| at least 1 validator | nobody could vote, and consensus can't start | `ErrEmptyValidatorSet` |
| power > 0 | a zero-power validator adds nothing and confuses the quorum math | `ErrZeroPower` |
| no duplicate addresses | a validator listed twice would get **double** voting power | `ErrDuplicateValidator` |
| total power doesn't overflow `uint64` | an overflowing total wraps to a tiny number, which breaks the 2/3 math (Lesson 1's overflow check, applied to power) | `ErrPowerOverflow` |

### Encapsulation: why `Validators()` returns a copy

`ValidatorSet` stores its validators in an **unexported** slice. If `Validators()` returned that slice
directly, a caller could do `vs[0].Power = 999` and **silently change the real set** (slices share memory,
as in Lesson 10's `q := p`). So it returns `slices.Clone(...)`.

**Verified mutation:** returning the internal slice makes `TestValidatorSetLookupAndHash` fail with *"copy leaked"*.

---

## 3. Theory: blocks and headers

```
┌──────────────────────── Block (height 2) ────────────────────────┐
│ Header                                                            │
│   ChainID:        "bft-lab-1"                                     │
│   Height:         2                                               │
│   PrevHash:       hash of block 1's header  ◄── hash chain        │
│   TxRoot:         MerkleRoot(Txs)           ◄── Merkle tree        │
│   ValidatorsHash: ValidatorSet.Hash()       ◄── who's in charge   │
│   Proposer:       address of the proposer                         │
│ Txs: [ tx1, tx2, ... ]                                            │
└───────────────────────────────────────────────────────────────────┘
Block hash = Header.Hash() = HashFields(every header field)
```

| Concept | Meaning |
|---|---|
| **the header is the block's identity** | votes are for `Header.Hash()`, 32 bytes, not for the whole block |
| `TxRoot` | links the small header to the (possibly huge) transaction list. Changing any tx breaks `TxRoot`, so it breaks the hash |
| `PrevHash` | Lesson 8's hash chain, now with real headers |
| `ChainID` | stops a block from one chain being presented as a block of another |
| **every field must be in the hash** | a field left out of `Hash()` could be changed **without changing the block's identity**, so an attacker could swap it. `TestHeaderHashEveryField` changes each field one at a time and checks the hash changes |

### `ValidateBasic`: stateless checks

"Basic" means *checks that need only this block*: chain ID set, height ≥ 1, `TxRoot` matching the
transactions. Checks that need **other** data (is `PrevHash` really the previous block? is the proposer
correct for this round?) come later, in consensus. Cosmos SDK messages have a `ValidateBasic` with the same meaning.

---

## 4. Theory: votes, and how to make them un-replayable

```go
type Vote struct {
    Type      VoteType       // Prevote or Precommit
    Height    uint64
    Round     uint64
    BlockHash crypto.Hash    // the block voted for, or ZERO = "nil vote"
    Validator crypto.Address
    Signature crypto.Signature
}
```

Lesson 9 §9.4 warned: **a signature alone doesn't stop replay**. A vote fixes this by signing **all of its context**:

```
SignBytes = HashFields( "bft-lab/vote", chainID, type, height, round, blockHash )
              domain tag   which chain   which step  when  which round  what
```

| Attack | Stopped because the signed bytes contain… | Verified by |
|---|---|---|
| replay a vote on **another chain** | `chainID` | `TestVoteSignVerify` "cross-chain replay" |
| replay a round-0 vote in **round 1** | `round` | "round replay" |
| reuse a **prevote** as a **precommit** | `type` | "type reuse" |
| attach the signature to **another block** | `blockHash` | "block swap" |
| claim another validator's vote | `Verify` checks `pub.Address() == v.Validator` | "wrong key" |

**Verified mutation:** removing `chainID` from `SignBytes` makes the test fail with *"cross-chain replay"*.

### Nil votes

A validator that saw **no acceptable block** in time (the proposer was offline, or proposed something invalid)
still votes, **for nothing**: `BlockHash` = zero hash. Nil votes are how a round fails cleanly and
consensus moves to the next round (topic 15). `IsNil()` checks for this.

### Why `Sign` has a pointer receiver

```go
func (v *Vote) Sign(chainID string, key crypto.PrivateKey)   // *Vote: modifies the vote
func (v Vote)  Verify(chainID string, pub crypto.PublicKey) bool  // Vote: only reads
```
`Sign` **writes** `v.Signature`, so it needs the real vote (pointer). With a value receiver, it would sign a
**copy**, and the signature would be thrown away: a silent bug.

---

## 5. Go concepts in this package

| Concept | Where | Meaning |
|---|---|---|
| **importing your own package** | `"github.com/pky1987/bft-lab/crypto"` | `types` builds on `crypto` |
| `slices.Clone`, `slices.SortFunc` | `NewValidatorSet` | standard-library helpers: copy a slice, sort with a comparison function |
| `bytes.Compare(a, b)` | the sort function | returns −1, 0 or +1: byte-by-byte ordering of addresses |
| **comparison function** `func(a, b T) int` | `SortFunc` | negative = a first, positive = b first |
| **constructor returning a pointer + error** | `NewValidatorSet` → `(*ValidatorSet, error)` | invalid sets can't be created at all |
| **named type over `byte`** + `const` | `type VoteType byte`, `Prevote = 1` | an "enum": a small set of named values |
| **pointer vs value receivers** | `Sign` vs `Verify` | modify → pointer. Read only → value |
| **function values in a table** | `TestHeaderHashEveryField` | each test row holds a function that changes one field |

---

## 6. Properties and the tests that will prove them

| Property | Test |
|---|---|
| same validators in any order → same hash. Stored sorted. Total and size correct | `TestValidatorSetSortedDeterministic` |
| empty / zero power / duplicate / overflow → the right error | `TestValidatorSetErrors` (table-driven) |
| lookup finds real validators and not unknown ones. Power is part of the hash. `Validators()` is a copy | `TestValidatorSetLookupAndHash` |
| a valid block passes. A tampered tx, height 0 or empty chain ID fails | `TestBlockValidateBasic` |
| changing **any** header field changes the hash | `TestHeaderHashEveryField` |
| block 2's `PrevHash` is block 1's hash | `TestBlockChainLink` |
| valid vote verifies. Cross-chain / round / type / block replay and wrong key are rejected. Nil votes work | `TestVoteSignVerify` |

---

## 7. How this maps to CometBFT

| Here | CometBFT |
|---|---|
| `Validator{Address, PubKey, Power}` | `types.Validator{Address, PubKey, VotingPower, ProposerPriority}` |
| `ValidatorSet`, sorted, with total power | `types.ValidatorSet` (CometBFT sorts by voting power, then address) |
| `Header` fields | `types.Header`: `ChainID`, `Height`, `LastBlockID`, `DataHash`, `ValidatorsHash`, `ProposerAddress`, … |
| `Vote{Type, Height, Round, BlockHash, Validator, Signature}` | `types.Vote{Type, Height, Round, BlockID, ValidatorAddress, Signature, …}` |
| `SignBytes(chainID)` | `VoteSignBytes(chainID, vote)`: canonical, chain-id-bound bytes |
| `ValidateBasic` | `ValidateBasic` on blocks and votes |

---

## 8. Interview questions

1. Why is voting weighted by stake instead of one-validator-one-vote?
2. Why must a validator set be sorted deterministically?
3. What would happen if a header field were left out of `Header.Hash()`?
4. Which fields must a vote sign to prevent replay, and why each one?
5. What is a nil vote, and when does a validator cast one?
6. Why does `Vote.Sign` need a pointer receiver?

---

## 9. In my own words (Prakash, fill this in)

- Voting power is:
- The validator set is sorted because:
- A vote signs its height, round and chain ID because:
- A nil vote means:
