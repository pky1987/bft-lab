package node

//Package node runs a validator's event loop: one goroutine that waits for message,
// timeouts and shotdown, and handles them one at the time.

import (
	"context"
	"time"

	"github.com/pky1987/bft-lab/p2p"
)

// Node is a single validator's event loop

type Node struct {
	ID        p2p.NodeID
	Inbox     <-chan p2p.Message // where messages arrive (from ChanTransport.Inbox)
	Timeout   time.Duration      // how long to wait for a message before OnTimeout fires
	onMessage func(p2p.Message)  //called for every message,on the loop goroutines.
	onTimeout func()             //called when Timeout passes with no message
}

// Run handles events until ctx is cancelled, then return ctx.Err().
// Only the Run goroutine calls onMessage and onTimeout, so they need no locks.

func (n *Node) Run(ctx context.Context) error {
	timer := time.NewTimer(n.Timeout)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg := <-n.Inbox:
			n.onMessage(msg)
			timer.Reset(n.Timeout)
		case <-timer.C:
			n.onTimeout()
			timer.Reset(n.Timeout)
		}
	}
}
