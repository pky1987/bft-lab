package p2p

import (
	"testing"
)

// Delivered sends n messages through a 50% lossy network with given seed and returns
// which message number arrived.

func delivered(t *testing.T, seed uint64, n int) []byte {
	t.Helper()
	mem := NewMemTransport()
	mem.Register("bob")
	lossy := NewLossyTransport(mem, 0.5, seed)
	for i := 0; i < n; i++ {
		if err := lossy.Send(Message{From: "alice", To: "bob", Payload: []byte{byte(i)}}); err != nil {
			t.Fatalf("Send %d:%v", i, err)
		}
	}
	var got []byte
	for {
		msg, ok := lossy.Receive("bob")
		if !ok {
			return got
		}
		got = append(got, msg.Payload[0])
	}
}
func TestLossyDeterministic(t *testing.T) {
	a := delivered(t, 42, 100)
	b := delivered(t, 42, 100)
	c := delivered(t, 7, 100)
	if string(a) != string(b) {
		t.Fatalf("same seed, different results: %v vs %v", a, b)
	}
	if string(a) == string(c) {
		t.Fatalf("different seeds, same results: %v vs %v", a, c)
	}
	t.Logf("seed 42 delivered %d/100, seed 7 delivered %d/100", len(a), len(c))
}

func TestLossyNeverDrops(t *testing.T) {
	mem := NewMemTransport()
	mem.Register("bob")
	lossy := NewLossyTransport(mem, 0.0, 42)
	for i := 0; i < 100; i++ {
		if err := lossy.Send(Message{From: "alice", To: "bob", Payload: []byte{byte(i)}}); err != nil {
			t.Fatalf("Send %d:%v", i, err)
		}
	}
	for i := 0; i < 100; i++ {
		msg, ok := lossy.Receive("bob")
		if !ok {
			t.Fatalf("bob expected message %d, got none", i)
		}
		if msg.Payload[0] != byte(i) {
			t.Fatalf("message %d out of order: got %d", i, msg.Payload[0])
		}
	}
	t.Logf("Never-dropping transport delivered all 100 messages")
	lossyDropped := lossy.Dropped()
	if lossyDropped != 0 {
		t.Fatalf("Never-dropping transport dropped %d messages", lossyDropped)
	}
}

func TestLossyDropEverything(t *testing.T) {
	mem := NewMemTransport()
	mem.Register("bob")
	lossy := NewLossyTransport(mem, 1.0, 42)
	for i := 0; i < 100; i++ {
		if err := lossy.Send(Message{From: "alice", To: "bob", Payload: []byte{byte(i)}}); err != nil {
			t.Fatalf("Send %d:%v", i, err)
		}
	}
	if _, ok := lossy.Receive("bob"); ok {
		t.Fatalf("Expected no messages, but got one")
	}
	if got := lossy.Dropped(); got != 100 {
		t.Fatalf("expected 100 dropped messages, got %d", got)
	}
}

func TestLossyRoughRate(t *testing.T) {
	mem := NewMemTransport()
	mem.Register("bob")
	lossy := NewLossyTransport(mem, 0.3, 42)
	for i := 0; i < 10000; i++ {
		err := lossy.Send(Message{From: "alice", To: "bob", Payload: []byte{byte(i)}})
		if err != nil {
			t.Fatalf("Send %d:%v", i, err)
		}
	}
	if got := lossy.Dropped(); got < 2700 || got > 3300 {
		t.Fatalf("dropped %d of 10000, want 2700–3300", got)
	}
}
