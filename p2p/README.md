# `p2p`: how nodes talk to each other

> **One sentence:** `p2p` moves messages (votes, proposals) between validators, behind an
> interface, so consensus code works the same on a perfect test network, a deliberately broken
> network, or real TCP.

---

## 1. Why this package exists

Validators must exchange messages constantly ("here's my proposed block", "I vote yes"). But **how**
messages travel depends on the situation:

| Situation | Network we want |
|---|---|
| Unit tests | in-memory, instant, perfectly ordered, deterministic |
| Simulator | in-memory, but **we** inject loss, delay, reordering, partitions |
| Real demo | TCP between separate processes |

If consensus code called TCP functions directly, you could never test it deterministically, and you
could never simulate a network failure. So consensus depends on a **contract** (`Transport`), and
we swap implementations underneath it.

```
            consensus (later)
                  │  only knows "Transport"
                  ▼
        ┌───────────────────┐
        │ Transport (iface) │   Send(msg) error
        └─────────┬─────────┘   Receive(id) (Message, bool)
                  │             (all implementations are safe for concurrent use, see §6)
      ┌───────────┼────────────────┬──────────────────┐
      ▼           ▼                ▼                  ▼
 MemTransport  LossyTransport   ChanTransport     TCPTransport
  (tests)      (fault injection) (channels, §7)    (Week 3)
```

This is **layering**: each layer does one job and knows nothing about the layers above it.
`Transport` carries `[]byte` payloads and never looks inside. It doesn't know what a vote is.

---

## 2. Theory: interfaces in Go

An **interface** is a list of method signatures: a contract that says *what*, never *how*.

```go
type Transport interface {
    Send(msg Message) error
    Receive(id NodeID) (Message, bool)
}
```

- **Implicit satisfaction:** there's no `implements` keyword. Any type that has these two methods with
  these exact signatures **is** a `Transport`, automatically.
- **Small is good:** a Go proverb says *"the bigger the interface, the weaker the abstraction."* A
  validator needs only two things from the network, send and receive, so the interface has two methods.
- **Compile-time check:**
  ```go
  var _ Transport = (*MemTransport)(nil)
  ```
  "a `*MemTransport` must be usable as a `Transport`." If someone renames `Send`, the **build** fails
  on this line, instead of a confusing error far away. `_` means "I only want the check, not a variable."

**Analogy:** a USB-C port. The phone (consensus) only cares about the port (interface). The
charger, power bank or laptop (implementations) can be swapped freely.

**Where you'll see it again:** every Cosmos SDK module has `types/expected_keepers.go`, which
declares interfaces like `BankKeeper` with only the methods that module needs. Modules depend on
interfaces, never on each other's concrete types. CometBFT's own `p2p` package also defines a
`Transport` interface.

---

## 3. Types

| Type | Meaning |
|---|---|
| `NodeID string` | identifies a validator on the network |
| `Message{From, To, Payload []byte}` | one unit of communication. `Payload` is opaque bytes (encoded vote/proposal) |
| `ErrUnknownNode` | sentinel error: sending to a node that isn't registered |
| `Transport` | the interface (contract) |

---

## 4. `MemTransport`: the perfect network

Each node has an **inbox**, a queue of messages. Delivery is instant and **FIFO** (first in, first out).

```
Send(alice→bob, m1), Send(m2), Send(m3)

inboxes:
  alice: []
  bob:   [m1, m2, m3]
           ▲        ▲
           │        └── append(): new messages go to the BACK
           └── Receive("bob") takes from the FRONT  → m1, then m2, then m3
```

| Method | Behaviour | Go concepts |
|---|---|---|
| `NewMemTransport()` | empty network | `make(map...)` |
| `Register(id)` | create an inbox. **Idempotent**: registering twice doesn't wipe it | comma-ok, `if !ok` |
| `Send(msg)` | append to `msg.To`'s inbox, or `ErrUnknownNode` | `append` (must assign the result back!) |
| `Receive(id)` | pop the oldest message, or `(Message{}, false)` if empty | slicing `inbox[1:]`, zero value `Message{}`, multiple returns |

