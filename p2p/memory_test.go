package p2p

import (
	"errors"
	"testing"
)

func TestMemTransportFIFO(t *testing.T) {
	tr := NewMemTransport()
	tr.Register("alice")
	tr.Register("bob")

	err := tr.Send(Message{From: "alice", To: "ghost", Payload: []byte("hi")})
	if !errors.Is(err, ErrUnknownNode) {
		t.Fatalf("err=%v,want %v", err, ErrUnknownNode)
	}
	// Send 3 messages from alice to bob with Payload.
	for i := 0; i < 3; i++ {
		msg := Message{From: "alice", To: "bob", Payload: []byte{byte(i)}}
		if err := tr.Send(msg); err != nil {
			t.Fatalf("send message %d:%v", i, err)
		}
	}
	// Receive 3 messages for bob, check payload and order.
	for i := 0; i < 3; i++ {
		msg, ok := tr.Receive("bob")
		if !ok {
			t.Fatalf("bob expected message %d, got none", i)
		}
		if msg.From != "alice" || msg.To != "bob" || len(msg.Payload) != 1 || msg.Payload[0] != byte(i) {
			t.Fatalf("bob expected message %d from alice, got %+v", i, msg)
		}
	}
	if _, ok := tr.Receive("bob"); ok {
		t.Fatalf("expected bob's inbox to be empty after 3 receives")
	}
}

func TestMemTransportEmptyInbox(t *testing.T) {
	tr := NewMemTransport()
	tr.Register("bob")
	if _, ok := tr.Receive("bob"); ok {
		t.Fatalf("expected bob's inbox to be empty")
	}
	if _, ok := tr.Receive("ghost"); ok {
		t.Fatalf("expected ghost's inbox to be empty")
	}
}

func TestMemTransportIsolation(t *testing.T) {
	tr := NewMemTransport()
	tr.Register("alice")
	tr.Register("bob")
	msg := Message{From: "alice", To: "bob", Payload: []byte("hello")}
	if err := tr.Send(msg); err != nil {
		t.Fatalf("send message: %v", err)
	}
	_, ok := tr.Receive("alice")
	if ok {
		t.Fatalf("alice should not receive any message")
	}
	msg, ok = tr.Receive("bob")
	if !ok {
		t.Fatalf("bob expected message, got none")
	}
	if string(msg.Payload) != "hello" {
		t.Fatalf("bob expected message with payload 'hello', got %+v", msg)
	}
}
