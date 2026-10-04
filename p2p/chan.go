package p2p

import (
	"errors"
	"sync"
)

//ErrInboxFull is returned when the reciever's inbox has no free space.

var ErrInboxFull = errors.New("inbox full")

//ChanTransport is a transport where every node's inbox is a buffered channel.
// It is safe for concurrent use, and event loops can wait on Inbox with select.

type ChanTransport struct {
	mu      sync.Mutex //guards the inboxes map
	inboxes map[NodeID]chan Message
	size    int //capacity of each inbox
}

// Compile-time check: ChanTransport must satisfy Transport

var _ Transport = (*ChanTransport)(nil)

// NewChanTransport returns a network whose inboxes hold up to size messages.
func NewChanTransport(size int) *ChanTransport {
	return &ChanTransport{inboxes: make(map[NodeID]chan Message), size: size}
}

// Register creates a buffered inbox for id.
func (c *ChanTransport) Register(id NodeID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.inboxes[id]; !ok {
		c.inboxes[id] = make(chan Message, c.size)
	}
}

// Inbox looks up id's channel under a read lock.
func (c *ChanTransport) inbox(id NodeID) (chan Message, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ch, ok := c.inboxes[id]
	return ch, ok
}

//Send puts msg into the receiver's inbox without blocking.
// A full inbox returns ErrInboxFull(backpressure) instead of waiting.

func (c *ChanTransport) Send(msg Message) error {
	ch, ok := c.inbox(msg.To)
	if !ok {
		return ErrUnknownNode
	}
	select {
	case ch <- msg:
		return nil
	default:
		return ErrInboxFull
	}

}

// Receive takes the next message for id without blocking.
func (c *ChanTransport) Receive(id NodeID) (Message, bool) {
	ch, ok := c.inbox(id)
	if !ok {
		return Message{}, false
	}
	select {
	case msg := <-ch:
		return msg, true
	default:
		return Message{}, false
	}
}

// Inbox returns id's inbox as a receive-only channel, so an event loop can
// block on it with select.
func (c *ChanTransport) Inbox(id NodeID) (<-chan Message, bool) {
	return c.inbox(id)
}