### Properties (each one has a test)

| Property | Why consensus needs it | Test |
|---|---|---|
| **FIFO order** | votes processed in the order sent. Makes runs reproducible | `TestMemTransportFIFO` |
| **Unknown node is an error** | a silently "successful" send to a typo'd validator would make votes vanish | `TestMemTransportFIFO` (ghost send) |
| **Empty inbox returns false** | the consensus loop polls `Receive` constantly and must never process a fake message | `TestMemTransportEmptyInbox` |
| **Isolation** | a node must never see another node's messages | `TestMemTransportIsolation` |

---

## 5. `LossyTransport`: breaking the network on purpose (Lesson 4 ✅)

### Why

`MemTransport` is perfect, and real networks are not. **BFT consensus exists precisely to stay safe on a
bad network.** To prove that, we need to make the network misbehave **in a controlled way**. That's
**fault injection**.

### The decorator (wrapper) pattern

Instead of editing `MemTransport` (which would make it messy and break its tests), we **wrap** it:

```
consensus ── Send(msg) ──► LossyTransport ── roll dice ──► dropped (counted, gone)
                                │
                                └── passed on ──► MemTransport (unchanged)
```

`LossyTransport` **is** a `Transport` (it has `Send` and `Receive`) and **holds** a `Transport`
(`inner`). Consensus can't tell the difference. Wrappers can be stacked later:
`Lossy(Delay(Reorder(TCP)))`.

### Fields and methods

```go
type LossyTransport struct {
    inner    Transport    // the wrapped network: any Transport (Mem, TCP, another wrapper)
    dropRate float64      // 0.0 = never drop, 1.0 = drop everything
    rng      *rand.Rand   // seeded random number generator (math/rand/v2)
    dropped  int          // how many messages were lost so far
}
```

| Method | Behaviour | Go concepts |
|---|---|---|
| `NewLossyTransport(inner, dropRate, seed)` | builds the wrapper with a seeded RNG: `rand.New(rand.NewPCG(seed, seed))` | constructor returning a pointer, interface-typed field |
| `Send(msg)` | rolls `rng.Float64()` (a number in 0.0–1.0). If it's `< dropRate`, the message is dropped (`dropped++`, return `nil`), otherwise **delegates** to `inner.Send(msg)` | delegation, early return |
| `Receive(id)` | passes straight through to `inner.Receive(id)`. Loss happens once, on send, so it isn't doubled | delegation |
| `Dropped()` | returns the drop counter, so tests can check the rate | getter method |

```
Send(msg)
   │
   ▼
rng.Float64()  →  e.g. 0.21
   │
   ├── 0.21 < dropRate (0.3)?  yes → dropped++ → return nil   (message is gone, sender not told)
   │
   └── no → inner.Send(msg)    → MemTransport appends to inbox
```

### Seeded randomness = replayable failures

- Unseeded randomness means a bug appears on one run and vanishes on the next (a **flaky** failure), and you can never debug it.
- A **seeded** RNG (`rand.New(rand.NewPCG(seed, seed))`) produces the **same** sequence every time.
  Print the seed when a run fails, and anyone can replay the exact failure.
- `dropRate` is a `float64`. That's fine here because this is **test infrastructure**, not chain
  state. Validators never compute balances with it.

### Measured results (reproducible: same seed, same numbers, every machine)

| Setup | Result |
|---|---|
| 100 messages, 50% loss, seed 42 | **47** delivered |
| 100 messages, 50% loss, seed 7 | **42** delivered (a different seed gives a different pattern) |
| 10,000 messages, 30% loss, seed 42 | **3,025** dropped (30.25%) |
| 10,000 messages, 30% loss, seed 1 | **2,989** dropped (29.89%) |

The rate isn't exactly 30% because it's random, but with a fixed seed it is **exactly the same every
run**. That's why `TestLossyRoughRate` can check a range (2,700–3,300) without ever being flaky.

### Properties (each one has a test)

