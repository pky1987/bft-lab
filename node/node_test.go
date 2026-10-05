package node

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pky1987/bft-lab/p2p"
)

//newTestNode returns a node whose handlers report into channels, so tests can wait
// for amessage was handled or a timeout fired.

func newTestNode(inbox <-chan p2p.Message, timeout time.Duration) (*Node, chan p2p.Message, chan struct{}) {
	got := make(chan p2p.Message, 10)
	timeouts := make(chan struct{}, 10)
	n := &Node{
		ID:        "bob",
		Inbox:     inbox,
		Timeout:   timeout,
		onMessage: func(m p2p.Message) { got <- m },
		onTimeout: func() { timeouts <- struct{}{} },
	}
	return n, got, timeouts

}

func TestNodeStopsOnCancel(t *testing.T) {
	n, _, _ := newTestNode(make(<-chan p2p.Message), time.Hour)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() { done <- n.Run(ctx) }() // run blocks, so run it in a goroutine and send its result to done
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err=%v,want %v", err, context.Canceled)
		}
	case <-time.After(time.Second):
		t.Fatalf("Run did not stop after cancel")
	}
}

// A message sent through ChanTransport reaches OnMessage.
func TestNodeHandleMessage(t *testing.T) {
	tr := p2p.NewChanTransport(10)
	tr.Register("bob")
	inbox, _ := tr.Inbox("bob")
	n, got, _ := newTestNode(inbox, time.Hour) // pas inbox,keep got
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = n.Run(ctx) }()
	if err := tr.Send(p2p.Message{From: "alice", To: "bob", Payload: []byte("vote")}); err != nil {
		t.Fatalf("send:%v", err)
	}
	select {
	case m := <-got:
		if string(m.Payload) != "vote" {
			t.Fatalf("got payload %q,want %q", m.Payload, "vote")
		}
	case <-time.After(time.Second):
		t.Fatalf("Run did not stop after cancel")
	}
}

// With no messages, OnTimeout fires, and keeps firing
func TestNodeTimeoutFires(t *testing.T) {
	n, _, timeouts := newTestNode(make(<-chan p2p.Message), 20*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = n.Run(ctx)
	}()
	for i := 0; i < 2; i++ {
		select {
		case <-timeouts:

		case <-time.After(time.Second):
			t.Fatalf("timeout %d didnot fire within 1s", i)
		}
	}

}

// When the context's deadline passes, Run stops by itself and returns context.DeadlineExceeded
func TestNodestopsOnDeadline(t *testing.T) {
	n, _, _ := newTestNode(make(<-chan p2p.Message), time.Hour)

	// a context that cancels ITSELF after 20ms
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := n.Run(ctx) //block for 20 ms then returns
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err=%v,want %v", err, context.DeadlineExceeded)
	}
	t.Logf("Run stopped after %v", time.Since(start).Round(time.Millisecond))
}

// If messages keep arriving more often than the timeout, the timeout never fires.
func TestNodeMessageResetsTimer(t *testing.T) {
	tr := p2p.NewChanTransport(100)
	tr.Register("bob")
	inbox, _ := tr.Inbox("bob")
	n, got, timeouts := newTestNode(inbox, 100*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = n.Run(ctx)
	}()

	//Keep talking every 20 ms for 30ms the 100ms tieouts must never fire
	for i := 0; i < 15; i++ {
		if err := tr.Send(p2p.Message{From: "alice", To: "bob"}); err != nil {
			t.Fatalf("send %d:%v", i, err)
		}
		<-got //wait until nodes has handled it
		time.Sleep(20 * time.Millisecond)
	}
	select {
	case <-timeouts:
		t.Fatalf("timeout fired although message kept arriving")
	default:
		//mailbox empty:no timeout fired
	}
}
