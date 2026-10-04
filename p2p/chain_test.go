package p2p

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestChanTransportInboxBlocking(t *testing.T) {
	tr := NewChanTransport(1)
	tr.Register("bob")
	inbox, ok := tr.Inbox("bob")
	if !ok {
		t.Fatalf("bob has no inbox")
	}

	//alice sends 10ms from now, in another goroutine
	go func() {
		time.Sleep(10 * time.Millisecond)
		_ = tr.Send(Message{From: "alice", To: "bob", Payload: []byte("Prakash")})
	}()

	//bob BLOCKS here(no CPU used) until the message arrives, or gives up after 1s
	select {
	case msg := <-inbox:
		if string(msg.Payload) != "Prakash" {
			t.Fatalf("got %q, want %q", msg.Payload, "Prakash")
		}
	case <-time.After(time.Second):
		t.Fatalf("time out waititng for message")
	}
}

// 3 messages from one sender come out in order, then the inbox is empty.
func TestChanTransportFIFO(t *testing.T) {
	tr := NewChanTransport(10)
	tr.Register("bob")
	// Send 3 message from sender in FIFO order
	for n := 0; n < 3; n++ {
		msg := Message{From: "alice", To: "bob", Payload: []byte{byte(n)}}
		if err := tr.Send(msg); err != nil {
			t.Fatalf("send message %d:%v", n, err)
		}
	}
	// Receive 3 message for bob, and payload in order.
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
		t.Fatal("exepected bob's inbox to be empty after 3 receive")
	}
}

// Sending to, receiving from, or asking for the inbox of an unregistered node all fail.
func TestChanTransportUnknownNode(t *testing.T) {
	tr := NewChanTransport(10)
	tr.Register("bob")
	err := tr.Send(Message{From: "alice", To: "ghost", Payload: []byte("Prakash")})
	if !errors.Is(err, ErrUnknownNode) {
		t.Fatalf("err:%v,want:%v", err, ErrUnknownNode)
	}
	if _, ok := tr.Receive("ghost"); ok {
		t.Fatalf("Receive on unregistered node")
	}
	if _, ok := tr.Inbox("ghost"); ok {
		t.Fatalf("Inbox on unregistered node")
	}
}

// With capacity 2, the first 2 sends succeed and the 3rd returns ErrInboxFull. Registering bob again doesn't wipe his inbox
func TestChanTransportInboxFull(t *testing.T) {
	tr := NewChanTransport(2)
	tr.Register("bob")

	for n := 0; n < 2; n++ {
		msg := Message{From: "alice", To: "bob", Payload: []byte{byte(n)}}
		if err := tr.Send(msg); err != nil {
			t.Fatalf("send message %d:%v", n, err)
		}

	}
	err := tr.Send(Message{From: "alice", To: "bob", Payload: []byte("Prakash")})
	if !errors.Is(err, ErrInboxFull) {
		t.Fatalf("3rd send:err=%v,want=%v", err, ErrInboxFull)
	}
	tr.Register("bob")
	if _, ok := tr.Receive("bob"); !ok {
		t.Fatalf("Bob received the message.")
	}
}

// 10 senders × 100 while bob receives at the same time → bob gets exactly 1,000
// TestChanTransportConcurrent: 10 senders × 100 while bob receives at the same time.
func TestChanTransportConcurrent(t *testing.T) {
	const senders, perSender = 10, 100
	const total = senders * perSender
	tr := NewChanTransport(total) // room for every message, so no send fails from timing
	tr.Register("bob")
	var wg sync.WaitGroup

	// Act 1: 10 senders
	for s := 0; s < senders; s++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			from := NodeID(fmt.Sprintf("validator-%d", id))
			for i := 0; i < perSender; i++ {
				if err := tr.Send(Message{From: from, To: "bob"}); err != nil {
					t.Errorf("send: %v", err)
					return
				}
			}
		}(s)
	}

	// Act 2: bob receives at the same time
	received := 0
	wg.Add(1)
	go func() {
		defer wg.Done()
		for received < total {
			if _, ok := tr.Receive("bob"); ok {
				received++
			}
		}
	}()

	wg.Wait()

	// Assert
	if received != total {
		t.Fatalf("bob received %d, want %d", received, total)
	}
}

// TestChanTransportConcurrentRegister: registering nodes while sending must not race.
func TestChanTransportConcurrentRegister(t *testing.T) {
	tr := NewChanTransport(1000)
	tr.Register("bob")
	var wg sync.WaitGroup
	wg.Add(2)

	// A: writes the map
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			tr.Register(NodeID(fmt.Sprintf("node-%d", i)))
		}
	}()

	// B: reads the map at the same time
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_ = tr.Send(Message{To: "bob"})
		}
	}()

	wg.Wait()
}