| Property | Test | How |
|---|---|---|
| **Deterministic**: same seed → identical delivered messages. Different seed → different | `TestLossyDeterministic` | runs the `delivered` helper 3× (seeds 42, 42, 7) and compares |
| **dropRate 0 loses nothing**, and order is preserved | `TestLossyNeverDrops` | 100 sent → 100 received in order, `Dropped() == 0` |
| **dropRate 1 loses everything** | `TestLossyDropEverything` | 100 sent → inbox empty, `Dropped() == 100` |
| **Drop rate is roughly correct** | `TestLossyRoughRate` | 10,000 sent at 0.3 → `Dropped()` within 2,700–3,300 |

**Mutation check:** flipping `<` to `>` in `Send` (so the drop rate becomes `1 - dropRate`) makes
`TestLossyNeverDrops`, `TestLossyDropEverything` and `TestLossyRoughRate` fail. That proves the
tests guard the drop logic. (`TestLossyDeterministic` still passes, because the flipped version is
still deterministic. Each test guards a different property.)

### Why a dropped `Send` returns `nil`, not an error

On a real network you send a packet and **never learn** whether it arrived. Consensus must cope
using **timeouts** (stop waiting) and **retries/re-gossip**. You'll build those in Weeks 2–4.

---

## 6. Concurrency: goroutines, WaitGroup, data races, mutex (Lesson 5 ✅)

### Why

Real validators do many things **at the same time**: receive votes from many peers, send their own,
run timers, execute blocks. In Go, "at the same time" means **goroutines**. As soon as goroutines
share data, you can get **data races**, the most dangerous bug class in distributed systems.

### Goroutines

A **goroutine** is a function running **concurrently** with the rest of the program. Start one with `go`:

```go
sendVotes()      // normal call: wait until it finishes
go sendVotes()   // goroutine: start it and continue IMMEDIATELY
```

- Very cheap: a program can run **millions**. That's one reason CometBFT, Cosmos SDK, geth and Kubernetes are written in Go.
- Starting one with an argument:
  ```go
  go func(id int) {
      // ... uses id ...
  }(s)          // ← (s) CALLS the function. It must be on the same line as }
  ```
  Go inserts an automatic `;` after a `}` at the end of a line, so `}` ⏎ `(s)` becomes
  `go func(){...};` with no call, giving the error *"expression in go must be function call"*.
- Inside a goroutine, report failures with `t.Errorf` + `return`, never `t.Fatalf`. `Fatalf` may only be
  called from the test's own goroutine.

### `sync.WaitGroup`: waiting for goroutines to finish

```go
var wg sync.WaitGroup
wg.Add(1)            // counter +1: "one more worker"
go func() {
    defer wg.Done()  // counter -1 when this function exits (however it exits)
    // ... work ...
}()
wg.Wait()            // block until the counter is 0
```

Without `Wait()`, the test would check results **before** the goroutines finished.
`defer` means "run this when the surrounding function exits", so `Done()` always runs, even on an early `return`.

