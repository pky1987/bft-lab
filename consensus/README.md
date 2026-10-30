# `consensus/`: BFT consensus

> **One sentence:** the code that lets validators agree on one block per height, even when up to
> 1/3 of the voting power lies, crashes or is cut off.

This package grows over Lessons 12–16 and 22. Each lesson adds a section here.

---

## Lesson 12: quorum math + `VoteSet`

### 1. Why it exists

In Lesson 11 a validator could **sign** a vote. But one vote decides nothing. Consensus needs to know:
*"have enough validators voted for the same block?"* "Enough" has to be a number that stays **safe**
even when some validators are Byzantine (they lie, double-vote or stay silent).

```
            gossip (many copies, any order, some forged)
 votes ───────────────────────────────────────────────┐
                                                      ▼
                                   ┌──────────────────────────────┐
                                   │ VoteSet (height 5, round 0,  │
                                   │          type Prevote)       │
                                   │  1. right H/R/type?          │
                                   │  2. known validator?         │
                                   │  3. signature valid?         │
                                   │  4. duplicate / double vote? │
                                   │  5. add power to its block   │
                                   └──────────────┬───────────────┘
                                                  │ TwoThirdsMajority()
                                                  ▼
                                consensus state machine (Lesson 14):
                                "2/3+ prevoted B → precommit B"
```

Without it there's no consensus. Every later lesson (happy path, timeouts, locking, evidence) asks a
VoteSet the same question: *is there a 2/3+ majority, and for what?*

### 2. Theory

#### 2.1 The fault model: n ≥ 3f + 1

- **f** = how much voting power can be Byzantine (worst case: it does anything, including lying).
- In an asynchronous network you **cannot wait for everyone**, because f validators might never answer.
  So you can only wait for **n − f** votes.
- Out of those n − f votes, up to **f** may come from liars. The honest votes must still **outnumber**
  the liars: (n − f) − f > f → **n > 3f** → **n ≥ 3f + 1**.

| Validators n | Max faulty f = ⌊(n−1)/3⌋ | Quorum (> 2/3) |
|---|---|---|
| 1 | 0 | 1 |
| 3 | 0 | 3 |
| **4** | **1** | **3** |
| 7 | 2 | 5 |
| 10 | 3 | 7 |
| 100 | 33 | 67 |

That's why our test network has **4 validators**: it's the smallest set that survives one traitor.

#### 2.2 Why "more than 2/3" and not "half"

Take any two quorums Q1 and Q2, each with more than 2/3 of the power. How much must they share?

```
 |Q1| + |Q2| − n  =  overlap
 > 2n/3 + 2n/3 − n  =  n/3   >   f
```

```
 n = 4, f = 1, quorum = 3
 ┌─────────── Q1 (voted A) ───────────┐
 [ v1 ]  [ v2 ]  [ v3 ]  [ v4 ]
         └─────────── Q2 (voted B) ──────────┘
         overlap = {v2, v3}: 2 validators > f = 1
         → at least ONE of them is honest
         → an honest validator would have to vote A and B: impossible
         → A and B can't both get a quorum  ✅ SAFETY
```

With "more than 1/2" the overlap is just more than 0. One Byzantine validator in the overlap could vote for
both A and B, and the chain would **fork** (two different blocks committed at the same height).

**Interview line:** *"Any two 2/3+ quorums intersect in more than 1/3 of the power. Since at most 1/3 is
faulty, the intersection contains an honest validator, and honest validators never vote twice. So two
conflicting blocks can never both reach a quorum."*

#### 2.3 Counting power, not heads

Validators have different **voting power** (stake). "2/3" always means 2/3 of the **total power**, never
2/3 of the number of validators. With powers (10, 20, 30), the single validator with 30 can't reach a
quorum alone (needs 41), but 20 + 30 = 50 can.

#### 2.4 The integer math (and the overflow trap)

We want the **smallest** power p with 3p > 2·total.

