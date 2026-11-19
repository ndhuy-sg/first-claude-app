package events

import (
	"testing"

	"github.com/ndhuy-sg/first-claude-app/api/internal/platform"
)

func recv(t *testing.T, ch <-chan platform.Event) (platform.Event, bool) {
	t.Helper()
	select {
	case e := <-ch:
		return e, true
	default:
		return platform.Event{}, false
	}
}

func TestHub_BranchRouting(t *testing.T) {
	h := NewHub()
	cn1, cn2 := int64(1), int64(2)
	a, ca := h.Subscribe(&cn1)
	defer ca()
	b, cb := h.Subscribe(&cn2)
	defer cb()
	all, call := h.Subscribe(nil) // owner
	defer call()

	h.Publish(platform.Event{Type: "order.updated", BranchID: 1})
	if e, ok := recv(t, a); !ok || e.Type != "order.updated" {
		t.Fatal("CN1 subscriber did not get the CN1 event")
	}
	if _, ok := recv(t, b); ok {
		t.Fatal("CN2 subscriber got a CN1 event")
	}
	if _, ok := recv(t, all); !ok {
		t.Fatal("chain subscriber did not get the CN1 event")
	}

	h.Publish(platform.Event{Type: "product.sold_out"}) // BranchID 0 = everyone
	for i, ch := range []<-chan platform.Event{a, b, all} {
		if _, ok := recv(t, ch); !ok {
			t.Fatalf("subscriber %d missed the broadcast", i)
		}
	}
}

func TestHub_SlowSubscriberDoesNotBlock(t *testing.T) {
	h := NewHub()
	h.buffer = 2
	ch, cancel := h.Subscribe(nil)
	defer cancel()
	for i := 0; i < 10; i++ { // must not block although nobody reads
		h.Publish(platform.Event{Type: "order.updated", BranchID: 1})
	}
	if len(ch) != 2 || h.Dropped() != 8 {
		t.Fatalf("buffered %d, dropped %d", len(ch), h.Dropped())
	}
}

func TestHub_UnsubscribeAndClose(t *testing.T) {
	h := NewHub()
	_, c1 := h.Subscribe(nil)
	_, c2 := h.Subscribe(nil)
	if h.Subscribers() != 2 {
		t.Fatalf("subscribers = %d", h.Subscribers())
	}
	c1()
	c1() // idempotent
	if h.Subscribers() != 1 {
		t.Fatalf("subscribers = %d", h.Subscribers())
	}
	c2()
	h.Publish(platform.Event{Type: "x"})
	h.Close()
	select {
	case <-h.Done():
	default:
		t.Fatal("Done not closed")
	}
	h.Close() // idempotent
}

func TestFind(t *testing.T) {
	h := NewHub()
	rec := &platform.RecordingPublisher{}
	if Find(h) != h || Find(Fanout{rec, h}) != h || Find(rec) != nil {
		t.Fatal("Find")
	}
	Fanout{rec, h}.Publish(platform.Event{Type: "x"})
	if len(rec.Events) != 1 {
		t.Fatal("fanout did not reach recorder")
	}
}