`Wait()` also makes everything the goroutines wrote **visible** to the code after it. That's why the
test can safely read `received` (written only by bob's goroutine) after `wg.Wait()`.

### Data races

A **data race** happens when two goroutines access the **same memory** at the same time and **at least one writes**.

```
 goroutine A                         goroutine B
 read  bob's inbox → [m1, m2]
                                     read  bob's inbox → [m1, m2]
 write [m1, m2, m3]
                                     write [m1, m2, m4]     ← m3 LOST
```

**Lost update:** even `x++` is **three** steps (read, add, write). Two goroutines doing it at once can
both read 305 and both write 306, so one increment vanishes.

What we actually observed in this package:

| Situation | Result |
|---|---|
| `MemTransport` without a mutex, 10 concurrent senders, no `-race` | the program **crashed**: `fatal error: concurrent map writes` (for a validator, a **chain halt**) |
| same, with `-race` | `WARNING: DATA RACE`: read at `memory.go` (map lookup) vs. previous write at `memory.go` (append), with the exact lines and goroutines |
| `LossyTransport` without a mutex, no `-race` | `delivered 693 + dropped 306 != sent 1000`: one message vanished from the accounting (a lost update on `dropped++`) |

Races are **random**: the code works 99 times and fails on the 100th, on another machine, under load.
That's why we always test with `-race` and repeat with `-count=3`.

### Reading a race report

```
WARNING: DATA RACE
Read at 0x00c00018c6f0 by goroutine 9:          ← who READ, and where
  ...(*MemTransport).Send()  memory.go:25
Previous write at 0x00c00018c6f0 by goroutine 11: ← who WROTE the same address
  ...(*MemTransport).Send()  memory.go:29
Goroutine 9 (running) created at:
  concurrent_test.go:18                          ← where the goroutines were started
```
Same address + two goroutines + at least one write = a data race. The line numbers show where to add protection.

### `sync.Mutex`: one goroutine at a time

```go
m.mu.Lock()          // enter. If someone else is inside, WAIT here
// critical section: only one goroutine at a time
m.mu.Unlock()        // leave. The next waiting goroutine may enter
```
Analogy: a single-person bathroom with a lock.

**How `MemTransport` uses it:**
```go
type MemTransport struct {
    mu      sync.Mutex // guards inboxes
    inboxes map[NodeID][]Message
}

func (m *MemTransport) Send(msg Message) error {
    m.mu.Lock()
    defer m.mu.Unlock()
    ...
}
```

| Rule | Why |
|---|---|
| `defer m.mu.Unlock()` right after `Lock()` | functions with several `return`s would otherwise need `Unlock()` before each one. Forget one, and the next caller waits forever (**deadlock**, the node freezes) |
| Lock **every** method that touches the guarded data (`Register`, `Send`, `Receive`) | a race needs only one writer. `Receive` writes the map too |
| Put `mu` right above the fields it guards, with a comment | Go convention, so readers know what's protected |
| **Never copy a mutex**: use pointer receivers `(m *MemTransport)` | a copied mutex locks its own copy and protects nothing. `go vet` (copylocks) catches this |
| **Lock the data, not the methods** | `LossyTransport.Receive` touches no guarded field, so it needs no lock |
| **Keep critical sections small. Don't hold your lock while calling other components** | `LossyTransport.Send` locks only around `rng`/`dropped`, copies the decision into `drop`, unlocks, and **then** calls `inner.Send`. Holding a lock across calls into other code is slower and is how deadlocks start (A holds lock 1 and waits for 2, while B holds 2 and waits for 1) |

**`LossyTransport` (Exercise 2):** `rng.Float64()` changes internal state (`rand.Rand` is not safe for
concurrent use), and `dropped++` is a read-modify-write. Both are guarded by `mu`, and `Dropped()` locks too, because reading while others write is also a race.

### Concurrency tests

| Test | Scenario | Catches |
|---|---|---|
| `TestMemTransportConcurrentSend` | 10 goroutines × 100 sends to bob at once, then count | races in `Send` / `Register` |
| `TestMemTransportConcurrentSendReceive` | bob receives **while** the 10 senders send | races in `Receive`. Verified: removing `Receive`'s lock is caught here (7 data races), but **not** by the test above, because there `Receive` only runs after `wg.Wait()` and never overlaps with `Send` |
| `TestLossyConcurrentSend` | 10 goroutines send through a 30% lossy network. Invariant: delivered + dropped == 1,000 | races on `rng` and `dropped` (conservation again) |

```bash
go test -count=3 -race -v -run Concurrent ./p2p
```

---

## 7. Channels and `select`: `ChanTransport` (Lesson 6 ✅)

### Why

In §6 goroutines **shared** a map and took turns with a mutex. Go offers a second style, and it's the
one CometBFT is built around:

> *"Don't communicate by sharing memory; share memory by communicating."* (Go proverb)

Instead of many goroutines touching the same data, one goroutine **sends** a value through a
**channel** and another **receives** it. The channel does the synchronisation for you.

Why consensus needs this: a validator constantly waits for **several things at once**: "a vote
arrived" **or** "the round timed out" **or** "shut down". Only channels + `select` can express
"wait for whichever happens first". A mutex can't.

### Channels: the basics

A channel is a typed pipe between goroutines.

```go
ch := make(chan Message)       // unbuffered: holds 0 messages
ch := make(chan Message, 100)  // buffered:   holds up to 100 messages

ch <- msg                      // SEND    (arrow points INTO the channel)
msg := <-ch                    // RECEIVE (arrow points OUT of the channel)
msg, ok := <-ch                // ok == false once the channel is closed and empty
close(ch)                      // no more sends. Receivers drain what's left, then get ok == false
```

**Blocking rules (the most important thing to memorise):**

| Channel | Send blocks when… | Receive blocks when… |
|---|---|---|
| **unbuffered** `make(chan T)` | nobody is receiving right now | nobody is sending right now |
| **buffered** `make(chan T, n)` | the buffer is **full** (n items) | the buffer is **empty** |
| **nil** (declared, never `make`d) | forever | forever |

```
unbuffered: a hand-to-hand handoff, where both people must be there at the same moment
   sender ──(waits)──► 🤝 ◄──(waits)── receiver

buffered (cap 3): a mailbox. The sender only waits when it's full
   sender ──► [ m1 | m2 | m3 ] ──► receiver
```

**Channels are already goroutine-safe.** Many goroutines can send to and receive from the same channel
with no mutex. And a channel is **FIFO**: messages from one sender come out in the order they went in.

### Deadlock: the channel version of a frozen node

```go
ch := make(chan int)   // unbuffered
ch <- 1                // nobody will ever receive, so this blocks forever
```
Verified output:
```
fatal error: all goroutines are asleep - deadlock!
goroutine 1 [chan send]:
```
Go detects that **every** goroutine is stuck and kills the program. In a validator, a deadlock means
the node stops voting, and the chain may stall.

A related bug is the **goroutine leak**: one goroutine blocked forever (waiting on a channel nobody
will ever use) while the rest of the program keeps running. No crash, just slowly growing memory.

### `select`: wait for whichever happens first

```go
select {
case msg := <-inbox:          // a message arrived
	handle(msg)
case <-time.After(time.Second): // or 1 second passed with nothing
	fmt.Println("timeout")
}
```
- `select` blocks until **one** case is ready, then runs **only that case**.
- If several cases are ready, it picks one **at random** (yes, randomness again: never rely on case order).
- With a `default` case, `select` **never blocks**: if nothing is ready, `default` runs immediately.

| Pattern | Code | Used in `ChanTransport` for |
|---|---|---|
| **Non-blocking send** | `select { case ch <- m: ...; default: ... }` | `Send`: a full inbox returns `ErrInboxFull` instead of freezing the sender |
| **Non-blocking receive** | `select { case m := <-ch: ...; default: ... }` | `Receive`: an empty inbox returns `false` (satisfies the `Transport` contract) |
| **Blocking receive with timeout** | `select { case m := <-ch: ...; case <-time.After(d): ... }` | how an event loop waits on `Inbox()` (the node, next lesson) |

### Directional channel types

```go
func (c *ChanTransport) Inbox(id NodeID) (<-chan Message, bool)
```
`<-chan Message` means **receive-only**. Whoever calls `Inbox` can read messages but **can't** send
into or close someone else's inbox. The compiler enforces it. (`chan<- Message` is send-only.)
It's the same idea as the lowercase `balances` field in the ledger: limit what callers can do.

### Backpressure

A buffered inbox has a fixed size. What should happen when a slow node's inbox is full?

| Option | Problem |
|---|---|
| block the sender until there's room | one slow validator freezes everyone who sends to it |
| grow the buffer forever | memory exhaustion (a **DoS** vector: an attacker floods you with messages) |
| **reject with `ErrInboxFull`** ✅ | the sender knows immediately and can drop, retry or slow down |

Refusing work you can't handle is called **backpressure**. Real nodes need it too: CometBFT's peer
connections have bounded send queues for the same reason.

### Mutex vs channel: which one, when?

| Use a **mutex** when… | Use a **channel** when… |
|---|---|
| protecting shared **state** (a map, a counter) | passing **ownership of data** or **events** between goroutines |
| the critical section is short | you need to **wait** for something, or for one of several things (`select`) |
| e.g. `MemTransport.inboxes`, `LossyTransport.dropped` | e.g. node inboxes, timeouts, shutdown signals |

`ChanTransport` uses **both**: an `RWMutex` guards the **map** of inboxes (which can change when
nodes register), and the **channels** carry the messages themselves.

`sync.RWMutex` is a mutex with two modes: **many readers at once** (`RLock`), or **one writer
alone** (`Lock`). `Send`/`Receive`/`Inbox` only *read* the map, so they share `RLock`. `Register`
*writes* it, so it takes `Lock`.

### How CometBFT does it

The heart of CometBFT's consensus engine (`consensus/state.go`, the `receiveRoutine`) is a single
goroutine running a loop around **one big `select`**: peer messages, internal messages, timeout
ticks and a quit signal. Only that goroutine changes consensus state, so most consensus logic needs
no locks at all. You'll build the same shape in the next lesson.

### Properties and the tests that will prove them

| Property | Test |
|---|---|
| FIFO for a single sender | `TestChanTransportFIFO` |
| unknown node → `ErrUnknownNode`. `Receive`/`Inbox` on unknown nodes → `false` | `TestChanTransportUnknownNode` |
| full inbox → `ErrInboxFull` (backpressure). `Register` is idempotent | `TestChanTransportInboxFull` |
| `Inbox()` can be waited on with `select` + timeout | `TestChanTransportInboxBlocking` |
| safe under concurrent send + receive | `TestChanTransportConcurrent` |
| safe when nodes **register** while others send | `TestChanTransportConcurrentRegister` |

### Results (measured on the committed code, `3c77e36`)

- 6 ChanTransport tests (in `chain_test.go`), all PASS 3× under `-race`. `chan.go` is at **100%** coverage per function
- Coverage guided the work: before `TestChanTransportInboxFull` existed, `Send` showed **83.3%**, and
  `go tool cover -html` showed the `return ErrInboxFull` line in red. Coverage is a map of what's still untested
- Implementation note: this version guards the map with a plain `sync.Mutex`. That's correct (no races),
  but `sync.RWMutex` + `RLock` in `inbox()` would let lookups run in parallel

**Break experiments (run on a copy of the committed code):**

| Mutation | `TestChanTransportConcurrent` | `TestChanTransportConcurrentRegister` | `TestChanTransportInboxFull` |
|---|---|---|---|
| remove the lock in `inbox()` | ✅ passes (misses it) | ❌ **`DATA RACE`** | – |
| make `Send` blocking (`ch <- msg`) | – | – | ❌ **hangs**: `panic: test timed out after 10s` |

Why only `ConcurrentRegister` catches the missing lock: it's the only test that **writes** the map
(`Register`) while another goroutine **reads** it (`Send` → `inbox()`). A blocking send on a full
inbox waits forever for a receiver that never comes. Go's runtime didn't report "all goroutines are asleep"
here because the test framework keeps other goroutines alive, so the symptom is a **hang**, caught only
by the test timeout. In a validator, the same bug means a node freezes when one peer stops reading.

**Lessons from writing these tests:**
- copying `TestMemTransportFIFO` carried over a `To: "ghost"` → `ErrUnknownNode` check. Changing "ghost"
  to "bob" silently reversed its meaning, **and** left an extra message in the inbox that broke the order
  check. One test = one behaviour: if a line doesn't serve the test's sentence, delete it
- capacity N means N sends succeed and send N+1 fails (the first draft used capacity 3 and expected the 3rd send to fail)
- a concurrency test needs a capacity large enough that **timing can't cause failures** (`NewChanTransport(total)`),
  otherwise it's flaky
- `t.Errorf("Error:", err)` → vet error "call has arguments but no formatting directives": every value needs a `%v`

---

## 8. Failure models (what we'll inject over the coming weeks)

| Fault | Real-world cause | Wrapper |
|---|---|---|
| **Loss** | congestion, dropped packets | `LossyTransport` (now) |
| **Delay** | slow links, distance | `DelayTransport` |
| **Reordering** | different network paths | `ReorderTransport` |
| **Duplication** | retransmissions | `DuplicateTransport` |
| **Partition** | a datacenter is cut off, splitting nodes into groups that can't talk | `PartitionTransport` |
| **Crash** | a node goes offline | stop the node |
| **Byzantine** | a malicious node sends conflicting votes | a Byzantine node implementation |

Consensus guarantees under faults:
- **Safety:** honest nodes never commit different blocks. Must hold **always**, even under partitions.
- **Liveness:** the chain keeps making progress. Only required once the network behaves again.

---

## 9. Tests

```bash
go test -count=3 -race -v -cover ./p2p   # everything, 3×, with the race detector
go test -count=1 -v -run TestLossy ./p2p # -count=1 = ignore cached results
```

| File | Tests | Coverage |
|---|---|---|
| `memory_test.go` | `TestMemTransportFIFO`, `TestMemTransportEmptyInbox`, `TestMemTransportIsolation` | 100% |
| `lossy_test.go` | `TestLossyDeterministic`, `TestLossyNeverDrops`, `TestLossyDropEverything`, `TestLossyRoughRate` | 100% |
| `concurrent_test.go` | `TestMemTransportConcurrentSend`, `TestMemTransportConcurrentSendReceive`, `TestLossyConcurrentSend` | 100% |
| `chain_test.go` | `TestChanTransportInboxBlocking`, `TestChanTransportFIFO`, `TestChanTransportUnknownNode`, `TestChanTransportInboxFull`, `TestChanTransportConcurrent`, `TestChanTransportConcurrentRegister` | 100% |

**Break experiments:**
- switching `Receive` to LIFO (`inbox[len(inbox)-1]`) makes `TestMemTransportFIFO` fail, which proves the test really checks order
- flipping `<` → `>` in `LossyTransport.Send` fails 3 of the 4 Lossy tests (see §5)
- removing the lock from `MemTransport.Receive` is caught only by `TestMemTransportConcurrentSendReceive` (see §6)

---

## 10. Go lessons learned while building this package

- `if x := f(); cond { }` needs **both** parts. A missing condition gives `expected operand`
- `:=` creates variables (at least one must be new). `=` reuses existing ones
- **Scope/shadowing:** `err` created inside `if err := ...;` exists only inside that `if`. Outside, the
  name refers to the *outer* `err`. This caused a real failing test (`unknown node`)
- `Test` + lowercase letter (`Testmem...`) is **not** a test, and `go vet` rejects it
- Struct field names are case-sensitive: `Payload` ≠ `payload`
- `return` leaves the **whole function**, not just the loop (`break` leaves a loop)
- `Message{...}` builds a struct, while `Message(...)` would be a function call or conversion
- `if got := f() != 100` stores a **bool** (comparison happens first). You need `if got := f(); got != 100`
- A test function has exactly the shape `func TestXxx(t *testing.T)`, with **no return values**
- `t.Logf` only prints, and `t.Fatal`/`t.Fatalf` fails the test. A check that only logs can never catch a bug
- Work out expected numbers by hand first: 30% of 1,000 is 300, not 3,000
- `}` and `(s)` must be on the same line in `go func(){...}(s)`, because of automatic semicolon insertion
- `x++` is not atomic: read, add, write
- A test only proves what it exercises: a race in `Receive` is invisible unless `Receive` runs **concurrently** with `Send`

---

## 11. Interview questions this package prepares you for

1. Why does consensus code depend on an interface instead of a concrete network type?
2. What's the decorator pattern, and how does it help fault injection?
3. Why must simulation randomness be seeded?
4. What's the difference between safety and liveness? Which one may be lost during a partition?
5. A message was sent but never arrived, and the sender got no error. How should consensus handle it?
6. What is a data race? Why is `counter++` unsafe across goroutines, and how do you detect races in Go?
7. Why should you avoid holding a mutex while calling into another component?
8. In a validator, what are the consequences of `fatal error: concurrent map writes`?
9. Buffered vs unbuffered channels: when does each one block?
10. How does `select` let a node wait for "message OR timeout OR shutdown"? What does `default` change?
11. What is backpressure, and why is an unbounded message queue a DoS risk?
12. When would you choose a mutex over a channel, and vice versa?

---