- ❌ Naive: `2*total/3 + 1`. When total is huge, `2*total` **overflows** uint64 and wraps around.
- ✅ Ours: `total − ⌈total/3⌉ + 1`. Nothing is ever multiplied, so it can't overflow.

```
 total = 4:  ⌈4/3⌉ = 2   →  4 − 2 + 1 = 3   (3·3 = 9 > 8 ✅, 3·2 = 6 > 8 ❌)
 total = 100: ⌈100/3⌉ = 34 → 100 − 34 + 1 = 67
```

Measured (BREAK 2): the naive formula gives `Quorum(MaxUint64) = 6148914691236517205`, which is **1/3**
of the total, not 2/3. An attacker with 1/3 of the stake could then commit blocks alone.

#### 2.5 Nil votes

A vote with a **zero** BlockHash means *"I saw no acceptable block in time."* VoteSet counts nil like any
other "block". 2/3+ nil prevotes is a real outcome: it tells everyone to move to the next round (Lesson 15).

#### 2.6 Duplicates vs equivocation

| Situation | What happened | VoteSet does |
|---|---|---|
| Same validator, **same** vote again | normal gossip (peers forward votes many times) | ignore: `(false, nil)` |
| Same validator, **different** block, same H/R/type | **equivocation** (double-signing), a Byzantine act | reject: `ErrConflictingVote` |

If a duplicate were counted twice, one validator could reach a quorum by itself (BREAK 5). If the second
vote of a double-signer were counted, it would back **both** blocks, and §2.2's safety proof breaks
(BREAK 4). In Lesson 22 the two conflicting votes become **evidence** that gets the validator slashed.

#### 2.7 Two different questions

| Method | Question | Used for |
|---|---|---|
| `TwoThirdsMajority()` | has 2/3+ voted for the **same** block (or nil)? | precommit / commit decisions |
| `HasTwoThirdsAny()` | has 2/3+ voted **at all** (split votes allowed)? | starting timeouts (Lesson 15) |

A 2–1–1 split across A, B and nil in a 4-validator set has 2/3 *any*, but no *majority*.

### 3. Types

| Type | Meaning |
|---|---|
| `VoteSet` | votes of ONE (height, round, type), with power counted per block hash |
| `votes map[crypto.Address]types.Vote` | one vote per validator (used to spot duplicates and double votes) |
| `byBlock map[crypto.Hash]uint64` | power behind each block hash (the zero hash = nil) |
| `sum uint64` | power of everyone who voted, for any block |

### 4. Functions

| Function | What it does |
|---|---|
| `Quorum(total)` | smallest power that is > 2/3 of total, overflow-free |
| `HasQuorum(power, total)` | `power >= Quorum(total)` |
| `MaxFaulty(total)` | largest f with 3f < total, i.e. `(total−1)/3` |
| `NewVoteSet(chainID, h, r, type, vals)` | empty set for one step; maps made with `make` |
| `AddVote(v) (bool, error)` | 5 checks in order (H/R/type → known → signature → dup/double → count) |
| `TwoThirdsMajority() (Hash, bool)` | the block with 2/3+, if any |
| `HasTwoThirdsAny() bool` | 2/3+ voted for anything |
| `Size() int` | number of distinct validators that voted |

**Why the check order matters in `AddVote`:** cheap checks (compare numbers) come first, and the expensive
one (signature verify, about 37 µs from Lesson 9) comes after. That way a flood of wrong-height junk can't
make us burn CPU on signature checks. And every check happens **before** any write (the Lesson 1 rule:
check first, then write), so a rejected vote changes nothing.

### 5. Invariants and the tests that prove them

