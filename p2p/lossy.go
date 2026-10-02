package p2p

import (
	"math/rand/v2"
)

// LossyTransport wraps another Transport and randomly drops send messages,
// simulating an unrealiabe network. The RNG is seeded, so the same seed always drop
// the same messages, which makes failures replayable.

type LossyTransport struct {
	inner    Transport
	dropRate float64 // 0.0 means no drops, 1.0 means all drops.
	rng      *rand.Rand
	dropped  int
}

// Compile-time Check: the build fails if LossyTransport stops satisfying Transport.
var _ Transport = (*LossyTransport)(nil)

// NewLossyTransport wraps inner. DropRate is the probability(0.0-1.0) that any single message is lost.
func NewLossyTransport(inner Transport, dropRate float64, seed uint64) *LossyTransport {
	return &LossyTransport{
		inner:    inner,
		dropRate: dropRate,
		rng:      rand.New(rand.NewPCG(seed, seed)),
	}
}

// Send either drop msg ( returing nil,like a real network that loses a packet silently
// or forwads it to the wrapped transport.)
func (l *LossyTransport) Send(msg Message) error {
	if l.rng.Float64() < l.dropRate {
		l.dropped++
		return nil
	}
	return l.inner.Send(msg)
}

// Receive is passed  straight through: loss happens on the way in.
func (l *LossyTransport) Receive(id NodeID) (Message, bool) {
	return l.inner.Receive(id)
}

// Dropped reports how many messages have been lost so far.
func (l *LossyTransport) Dropped() int {
	return l.dropped
}
