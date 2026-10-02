// Packages p2p defines how nodes exchange messages, independent of the underlying
// network (in memory for tests, TCP for real nodes, etc).
package p2p

import (
	"errors"
)

// NodeID identifies a node (validator) on the network.
type NodeID string

// Message is one unit of communication between two nodes.
type Message struct {
	From    NodeID
	To      NodeID
	Payload []byte //encoded vote, proposal, etc. Transport never look inside.
}

// ErrUnknownNode is returned when sending to a node that is not registered.
var ErrUnknownNode = errors.New("unknown node")

// Transport moves messages between nodes. Consensus code depends only on this interface,
// so same code runs on every kind of network.
type Transport interface {
	// Send delivers msg to msg.To's inbox.
	Send(msg Message) error
	// Receive return the next message for id, and false if none is waiting.
	Receive(id NodeID) (Message, bool)
}
