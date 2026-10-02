# `ledger`: the state machine

> **One sentence:** `ledger` is the data that all validators must agree on (who owns how much) and
> the rules for changing it.

---

## 1. Why this package exists

A blockchain is two things glued together:

```
┌──────────────────────────────┐
│  CONSENSUS (bft-lab/consensus, later)
│  "In what ORDER do transactions happen?"
│  Validators vote until they agree on the next block.
└──────────────┬───────────────┘
               │ ordered list of transactions
               ▼
┌──────────────────────────────┐
│  STATE MACHINE (this package)
│  "What does each transaction DO?"
│  Apply tx 1, tx 2, tx 3 ... to the balances.
└──────────────────────────────┘
```

Consensus never looks at balances, and the ledger never knows about voting. Keeping them separate is
called **separation of concerns**. In Cosmos the same split exists: **CometBFT** = consensus,
**Cosmos SDK modules** (like `x/bank`) = state machine.

### What is a "state machine"?

A system that has:
- a **state** (here: `address → balance`)
- **transitions** that change the state (here: `Deposit`, `Transfer`)
- **rules** deciding which transitions are allowed (the checks)

```
   state S0                 state S1                 state S2
 alice: 100   Transfer 30   alice: 70    Transfer 5    alice: 65
 bob:   0    ───────────►   bob:   30   ───────────►   bob:   35
```

### Why "replicated" matters

Every validator runs **its own copy** of this ledger. If they all start from the same state and apply
the same transactions in the same order, they must end in the **same state**. That's called
**state machine replication**, and it's the core idea of every blockchain.

```
            same txs, same order
   ┌──────────────┼──────────────┐
   ▼              ▼              ▼
Validator 1   Validator 2   Validator 3
alice: 65     alice: 65     alice: 65     ← must be identical
bob:   35     bob:   35     bob:   35       or the chain splits
```

That's why the ledger must be **deterministic** (see §4).

---

## 2. Data model

```go
type Address string                    // who: on a real chain, derived from a public key
type Ledger struct {
    balances map[Address]uint64        // state: address → balance
}
```

| Decision | Why |
|---|---|
| State is **only** `address → balance` | Names, history, etc. aren't state. Transactions are *operations* on the state, not stored fields |
| `Address` is a named type, not a plain `string` | The compiler stops you passing a random string where an address is expected |
| `balances` is **lowercase** (private) | Nobody outside the package can edit balances directly. Every change must go through the checked functions. Encapsulation is a security property |
| Balances are `uint64` (integers) | Money is never a float. `0.1 + 0.2 = 0.30000000000000004`, and validators that round differently would split the chain. We store the smallest unit (like Cosmos `upay`, where 1 PAY = 1,000,000 upay) |
| `uint64` is unsigned | A balance can never be negative, **but** it can wrap around (see §5) |

---

## 3. Operations

| Function | What it does | Can fail with |
|---|---|---|
| `New()` | empty ledger (map created with `make`, because writing to a nil map panics) | — |
| `Balance(addr)` | read a balance (0 for unknown accounts, the map's zero value) | — |
| `Deposit(addr, amount)` | add money, creating the account if needed | `ErrInvalidAmount`, `ErrOverflow` |
| `Transfer(from, to, amount)` | move money atomically | `ErrInvalidAmount`, `ErrSelfTransfer`, `ErrAccountNotFound`, `ErrInsufficientFunds`, `ErrOverflow` |

Errors are **sentinel errors** (fixed values), so callers check them with
`errors.Is(err, ledger.ErrInsufficientFunds)` instead of comparing text.

### Inside `Transfer`: READ → CHECK → WRITE

```
Transfer(alice → bob, 30)
│
├─ CHECK  amount > 0 ?                     no  → ErrInvalidAmount
├─ CHECK  from != to ?                     no  → ErrSelfTransfer
├─ READ   bal, ok := balances[from]
├─ CHECK  ok (account exists) ?            no  → ErrAccountNotFound
├─ CHECK  bal >= amount ?                  no  → ErrInsufficientFunds   (prevents UNDERFLOW)
├─ CHECK  balances[to] + amount fits?      no  → ErrOverflow            (prevents OVERFLOW)
│
│   ── nothing has been changed up to this line ──
│
├─ WRITE  balances[from] = bal - amount
└─ WRITE  balances[to]  += amount         → return nil (success)
```

**The rule:** read what you need, check *everything*, and only then write. Never write first.
If you write first and a later check fails, the half-done change stays (Go has no automatic
rollback), and the later checks may even read the corrupted values and pass.

---

## 4. Invariants: the rules that must ALWAYS hold

An **invariant** is a property that is true before and after every operation, no matter what input
is given. Invariants are how protocol engineers and auditors reason about safety.

| # | Invariant | Meaning | Enforced by | Proven by |
|---|---|---|---|---|
| 1 | **Atomicity** | A failed operation changes **nothing** (all-or-nothing) | READ → CHECK → WRITE order | `TestTransfer` error rows (want == start), `FuzzTransfer` |
| 2 | **Conservation** | A transfer never creates or destroys money: sender loses exactly `amount`, receiver gains exactly `amount`, so total supply is unchanged | the two WRITE lines use the same `amount` | `FuzzTransfer` (millions of random inputs) |
| 3 | **No wrap-around** | Balances never underflow or overflow | `bal < amount` and `> MaxUint64-amount` checks | overflow/insufficient rows, fuzzing |
| 4 | **Determinism** | Same inputs → same result on every machine | integers only, no time/randomness/map-order dependence | (consensus tests later) |

### Atomicity example

```
Alice 100, Bob MaxUint64. Alice sends 1.
✅ correct:  overflow check fails → return error → Alice 100, Bob MaxUint64 (untouched)
❌ if we wrote first: Alice 99, then error → 1 coin vanished
```

### Conservation, and a trap

You might check conservation as `aliceStart + bobStart == aliceEnd + bobEnd`. **Don't**: the sum
itself can overflow (`MaxUint64 + 10` wraps). `FuzzTransfer` instead checks each side's **change**:
`aliceStart - aliceEnd == amount` and `bobEnd - bobStart == amount`.

### Determinism, and a trap

Go **randomises map iteration order** on purpose. `for addr := range balances` visits accounts in a
different order each run. Harmless when the order doesn't matter, but in consensus code (e.g.
"pay interest to each account, stopping when the pool runs out") different validators would get
different results, and the chain splits. This is one of the most common real Cosmos bugs. The fix is to
sort the keys first.

