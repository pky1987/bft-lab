package p2p

import (
	"fmt"
	"sync"
	"testing"
)

// TestMemTransportConcurrentSend has 10 validators send to bob at the same time.
func TestMemTransportConcurrentSend(t *testing.T) {
	const senders, perSender = 10, 100
	tr := NewMemTransport()
	tr.Register("bob")

	var wg sync.WaitGroup
	for s := 0; s < senders; s++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			from := NodeID(fmt.Sprintf("validator-%d", id))
			for i := 0; i < perSender; i++ {
				if err := tr.Send(Message{From: from, To: "bob"}); err != nil {
					t.Errorf("send:%v", err)
					return
				}
			}
		}(s)
	}
	wg.Wait()
	got := 0
	for {
		if _, ok := tr.Receive("bob"); !ok {
			break
		}
		got++
	}
	if got != senders*perSender {
		t.Fatalf("bob received %d messages,want %d (messages were lost)", got, senders*perSender)
	}

}

func TestMemTransportConcurrentReceive(t *testing.T) {
	const senders, perSender = 10, 100
	const total = senders * perSender
	tr := NewMemTransport()
	tr.Register("bob")

	var wg sync.WaitGroup
	for n := 0; n < senders; n++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			from := NodeID(fmt.Sprintf("validator-%d", id))
			for i := 0; i < perSender; i++ {
				if err := tr.Send(Message{From: from, To: "bob"}); err != nil {
					t.Errorf("send:%v", err)
					return
				}
			}
		}(n)
	}
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
	if received != total {
		t.Fatalf("bob received %d,want %d", received, total)
	}

}

func TestLossyConcurrentSend(t *testing.T) {
	const senders, perSender = 10, 100
	tr := NewMemTransport()
	tr.Register("bob")
	lossy := NewLossyTransport(tr, 0.3, 42)

	var wg sync.WaitGroup
	for n := 0; n < senders; n++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			from := NodeID(fmt.Sprintf("validator-%d", id))
			for i := 0; i < perSender; i++ {
				if err := lossy.Send(Message{From: from, To: "bob"}); err != nil {
					t.Errorf("send:%v", err)
					return
				}
			}
		}(n)
	}
	wg.Wait()
	got := 0
	for {
		if _, ok := tr.Receive("bob"); !ok {
			break
		}
		got++

	}
	if got+lossy.Dropped() != senders*perSender {
		t.Fatalf("bob received %d messages,want %d (messages were lost)", got, senders*perSender)
	}

}
