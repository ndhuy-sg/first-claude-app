// Package testutil gives tests a fresh, migrated and seeded PostgreSQL
// database plus an HTTP client for the full API.
//
// Requires TEST_DATABASE_URL pointing at a PostgreSQL server where the user
// may create databases (default postgres://postgres@localhost:54329/postgres).
// Locally: `scripts/test-db.sh start`. In CI: a postgres service container.
//
// IMPORTANT: HTTP-level tests must live in an external test package
// (package orders_test, not package orders), because testutil imports app,
// which imports every module.
package testutil

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"golang.org/x/crypto/bcrypt"

	"github.com/ndhuy-sg/first-claude-app/api/internal/app"
	"github.com/ndhuy-sg/first-claude-app/api/internal/auth"
	"github.com/ndhuy-sg/first-claude-app/api/internal/config"
	"github.com/ndhuy-sg/first-claude-app/api/internal/platform"
	"github.com/ndhuy-sg/first-claude-app/api/internal/seed"
	"github.com/ndhuy-sg/first-claude-app/api/internal/store"
	"github.com/ndhuy-sg/first-claude-app/api/migrations"
)

func AdminURL() string {
	if u := os.Getenv("TEST_DATABASE_URL"); u != "" {
		return u
	}
	return "postgres://postgres@localhost:54329/postgres?sslmode=disable"
}

func withDB(admin, db string) string {
	// replace the path component of the URL
	i := strings.Index(admin, "://")
	rest := admin[i+3:]
	slash := strings.Index(rest, "/")
	q := ""
	if k := strings.Index(rest, "?"); k >= 0 {
		q = rest[k:]
	}
	return admin[:i+3] + rest[:slash] + "/" + db + q
}

var (
	tmplOnce sync.Once
	tmplName string
	tmplErr  error
)

// template creates (once per migration set) a migrated template database.
func template(ctx context.Context) (string, error) {
	tmplOnce.Do(func() {
		h := sha256.New()
		_ = fs.WalkDir(migrations.FS, ".", func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				b, _ := migrations.FS.ReadFile(p)
				h.Write([]byte(p))
				h.Write(b)
			}
			return nil
		})
		tmplName = "mocpos_tmpl_" + hex.EncodeToString(h.Sum(nil))[:12]
		admin, err := pgx.Connect(ctx, AdminURL())
		if err != nil {
			tmplErr = fmt.Errorf("connect TEST_DATABASE_URL (run scripts/test-db.sh start): %w", err)
			return
		}
		defer admin.Close(ctx)
		if _, err = admin.Exec(ctx, "SELECT pg_advisory_lock(424242)"); err != nil {
			tmplErr = err
			return
		}
		defer admin.Exec(ctx, "SELECT pg_advisory_unlock(424242)")
		var exists bool
		if err = admin.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)", tmplName).Scan(&exists); err != nil {
			tmplErr = err
			return
		}
		if exists {
			return
		}
		if _, err = admin.Exec(ctx, "CREATE DATABASE "+tmplName+"_building"); err != nil {
			tmplErr = err
			return
		}
		db, err := sql.Open("pgx", withDB(AdminURL(), tmplName+"_building"))
		if err != nil {
			tmplErr = err
			return
		}
		goose.SetBaseFS(migrations.FS)
		goose.SetLogger(goose.NopLogger())
		_ = goose.SetDialect("postgres")
		if err = goose.Up(db, "."); err != nil {
			db.Close()
			tmplErr = err
			return
		}
		db.Close()
		_, tmplErr = admin.Exec(ctx, "ALTER DATABASE "+tmplName+"_building RENAME TO "+tmplName)
	})
	return tmplName, tmplErr
}

// NewDB returns a pool on a fresh, migrated, empty database that is dropped after the test.
func NewDB(t testing.TB) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	tmpl, err := template(ctx)
	if err != nil {
		t.Fatalf("test database: %v", err)
	}
	name := fmt.Sprintf("t_%d_%d", time.Now().UnixNano(), os.Getpid())
	admin, err := pgx.Connect(ctx, AdminURL())
	if err != nil {
		t.Fatalf("connect admin: %v", err)
	}
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name+" TEMPLATE "+tmpl); err != nil {
		admin.Close(ctx)
		t.Fatalf("create db: %v", err)
	}
	admin.Close(ctx)
	pool, err := pgxpool.New(ctx, withDB(AdminURL(), name))
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		a, err := pgx.Connect(context.Background(), AdminURL())
		if err == nil {
			_, _ = a.Exec(context.Background(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
			a.Close(context.Background())
		}
	})
	return pool
}

// NewSeeded returns a fresh database loaded with seed data and its fixtures.
func NewSeeded(t testing.TB) (*pgxpool.Pool, *seed.Fixtures) {
	t.Helper()
	pool := NewDB(t)
	fx, err := seed.Seed(context.Background(), pool, seed.Options{BcryptCost: bcrypt.MinCost})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	return pool, fx
}

// DefaultNow is the default test clock: 19/11/2026 07:00 in Vietnam.
var DefaultNow = time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC)

// Env is a running API server for one test.
type Env struct {
	T       testing.TB
	Pool    *pgxpool.Pool
	Fx      *seed.Fixtures
	Server  *httptest.Server
	Clock   *platform.FixedClock
	Events  *platform.RecordingPublisher
	Deps    *platform.Deps
	Queries *store.Queries
}

