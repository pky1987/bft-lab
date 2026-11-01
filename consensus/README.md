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

---

## Lesson 13: proposer selection (weighted round-robin)

### 1. Why it exists

Each round, exactly one validator proposes a block. Every node must work out **who** on its own, with no
messages, or they would need consensus just to run consensus. A validator with more stake should also
propose proportionally more often, and a small validator must never be locked out.

```
 ValidatorSet ─► ProposerSelector.Next() ─► Validator
                                               │
              Lesson 14: "am I the proposer this round?" yes → build and send a block
                                                         no  → wait for that validator's block
```

### 2. Theory

#### 2.1 Why the simple options fail

| Option | Problem |
|---|---|
| `height % n` | ignores stake: power 1 and power 1000 propose equally often |
| Random, each node separately | nodes disagree about the proposer |
| Hash of the previous block | whoever builds that block can grind it to pick the next proposer |

#### 2.2 The algorithm

Think of a queue where everyone earns credit at the rate of their stake, and taking a turn costs the whole pot:

1. Everyone adds their **power** to their priority.
2. The **highest priority** proposes. On a tie, the lowest address wins.
3. The winner subtracts the **total power**.

Measured with powers A=1, B=2, C=3 (total 6):

| Step | Proposer | Priorities after (A, B, C) |
|---|---|---|
| 1 | C | 1, 2, −3 |
| 2 | B | 2, −2, 0 |
| 3 | A | −3, 0, 3 |
| 4 | C | −2, 2, 0 |
| 5 | B | −1, −2, 3 |
| 6 | C | 0, 0, 0 |

In one cycle of 6 steps: C 3 times, B 2, A 1, exactly their power, and the state is back to all zeros.

#### 2.3 Why the sum stays zero