---

## 5. Overflow vs underflow

`uint64` holds `0 … 18,446,744,073,709,551,615` (`math.MaxUint64`). Going past either end **wraps**:

```
OVERFLOW  (past the top):     MaxUint64 + 1  →  0
UNDERFLOW (below zero):       0 - 1          →  18,446,744,073,709,551,615
```

| Bug | Who | Effect | Guard in `Transfer` |
|---|---|---|---|
| Underflow | sender | balance becomes huge, so **money is created** (theft) | `if bal < amount` |
| Overflow | receiver | balance becomes tiny, so **money is destroyed** | `if balances[to] > MaxUint64 - amount` |

Why the overflow check is written as `a > max - b` and not `a + b > max`: `a + b` would already have
wrapped, so the comparison would be wrong. Rearranging avoids ever computing the overflowing value.

Real-world: the 2018 "batchOverflow" bug let attackers mint huge token amounts in several ERC-20
contracts. Solidity ≥0.8 now checks arithmetic automatically, and Cosmos uses `math.Int` with overflow checks.

---

## 6. Tests and what each one proves

| Test | Type | Proves |
|---|---|---|
| `TestDeposit` | table-driven | deposit works, rejects 0, rejects overflow |
| `TestTransfer` | table-driven | each check fires with the right error **and** balances are unchanged on error (atomicity) |
| `TestTransferAfterEmptyingAccount` | scenario | a drained account still exists (value 0 stays in the map), so the next send gives `ErrInsufficientFunds`, not `ErrAccountNotFound` |
| `FuzzTransfer` | fuzz / property | atomicity + conservation hold for **any** balances and amount (millions of generated inputs) |

**Break experiments done:** removing `if bal < amount` made `TestTransfer/insufficient_funds` fail
with `err = <nil>`. That proves the test really guards the underflow check (mutation testing).

```bash
go test -v -cover ./ledger                           # all tests, coverage
go test -fuzz=FuzzTransfer -fuzztime=30s ./ledger    # fuzz for 30s
```

---

## 7. How this maps to the real Cosmos SDK

| Here | Cosmos SDK (`x/bank`) |
|---|---|
| `Ledger.balances` map | balances in a KV store, accessed through `collections` |
| `Transfer` | `Keeper.SendCoins`: checks/subtracts the sender's balance, then credits the receiver |
| our manual READ → CHECK → WRITE | same discipline **plus** the SDK runs each tx on a *cached* store and throws away all its writes if the tx fails |
| sentinel errors | registered errors like `sdkerrors.ErrInsufficientFunds` |
| `FuzzTransfer` invariants | `x/bank` registers invariants such as non-negative balances and total supply |
| `uint64` + manual checks | `math.Int` (arbitrary precision) with overflow checks |

---

## 8. Interview questions this package prepares you for

1. What's the difference between consensus and the state machine?
2. What is state machine replication, and why must the state machine be deterministic?
3. What is atomicity? What goes wrong if you write state before validating?
4. Explain integer overflow vs underflow. Which one lets an attacker steal funds?
5. Why is money never stored as a float on a blockchain?
6. Why is iterating a Go map dangerous in consensus code?
7. What is an invariant? Give two for a token ledger and explain how you'd test them.

---


