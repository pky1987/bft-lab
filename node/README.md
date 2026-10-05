# `node`: a validator's event loop

> **One sentence:** `node` runs one goroutine per validator that waits for **a message, a timeout,
> or a shutdown signal**, and handles whichever comes first, one event at a time.

---

## 1. Why this package exists

A validator spends its life **waiting**:

- "a vote arrived from a peer" → process it
- "the proposer has been silent too long" → give up on this round, move on (a **timeout**)
- "the operator is stopping the node" → shut down cleanly

These three things can happen in any order, at any time. The **event loop** is the piece of code that
waits for all three at once and reacts to whichever happens first. Every real consensus engine has one.

```
                ┌──────────────────────── Node.Run (ONE goroutine) ─────────────────────┐
  messages ───► │                                                                        │
 (ChanTransport │   for {                                                                │
   .Inbox)      │     select {                                                           │
                │       case msg := <-Inbox:   OnMessage(msg); restart timer             │
  timer ──────► │       case <-timer.C:        OnTimeout();    restart timer             │
                │       case <-ctx.Done():     return ctx.Err()   ← clean shutdown       │
  ctx ────────► │     }                                                                  │
 (cancel/       │   }                                                                    │
  deadline)     └────────────────────────────────────────────────────────────────────────┘
```

---

## 2. Theory: the single-owner event loop

### One goroutine owns the state

In `p2p`, many goroutines touched shared data, so we needed mutexes. The event loop flips that around:
**only the `Run` goroutine ever touches the node's consensus state**. Other goroutines don't
change the state. They **send events** to it through channels.

| Shared state + mutexes (`p2p` §6) | Single owner + channels (this package) |
|---|---|
| many goroutines lock, modify, unlock | one goroutine modifies, and the others send it messages |
| easy to forget a lock → data race | no locks needed for the owner's state |
| hard to reason about ordering | events are handled **one at a time, in a clear order** |

This is the **actor model**, and it's exactly how CometBFT is built: its consensus `receiveRoutine`
is one goroutine with one big `select` over peer messages, internal messages, timeout ticks and quit.
Because only that goroutine changes consensus state, most consensus logic needs **no locks**.

### Handlers are function values

```go
OnMessage func(p2p.Message)
OnTimeout func()
```
In Go, **functions are values**: you can store them in a struct field and call them later. The node
doesn't know *what* to do with a message (that will be the consensus logic, in Week 3). It only knows
*when* to call the handler. This separation makes the loop testable: tests pass in handlers that simply
record what happened.

Both handlers run **on the `Run` goroutine**, so they must not block for long. While a handler runs,
the node can't react to anything else.

---

## 3. Theory: `context`, cancellation and deadlines

`context.Context` is Go's standard way to say **"stop what you're doing"** across goroutines.

```go
ctx, cancel := context.WithCancel(context.Background())   // can be stopped by calling cancel()
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second) // stops itself after 5s
defer cancel()          // ALWAYS call cancel, or the context's resources leak

<-ctx.Done()            // a channel that is closed when the context is cancelled or times out
ctx.Err()               // why it stopped: context.Canceled or context.DeadlineExceeded
```

| Rule | Why |
|---|---|
| `ctx` is the **first parameter**: `Run(ctx context.Context)` | Go convention, used everywhere (net/http, gRPC, Cosmos SDK) |
| always `defer cancel()` | releases timers and goroutines tied to the context. `go vet` warns if you forget |
| check `errors.Is(err, context.Canceled)` | the error tells the caller **why** the work stopped |
| `context.Background()` | the empty root context, where every context tree starts |

**Why the loop must select on `ctx.Done()`:** without it, `Run` can never stop. The goroutine runs
forever (a **goroutine leak**), and a validator could never shut down cleanly. Cosmos SDK nodes stop
all their services exactly this way.

---

## 4. Theory: timers and why timeouts give liveness

### Why consensus needs timeouts

Remember: when a message is lost, the sender isn't told (`LossyTransport` §5 in `p2p`). If the proposer
crashes, a node waiting only for messages would **wait forever**, and the chain would stop. That
breaks **liveness** (the chain must keep making progress).

