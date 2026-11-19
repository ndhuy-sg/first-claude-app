package events_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ndhuy-sg/first-claude-app/api/internal/auth"
	"github.com/ndhuy-sg/first-claude-app/api/internal/events"
	"github.com/ndhuy-sg/first-claude-app/api/internal/platform"
	"github.com/ndhuy-sg/first-claude-app/api/internal/testutil"
)

type stream struct {
	t      *testing.T
	res    *http.Response
	lines  chan string
	cancel context.CancelFunc
}

// open connects to GET /events with the client's cookies and waits for the
// ": connected" comment (the subscription exists from then on).
func open(t *testing.T, env *testutil.Env, c *testutil.Client) *stream {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, env.Server.URL+"/api/v1/events", nil)
	for _, name := range []string{auth.SessionCookie, auth.DeviceCookie} {
		if v := c.Cookie(name); v != "" {
			req.AddCookie(&http.Cookie{Name: name, Value: v})
		}
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	s := &stream{t: t, res: res, lines: make(chan string, 100), cancel: cancel}
	t.Cleanup(s.close)
	if res.StatusCode != 200 {
		t.Fatalf("status %d", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content-type %q", ct)
	}
	go func() {
		sc := bufio.NewScanner(res.Body)
		for sc.Scan() {
			s.lines <- sc.Text()
		}
		close(s.lines)
	}()
	if l := s.next(2 * time.Second); l != ": connected" {
		t.Fatalf("first line %q", l)
	}
	s.next(time.Second) // blank line ending the comment
	return s
}

func (s *stream) close() { s.cancel(); s.res.Body.Close() }

func (s *stream) next(timeout time.Duration) string {
	select {
	case l, ok := <-s.lines:
		if !ok {
			return "<eof>"
		}
		return l
	case <-time.After(timeout):
		return "<timeout>"
	}
}

// event reads one "event:/data:" block.
func (s *stream) event(timeout time.Duration) (string, platform.Event, bool) {
	var name string
	var ev platform.Event
	for {
		l := s.next(timeout)
		switch {
		case l == "<timeout>" || l == "<eof>":
			return "", ev, false
		case strings.HasPrefix(l, "event: "):
			name = strings.TrimPrefix(l, "event: ")
		case strings.HasPrefix(l, "data: "):
			if err := json.Unmarshal([]byte(strings.TrimPrefix(l, "data: ")), &ev); err != nil {
				s.t.Fatalf("data: %v", err)
			}
		case l == "" && name != "":
			return name, ev, true
		}
	}
}

func TestEvents_ReceivePublishedEvent(t *testing.T) {
	env := testutil.NewEnv(t)
	s := open(t, env, env.Tn1())
	cn1 := env.Fx.Branches[1]
	env.Deps.Events.Publish(platform.Event{Type: "product.sold_out", BranchID: cn1, Data: map[string]any{"productId": 7}})
	name, ev, ok := s.event(2 * time.Second)
	if !ok || name != "product.sold_out" || ev.Type != "product.sold_out" || ev.BranchID != cn1 {
		t.Fatalf("got %q %+v %v", name, ev, ok)
	}
	if d, _ := ev.Data.(map[string]any); d["productId"] != float64(7) {
		t.Fatalf("data = %+v", ev.Data)
	}
	// the recording publisher of the test env still sees events
	if len(env.Events.Events) != 1 {
		t.Fatalf("recorder = %+v", env.Events.Events)
	}
}

func TestEvents_BranchIsolation(t *testing.T) {
	env := testutil.NewEnv(t)
	cn1Stream := open(t, env, env.Tn1())
	cn2Stream := open(t, env, env.Tn2())
	ownerStream := open(t, env, env.Owner())

	env.Deps.Events.Publish(platform.Event{Type: "order.updated", BranchID: env.Fx.Branches[1]})
	env.Deps.Events.Publish(platform.Event{Type: "user.locked", BranchID: env.Fx.Branches[2]})
	env.Deps.Events.Publish(platform.Event{Type: "transfer.shipped"}) // broadcast

	want := map[*stream][]string{
		cn1Stream:   {"order.updated", "transfer.shipped"},
		cn2Stream:   {"user.locked", "transfer.shipped"},
		ownerStream: {"order.updated", "user.locked", "transfer.shipped"},
	}
	for s, names := range want {
		for _, n := range names {
			got, _, ok := s.event(2 * time.Second)
			if !ok || got != n {
				t.Fatalf("got %q (%v), want %q", got, ok, n)
			}
		}
		if got, _, ok := s.event(150 * time.Millisecond); ok {
			t.Fatalf("unexpected extra event %q", got)
		}
	}
}

func TestEvents_HeartbeatAndCleanup(t *testing.T) {
	env := testutil.NewEnv(t)
	hub := events.Find(env.Deps.Events)
	if hub == nil {
		t.Fatal("no hub wired")
	}
	hub.SetHeartbeat(50 * time.Millisecond)
	s := open(t, env, env.Tn1())
	if l := s.next(2 * time.Second); l != ": heartbeat" {
		t.Fatalf("heartbeat line %q", l)
	}
	if hub.Subscribers() != 1 {
		t.Fatalf("subscribers = %d", hub.Subscribers())
	}
	s.close()
	deadline := time.Now().Add(2 * time.Second)
	for hub.Subscribers() != 0 {
		if time.Now().After(deadline) {
			t.Fatal("subscriber not removed after disconnect")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestEvents_RequiresAuth(t *testing.T) {
	env := testutil.NewEnv(t)
	env.Anon().Get("/api/v1/events").MustStatus(t, 401)
}