| # | Property | Test |
|---|---|---|
| 1 | Quorum has known values (1→1, 4→3, 100→67) | `TestQuorumTable` |
| 2 | Quorum is > 2/3 **and** the smallest such number (totals 1..10,000) | `TestQuorumIsSmallestMoreThanTwoThirds` |
| 3 | Two quorums always overlap by more than f | `TestQuorumsIntersectInAnHonestValidator` |
| 4 | No overflow at `MaxUint64` | `TestQuorumNoOverflow` |
| 5 | f = ⌊(n−1)/3⌋ | `TestMaxFaulty` |
| 6 | 2 of 4 is not a quorum, 3 of 4 is | `TestVoteSetReachesQuorumAtThree` |
| 7 | 2–1 split: no majority, but 2/3 any | `TestVoteSetSplitHasNoMajority` |
| 8 | 3 nil votes = nil majority | `TestVoteSetNilMajority` |
| 9 | a repeated vote counts once | `TestVoteSetDuplicateCountsOnce` |
| 10 | wrong H/R/type, outsider, bad sig → rejected, nothing stored | `TestVoteSetRejects` |
| 11 | a double vote is rejected and never counted | `TestVoteSetEquivocation` |

### 6. Break experiments (verified on the prototype, 2026-10-29)

| # | Mutation | Caught by |
|---|---|---|
| 1 | `total - third + 1` → `total - third` (≥ 2/3 instead of > 2/3) | Table, Smallest, Intersect (`total=2: overlap 0`) |
| 2 | naive `2*total/3 + 1` | NoOverflow: `Quorum(max) = 6148914691236517205` |
| 3 | MaxFaulty `(total-1)/3` → `total/3` | MaxFaulty: `MaxFaulty(3) = 1, want 0` |
| 4 | count the equivocating vote's power | Equivocation |
| 5 | count duplicates | DuplicateCountsOnce |
| 6 | skip the signature check | Rejects/bad_signature |
| 7 | skip the round check | Rejects/wrong_round |

Prototype result: 11 tests, 100% coverage, `-race` clean, golangci-lint 0 issues.

### 7. How it maps to CometBFT

| bft-lab | CometBFT |
|---|---|
| `VoteSet` | `types/vote_set.go` `VoteSet` |
| `AddVote` | `VoteSet.AddVote`, returning `ErrVoteConflictingVotes` for double signs |
| `TwoThirdsMajority` | `VoteSet.TwoThirdsMajority() (BlockID, bool)` |
| `HasTwoThirdsAny` | `VoteSet.HasTwoThirdsAny()` |
| `Quorum` | `ValidatorSet.TotalVotingPower()*2/3 + 1`. CometBFT caps total power at `MaxTotalVotingPower = MaxInt64/8`, so it never overflows; we chose the overflow-free formula instead |
| `byBlock` | `votesByBlock map[string]*blockVotes` |

CometBFT also keeps *peer-claimed* majorities (`peerMaj23s`) and a bit array per block. We skip those until
gossip optimisation matters.

### 8. Go lessons

- **Maps with struct keys:** `crypto.Hash` and `crypto.Address` are **arrays**, so they're comparable and
  can be map keys. A `[]byte` can't be a map key. (This is why Lesson 8 made `Hash` a `[32]byte`.)
- **A nil map panics on write:** `var m map[K]V; m[k] = v` panics. That's why `NewVoteSet` uses `make`.
- **`m[k] += x` on a missing key** starts from the zero value (0), so no "if exists" check is needed.
- **Comma-ok again:** `old, seen := s.votes[addr]`.
- **Map iteration order is random** (Lesson 1). `TwoThirdsMajority` loops over a map, which is safe here
  only because **at most one** block can have 2/3+ (§2.2). Ask yourself: why could it go wrong with a
  1/2 threshold?
- **Overflow-free integer math:** never multiply untrusted totals; rearrange the formula.

### 9. Interview questions

1. Why n ≥ 3f + 1? Derive it.
2. Why > 2/3 and not ≥ 2/3? Give the 3-validator counterexample.
3. Prove that two 2/3+ quorums intersect in an honest validator.
4. How do you compute 2/3 of a uint64 without overflow?
5. What's the difference between a duplicate vote and equivocation? What should happen to each?
6. Why is the signature check after the height/round/type checks?
7. What does a nil majority mean, and what does the node do next?
8. Why does CometBFT cap total voting power at MaxInt64/8?

### 10. In my own words

*(Prakash writes this)*
