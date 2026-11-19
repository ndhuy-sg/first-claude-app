// Package events is the in-memory Server-Sent Events hub (GET /events).
//
// The Hub implements platform.Publisher. Each connected client subscribes for
// its branch (branch-bound users) or for every branch (owner, warehouse
// manager). Events with BranchID 0 go to everyone. Publish never blocks: each
// subscriber has a bounded buffer and events for a slow subscriber are dropped
// (the client re-fetches state when it reconnects or on the next event).
//
// Phase 1 limitation: the hub lives in the `serve` process only. Events
// published by the separate `worker` process are not delivered to clients
// (cross-process delivery, e.g. via PostgreSQL LISTEN/NOTIFY, is out of scope).
package events

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ndhuy-sg/first-claude-app/api/internal/auth"
	"github.com/ndhuy-sg/first-claude-app/api/internal/platform"
)

const (
	DefaultHeartbeat = 25 * time.Second
	DefaultBuffer    = 64
)

// Hub fans events out to SSE subscribers.
type Hub struct {
	mu        sync.RWMutex
	subs      map[*subscriber]struct{}
	buffer    int
	heartbeat time.Duration
	dropped   atomic.Int64
	done      chan struct{}
	closeOnce sync.Once
}

type subscriber struct {
	branch *int64 // nil = every branch
	ch     chan platform.Event
}

func NewHub() *Hub {
	return &Hub{subs: map[*subscriber]struct{}{}, buffer: DefaultBuffer, heartbeat: DefaultHeartbeat, done: make(chan struct{})}
}

// Publish delivers e to matching subscribers without blocking.
func (h *Hub) Publish(e platform.Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for s := range h.subs {
		if e.BranchID != 0 && s.branch != nil && *s.branch != e.BranchID {
			continue
		}
		select {
		case s.ch <- e:
		default:
			h.dropped.Add(1)
		}
	}
}

// Subscribe registers a subscriber for branch (nil = all) and returns its
// channel and an idempotent cancel function.
func (h *Hub) Subscribe(branch *int64) (<-chan platform.Event, func()) {
	h.mu.Lock()
	s := &subscriber{branch: branch, ch: make(chan platform.Event, h.buffer)}
	h.subs[s] = struct{}{}
	h.mu.Unlock()
	var once sync.Once
	return s.ch, func() {
		once.Do(func() {
			h.mu.Lock()
			delete(h.subs, s)
			h.mu.Unlock()
		})
	}
}

// Subscribers returns the number of connected subscribers.
func (h *Hub) Subscribers() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subs)
}

// Dropped returns how many events were dropped for slow subscribers.
func (h *Hub) Dropped() int64 { return h.dropped.Load() }

// SetHeartbeat changes the heartbeat interval for streams opened afterwards.
func (h *Hub) SetHeartbeat(d time.Duration) {
	h.mu.Lock()
	h.heartbeat = d
	h.mu.Unlock()
}

// Close ends every open stream (used on server shutdown so it is not held up by SSE clients).
func (h *Hub) Close() { h.closeOnce.Do(func() { close(h.done) }) }

// Done is closed after Close.
func (h *Hub) Done() <-chan struct{} { return h.done }

// Fanout publishes to several publishers (e.g. a test recorder and the hub).
type Fanout []platform.Publisher

func (f Fanout) Publish(e platform.Event) {
	for _, p := range f {
		p.Publish(e)
	}
}

// Find returns the Hub inside p (p itself or a member of a Fanout), or nil.
func Find(p platform.Publisher) *Hub {
	switch v := p.(type) {
	case *Hub:
		return v
	case Fanout:
		for _, x := range v {
			if h := Find(x); h != nil {
				return h
			}
		}
	}
	return nil
}

// Register mounts GET /events. When d.Events is not (or does not contain) a
// Hub — as in tests, which use a RecordingPublisher — a new Hub is created and
// d.Events becomes Fanout{previous, hub}, so register this module before
// modules that might copy d.Events at registration time.
func Register(g *gin.RouterGroup, d *platform.Deps) {
	hub := Find(d.Events)
	if hub == nil {
		hub = NewHub()
		if d.Events == nil {
			d.Events = hub
		} else {
			d.Events = Fanout{d.Events, hub}
		}
	}
	g.GET("/events", auth.RequireAuth(), hub.serve)
}

func (h *Hub) serve(c *gin.Context) {
	p := auth.From(c)
	ch, cancel := h.Subscribe(p.BranchID)
	defer cancel()
	h.mu.RLock()
	beat := h.heartbeat
	h.mu.RUnlock()

	w := c.Writer
	hd := w.Header()
	hd.Set("Content-Type", "text/event-stream; charset=utf-8")
	hd.Set("Cache-Control", "no-cache, no-transform")
	hd.Set("Connection", "keep-alive")
	hd.Set("X-Accel-Buffering", "no") // disable proxy buffering (nginx)
	w.WriteHeader(http.StatusOK)
	if _, err := fmt.Fprint(w, ": connected\n\n"); err != nil {
		return
	}
	w.Flush()

	ticker := time.NewTicker(beat)
	defer ticker.Stop()
	ctx := c.Request.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-h.done:
			return
		case <-ticker.C:
			if _, err := fmt.Fprint(w, ": heartbeat\n\n"); err != nil {
				return
			}
			w.Flush()
		case e := <-ch:
			b, err := json.Marshal(e)
			if err != nil {
				continue
			}
			name := strings.NewReplacer("\n", "", "\r", "").Replace(e.Type)
			if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, b); err != nil {
				return
			}
			w.Flush()
		}
	}
}