Each step adds the total (everyone's power) and subtracts the total (the winner pays). So priorities always
sum to 0. A high priority means someone else is below zero, which is why nobody can run away forever.

#### 2.4 Determinism

Everything here is a pure function of the validator set: `ValidatorSet` is sorted, the tie-break is fixed, and
there's no clock, map iteration or randomness. **A slice, not a map, holds the priorities**, because Go randomises
map order (Lesson 1) and different nodes could then break ties differently. Nodes that disagree on the
proposer reject each other's blocks.

#### 2.5 The int64 limit

Priorities go negative, so they need a **signed** type, while powers are uint64. `int64(v.Power)` of a
huge value would wrap to a negative number, so `NewProposerSelector` rejects sets above
`MaxTotalPower = MaxInt64/8`, the same limit CometBFT uses.

#### 2.6 What we skip on purpose

CometBFT also rescales and re-centres priorities because its validator set **changes** over time. Our set
is fixed in P1, so we don't need that yet. You'll meet validator-set changes in P2.

The proposer is also **public in advance**. An attacker can target the next proposer with a DoS, and the
round timeout from Lesson 15 limits the damage: if the proposer is silent, the next round picks another.

### 3. Types and functions

| Name | Meaning |
|---|---|
| `ProposerSelector` | `vals` (sorted), `priority` (same indexes), `total` |
| `NewProposerSelector(set)` | starts all priorities at 0; `ErrPowerTooLarge` above the cap |
| `Next()` | runs one step and returns the proposer; 0 allocations |

### 4. Invariants and tests

| # | Property | Test |
|---|---|---|
| 1 | equal power gives plain round-robin in address order | `TestProposerEqualPowerRoundRobin` |
| 2 | over N cycles, each is chosen exactly N × power times | `TestProposerFairShare` |
| 3 | two selectors always agree | `TestProposerDeterministic` |
| 4 | priorities sum to 0 and stay within ±total | `TestProposerPrioritySumsToZero` |
| 5 | power 1 next to power 1000 still proposes each cycle | `TestProposerNoStarvation` |
| 6 | sets above the cap are rejected; the cap itself is allowed | `TestProposerRejectsHugePower` |

### 5. Break experiments (verified on the prototype, 2026-10-31)

| # | Mutation | Caught by |
|---|---|---|
| 1 | winner doesn't pay | RoundRobin, FairShare, SumsToZero, NoStarvation |
| 2 | add 1 instead of power | FairShare (`power 1 chosen 400 times, want 100`), SumsToZero |
| 3 | `>` → `>=` in `Next` (tie goes to the highest address) | RoundRobin |
| 4 | remove the cap check | RejectsHugePower |
| 5 | `>` → `>=` in the cap check | RejectsHugePower (`limit itself rejected`) |

Prototype result: 6 tests, 100% coverage, `-race` clean, lint 0. `Next` with 100 validators: about 125 ns/op,
0 allocs/op (one rough run on this laptop).

### 6. How it maps to CometBFT

| bft-lab | CometBFT |
|---|---|
| `priority[i]` | `Validator.ProposerPriority` |
| `Next` | `ValidatorSet.IncrementProposerPriority` |
| `MaxTotalPower` | `MaxTotalVotingPower = MaxInt64 / 8` |
| (skipped) | `RescalePriorities`, `shiftByAvgProposerPriority` |

### 7. Go lessons

- **Parallel slices:** `priority[i]` belongs to `vals[i]`, kept in step by index.
- **Signed vs unsigned:** use `int64` when a value can go negative, and guard the conversion from `uint64`.
- **`Validators()` returns a clone** (Lesson 11), so the selector can't damage the set.
- **Untyped constants:** `MaxTotalPower` has no fixed type, so it compares directly with a `uint64`.

### 8. Interview questions

1. Walk through weighted round-robin with powers 1, 2, 3.
2. Why must proposer selection be deterministic? What happens if two nodes disagree?
3. Why is the sum of priorities always 0?
4. Why not choose the proposer from the previous block's hash?
5. Why does a predictable proposer create a DoS risk, and what limits the damage?
6. Why does CometBFT rescale priorities, and why don't we need to?

### 9. In my own words

*(Prakash writes this)*

---

## Lesson 14: the happy path (propose → prevote → precommit → commit)

### 1. Why it exists

Lessons 8–13 built the pieces: hashes, signatures, blocks, votes, vote counting and proposer selection. This
lesson connects them, so that 4 validators commit a chain of identical blocks.

```
 Proposer            Everyone                 Everyone                  Everyone
 ────────            ────────                 ────────                  ────────
 PROPOSE  ──block──► PREVOTE  ──2/3+ votes──► PRECOMMIT ──2/3+ votes──► COMMIT ─► height + 1
 "here's B"          "B looks valid to me"    "I saw 2/3+ accept B"     B is final
```

**Happy path** means every validator is online, honest and fast. Timeouts arrive in Lesson 15 and locking in
Lesson 16, and you'll watch this version fail without them.

### 2. Theory

#### 2.1 Why two voting steps

One round of voting isn't enough. Suppose validators committed straight after prevotes, and a node saw 2/3+
prevotes for B while the others, because of the network, never did. That node has committed, the others
move on and could commit a different block in the next round: a fork.

- **Prevote** means *"I think B is valid."*
- **Precommit** means *"I know 2/3+ think B is valid."*
- **Commit** means *"I know 2/3+ know that."*

The full safety argument needs the locking rule from Lesson 16. For now, remember why there are two steps:
a commit must be backed by validators who know that the block is widely accepted.

#### 2.2 The state machine

```
 enterHeight
     │  proposer = selector.Next(); if it's me, broadcast a proposal
     ▼
 StepPropose ──valid proposal──► broadcast Prevote(B)
     ▼
 StepPrevote ──2/3+ prevotes for B──► broadcast Precommit(B)
     ▼
 StepPrecommit
     │
 (any step) ──have B and 2/3+ precommits for B──► commit B, height++, enterHeight
```

#### 2.3 A pure state machine

`State` has **no goroutines, clock or network**. It only does this:

```
 message in ─► State.Handle ─► messages to broadcast
```

So tests can control every message order, one seed reproduces one run, and Lesson 19's simulator will run
thousands of seeded runs. The same core can later sit behind the `node` event loop and a real transport.
See [ADR 0001](../docs/adr/0001-pure-consensus-state-machine.md).

A node also receives its **own** broadcasts, so its own votes get counted through the same checks as
everyone else's.

#### 2.4 Messages arrive in any order

| Arrives | What the node does | Why |
|---|---|---|
| vote for this height, before the proposal | counts it, doesn't act | it can't commit a block it hasn't seen |
| message for a **future** height | keeps it in `pending`, replays on entry | other nodes may simply be ahead |
| message for an **old** height | ignores it, no error | normal on a gossip network |
| invalid proposal | returns an error, doesn't vote | never support a bad block |

Votes are checked by `VoteSet` when replayed, so a bad early message gets dropped then.

#### 2.5 Checks on a proposal

Round → proposer → signature → duplicate or conflict → block contents (`ValidateBasic`, chain ID, height,
parent hash, validator set hash). Cheap checks come first, and nothing is stored until they all pass.

#### 2.6 Why signing the block hash is enough

The signature covers only the block hash, and the header's `TxRoot` commits to the transactions. So one
signature protects every transaction, as long as someone checks that the transactions match `TxRoot`.
`ValidateBasic` does that. Skip it, and a relayer could swap the transactions under a valid signature.

### 3. Known gaps (on purpose, for now)

| Gap | Effect | Fixed in |
|---|---|---|
| no timeouts or rounds | a silent proposer halts the chain forever | Lesson 15 |
| no locking | not yet safe once rounds exist | Lesson 16 |
| `pending` has no size limit | a Byzantine node could fill memory | Lesson 21 |
| no real network or encoding | runs in memory, inside tests | Lesson 23 |
| no double-sign evidence | conflicts are rejected but not recorded | Lesson 22 |

### 4. Files, types and functions

| Name | Meaning |
|---|---|
| `types.Proposal` | Height, Round, Block, Signature; `SignBytes`, `Sign`, `Verify` |
| `consensus.Message` | one broadcast: a proposal or a vote |
| `consensus.Step` | `StepPropose`, `StepPrevote`, `StepPrecommit` |
| `consensus.State` | one validator's state machine |
| `NewState`, `Start` | build it, then enter height 1 |
| `Handle(m)` | one message in, broadcasts out |
| `advance()` | moves forward as far as what it has seen allows |
| `enterHeight()` | resets per-height state, proposes if it's this node's turn, replays `pending` |
| `Height()`, `Blocks()` | read the current height and the committed chain |

### 5. Invariants and tests

| # | Property | Test |
|---|---|---|
| 1 | a proposal signature is bound to its chain, key, height, round and block | `TestProposalSignVerify` |
| 2 | a proposal signature isn't accepted as a vote | `TestProposalAndVoteSignaturesDiffer` |
| 3 | 4 nodes commit identical, correctly linked blocks | `TestFourNodesCommit` |
| 4 | proposers take turns | `TestProposersTakeTurns` |
| 5 | **agreement holds over 50 seeded shuffled delivery orders** | `TestAgreementUnderShuffledDelivery` |
| 6 | with one silent node, the chain halts at that node's turn | `TestOneSilentNodeHaltsAtItsTurn` |
| 7 | with 2 of 4 silent, nothing is ever committed | `TestTwoSilentNodesNeverCommit` |
| 8 | 7 kinds of bad proposal are rejected, with no vote | `TestRejectsBadProposals` |
| 9 | duplicates are ignored, conflicts are errors | `TestProposalDuplicateAndConflict` |
| 10 | no precommit before 2/3+ prevotes | `TestPrecommitNeedsPrevoteQuorum` |
| 11 | no commit without 2/3+ precommits **and** the block | `TestCommitNeedsPrecommitQuorumAndTheBlock` |
| 12 | early messages replay, old ones are ignored | `TestEarlyAndOldMessages` |

### 6. Break experiments (verified on the prototype, 2026-11-01)

| # | Mutation | Caught by |
|---|---|---|
| 1 | proposal without its block hash in `SignBytes` | `signature accepted for a different block` |
| 2 | proposal without chain ID in `SignBytes` | `proposal replayed on another chain` |
| 3 | act without having the block | FourNodes-style runs: `proposer sent a different block` |
| 4 | precommit without a prevote quorum | PrecommitNeedsPrevoteQuorum: `first proposal: out=2` |
| 5 | commit without a precommit quorum | TwoSilent: `committed with only 2 of 4 validators alive` |
| 6 | skip the parent check | RejectsBadProposals/wrong_parent |
| 7 | skip the proposer check | RejectsBadProposals/wrong_proposer |
| 8 | skip the proposal signature check | RejectsBadProposals/bad_signature |
| 9 | drop early messages | Shuffled: `seed 1: node 0 committed only 1 blocks` |
| 10 | never update `lastHash` | FourNodes: `block 2 does not link to its parent` |
| 11 | keep the old proposal at a new height | `proposer sent a different block` |

**One mutation that survived:** changing the proposal's domain tag to `"bft-lab/vote"` fails no test, because
a vote's signed fields already differ from a proposal's (votes include a type byte). So the tag is a second
layer of protection here, and no test proves it on its own. That's a good reminder that 100% coverage doesn't
mean every line is proven.

Prototype result: 100% coverage for `consensus` and `types`, `-race` clean, lint 0 issues on those packages.

### 7. How it maps to CometBFT

| bft-lab | CometBFT |
|---|---|
| `State` | `consensus/state.go` `State` |
| `Handle` | `handleMsg` inside `receiveRoutine` |
| `enterHeight` | `updateToState` + `enterNewRound` |
| `propose` | `enterPropose` / `defaultDecideProposal` |
| `advance` | `enterPrevote`, `enterPrecommit`, `enterCommit` |
| own broadcasts delivered to self | `internalMsgQueue` |
| `pending` | peers re-gossip instead, so CometBFT doesn't need this buffer |

CometBFT mixes its state machine with a timeout ticker, but it replays from a WAL and its tests drive it
message by message, which is the same idea as our pure core.

### 8. Go lessons

- **`iota`:** numbers constants 0, 1, 2 within a `const` block.
- **Optional fields with pointers:** in `Message`, `nil` means "not this kind".
- **`if` with an init statement and `&&`:** `if h, ok := f(); ok && h == hash`.
- **Returning a slice of effects** instead of doing I/O keeps logic testable.
- **Function values as dependencies:** `txs func() [][]byte` lets tests supply transactions now and a mempool later.
- **`*testing.T` helpers and fixtures** keep long tests readable.

### 9. Interview questions

1. Walk through one Tendermint height with 4 validators. How many messages are sent?
2. Why two voting rounds instead of one?
3. Why can't a node commit with 2/3+ precommits if it doesn't have the block?
4. What happens to the chain if the proposer is offline? (Lesson 15 fixes it.)
5. What does a pure state machine make possible for testing?
6. Why does a validator deliver its own votes to itself?
7. How does signing a single block hash protect every transaction?
8. What's the risk in buffering future-height messages?

### 10. In my own words

*(Prakash writes this)*
