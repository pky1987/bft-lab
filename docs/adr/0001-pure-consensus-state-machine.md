# ADR 0001: Consensus is a pure state machine

**Date:** 2026-11-01 · **Status:** Accepted

## Context

Consensus has to react to messages from a network and, from Lesson 15, to timeouts. We want to test it under
thousands of message orders and reproduce any failure from a seed (Lessons 19–21).

## Options

1. **Goroutines inside consensus.** Each node runs its own loop and reads from `p2p` directly. It looks closer
   to production, but tests depend on the scheduler and failures can't be replayed.
2. **A pure state machine.** `Handle(message)` returns the messages to broadcast. There's no clock, network or
   goroutine inside. A separate driver feeds it.

## Decision

Option 2. `consensus.State` does no I/O. A test harness drives it today, the simulator in Lesson 19, and the
`node` event loop with a real transport in Lesson 23. From Lesson 15, timeouts reach it as inputs too.

## Consequences

- **Good:** any message order can be tested, and one seed reproduces one run. No locks are needed inside `State`.
- **Good:** the same code runs under the simulator and on a real network.
- **Cost:** a driver has to exist, and a bug in it isn't caught by `State`'s tests.
- **Cost:** the Lesson 14 code doesn't run on a real network yet. That arrives in Lesson 23.