// NewEnv starts the full API on a fresh seeded database.
func NewEnv(t testing.TB) *Env {
	t.Helper()
	pool, fx := NewSeeded(t)
	clock := &platform.FixedClock{T: DefaultNow}
	ev := &platform.RecordingPublisher{}
	d := &platform.Deps{
		Pool:   pool,
		Cfg:    config.Config{CookieSecure: false, AllowedOrigins: []string{"http://localhost:3000"}, PublicBaseURL: "http://localhost:3000"},
		Clock:  clock,
		Events: ev,
	}
	srv := httptest.NewServer(app.NewRouter(d))
	t.Cleanup(srv.Close)
	return &Env{T: t, Pool: pool, Fx: fx, Server: srv, Clock: clock, Events: ev, Deps: d, Queries: store.New(pool)}
}

// Client is an HTTP client with its own cookies, like one browser/tablet.
type Client struct {
	env     *Env
	cookies map[string]string
	Origin  string // sent as Origin header on writes; defaults to an allowed origin
}

// Anon returns a client without any session.
func (e *Env) Anon() *Client {
	return &Client{env: e, cookies: map[string]string{}, Origin: "http://localhost:3000"}
}

// As returns a client logged in as userID with a password session (no device).
func (e *Env) As(userID int64) *Client {
	c := e.Anon()
	raw, err := auth.CreateSession(context.Background(), e.Queries, userID, nil, "password", e.Clock.Now())
	if err != nil {
		e.T.Fatalf("create session: %v", err)
	}
	c.cookies[auth.SessionCookie] = raw
	return c
}

// AsOnDevice returns a client on a POS device (raw device token) with a PIN session for userID.
func (e *Env) AsOnDevice(userID, deviceID int64, deviceToken string) *Client {
	c := e.Anon()
	c.cookies[auth.DeviceCookie] = deviceToken
	raw, err := auth.CreateSession(context.Background(), e.Queries, userID, &deviceID, "pin", e.Clock.Now())
	if err != nil {
		e.T.Fatalf("create session: %v", err)
	}
	c.cookies[auth.SessionCookie] = raw
	return c
}

// Tn1 is the CN1 cashier on the CN1 iPad; Tn2 the CN2 cashier on the CN2 iPad.
func (e *Env) Tn1() *Client   { return e.AsOnDevice(e.Fx.Users.Tn1, e.Fx.DeviceCN1, e.Fx.DeviceCN1Token) }
func (e *Env) Tn2() *Client   { return e.AsOnDevice(e.Fx.Users.Tn2, e.Fx.DeviceCN2, e.Fx.DeviceCN2Token) }
func (e *Env) Ql1() *Client   { return e.AsOnDevice(e.Fx.Users.Ql1, e.Fx.DeviceCN1, e.Fx.DeviceCN1Token) }
func (e *Env) Owner() *Client { return e.As(e.Fx.Users.Owner) }

// SetCookie sets a raw cookie on the client.
func (c *Client) SetCookie(name, value string) { c.cookies[name] = value }

// Cookie returns a cookie value the server set on this client.
func (c *Client) Cookie(name string) string { return c.cookies[name] }

// Resp is a decoded response.
type Resp struct {
	Status int
	Body   []byte
	Header http.Header
}

// JSON decodes the body into v (fails the test on error).
func (r *Resp) JSON(t testing.TB, v any) {
	t.Helper()
	if err := json.Unmarshal(r.Body, v); err != nil {
		t.Fatalf("decode %s: %v", string(r.Body), err)
	}
}

// ErrCode returns error.code of an error body, or "".
func (r *Resp) ErrCode() string {
	var b struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(r.Body, &b)
	return b.Error.Code
}

// Do sends a request. body (if not nil) is JSON-encoded. Extra headers as key, value pairs.
func (c *Client) Do(method, path string, body any, headers ...string) *Resp {
	t := c.env.T
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.env.Server.URL+path, rdr)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if method != http.MethodGet && c.Origin != "" {
		req.Header.Set("Origin", c.Origin)
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	for k, v := range c.cookies {
		req.AddCookie(&http.Cookie{Name: k, Value: v})
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do %s %s: %v", method, path, err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	for _, ck := range res.Cookies() {
		if ck.MaxAge < 0 || ck.Value == "" {
			delete(c.cookies, ck.Name)
		} else {
			c.cookies[ck.Name] = ck.Value
		}
	}
	return &Resp{Status: res.StatusCode, Body: b, Header: res.Header}
}

func (c *Client) Get(path string) *Resp             { return c.Do(http.MethodGet, path, nil) }
func (c *Client) Post(path string, body any) *Resp  { return c.Do(http.MethodPost, path, body) }
func (c *Client) Put(path string, body any) *Resp   { return c.Do(http.MethodPut, path, body) }
func (c *Client) Patch(path string, body any) *Resp { return c.Do(http.MethodPatch, path, body) }
func (c *Client) Delete(path string) *Resp          { return c.Do(http.MethodDelete, path, nil) }

// MustStatus fails the test when the response status differs.
func (r *Resp) MustStatus(t testing.TB, want int) *Resp {
	t.Helper()
	if r.Status != want {
		t.Fatalf("status = %d, want %d; body: %s", r.Status, want, string(r.Body))
	}
	return r
}