Tendermint/CometBFT solves this with timeouts at every step: if nothing useful happens in time, the
node gives up on that step and moves on (e.g. to the next round with a new proposer). CometBFT's
config has exactly these knobs: `timeout_propose`, `timeout_prevote`, `timeout_precommit`.

> **Safety never depends on timeouts. Liveness does.** A slow network may delay blocks, but it must never
> cause two honest nodes to commit different blocks.

### `time.Timer`

```go
timer := time.NewTimer(d)   // timer.C receives one value after d
defer timer.Stop()          // release it when Run returns
timer.Reset(d)              // start counting from now again
```

| Choice | When |
|---|---|
| `time.After(d)` | one-off waits in tests (`select { ...; case <-time.After(time.Second): }`) |
| `time.NewTimer(d)` + `Reset` | inside a **loop**. It reuses one timer instead of creating a new one per iteration |

**"Restart timer" after every event** means the timeout measures **silence**: it fires only if
`Timeout` passes with **no** message. (Since Go 1.23, `Reset` is safe to call without draining the
channel first. Our `go.mod` is 1.27, so the simple version is correct.)

---

## 5. Types and functions

```go
type Node struct {
    ID        p2p.NodeID
    Inbox     <-chan p2p.Message // receive-only: from ChanTransport.Inbox
    Timeout   time.Duration
    OnMessage func(p2p.Message)
    OnTimeout func()
}

func (n *Node) Run(ctx context.Context) error
```

| Part | Meaning |
|---|---|
| `Inbox <-chan p2p.Message` | the node can only **read** its inbox (the type from `p2p` §7) |
| `time.Duration` | Go's type for lengths of time: `20 * time.Millisecond`, `time.Second` |
| `Run` returns `error` | always `ctx.Err()`: the reason it stopped |

---

## 6. Properties and the tests that will prove them

| Property | Test |
|---|---|
| a message sent through `ChanTransport` reaches `OnMessage` | `TestNodeHandlesMessage` |
| with no messages, `OnTimeout` fires (repeatedly) | `TestNodeTimeoutFires` |
| `cancel()` makes `Run` return `context.Canceled` | `TestNodeStopsOnCancel` |
| a context deadline makes `Run` return `context.DeadlineExceeded` | `TestNodeStopsOnDeadline` |
| messages arriving more often than `Timeout` **prevent** the timeout (the timer is reset) | `TestNodeMessageResetsTimer` |

**Verified while preparing this lesson** (prototype, 10 runs under `-race`, 100% coverage):

| Mutation | Caught by |
|---|---|
| remove the `case <-ctx.Done()` | `TestNodeStopsOnCancel` fails ("Run did not stop"), and the deadline test hangs until the test timeout |
| remove `timer.Reset` after a message | `TestNodeMessageResetsTimer` fails ("timeout fired although messages kept arriving") |

### Testing time-based code without flakiness

- use **short** durations for the thing being tested (20 ms) and **long** safety nets in the test (1 s)
- never assert "it took exactly 20 ms". Assert "it happened" and "it happened before the safety net"
- a timeout you **don't** want to fire gets a huge value (`time.Hour`), so it can't interfere
- later in `bft-lab` we'll go further: the simulator will use a **fake clock**, so time is fully deterministic

---

## 7. How this maps to CometBFT

| Here | CometBFT |
|---|---|
| `Node.Run` loop with `select` | `consensus/state.go` `receiveRoutine` |
| `Inbox` | peer message queue fed by the p2p layer |
| `Timeout` + `OnTimeout` | the timeout ticker: `timeout_propose` / `timeout_prevote` / `timeout_precommit` |
| `ctx.Done()` | the service's quit signal when the node stops |
| `OnMessage` | handling proposals, block parts and votes |

---

## 8. Interview questions this package prepares you for

1. Why does a consensus engine use a single goroutine event loop instead of locks everywhere?
2. What happens to a BFT chain without timeouts if the proposer crashes?
3. "Safety never depends on timeouts, liveness does." Explain.
4. What is `context.Context` for? What's the difference between `Canceled` and `DeadlineExceeded`?
5. What is a goroutine leak, and how does selecting on `ctx.Done()` prevent it?
6. How do you test time-dependent code without making it flaky?

---

## 9. In my own words (Prakash, fill this in)

- An event loop is:
- The node waits on three channels because:
- `context` is used to:
- Timeouts give liveness because:
