package p2p

//MemTransport is an in-memory Transport. Messages are delivered instantly.
type MemTransport struct {
	inboxes map[NodeID][]Message
}

// Compile-time check: the build fails if MemTransport stops satisfying Transport.
var _ Transport = (*MemTransport)(nil)

// NewMemTransport returns a network with no nodes.
func NewMemTransport() *MemTransport {
	return &MemTransport{inboxes: make(map[NodeID][]Message)}
}

// Register adds a node so it can receive messages.
func (m *MemTransport) Register(id NodeID) {
	if _, ok := m.inboxes[id]; !ok {
		m.inboxes[id] = nil
	}
}

// Send appends msg to receiver's inbox.
func (m *MemTransport) Send(msg Message) error {
	inbox, ok := m.inboxes[msg.To]
	if !ok {
		return ErrUnknownNode
	}
	m.inboxes[msg.To] = append(inbox, msg)
	return nil
}

// Receive removes and returns the oldest message for id.
func (m *MemTransport) Receive(id NodeID) (Message, bool) {
	inbox := m.inboxes[id]
	if len(inbox) == 0 {
		return Message{}, false
	}
	msg := inbox[0]
	m.inboxes[id] = inbox[1:]
	return msg, true
}
