// Package app wires every module into one HTTP router.
package app

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ndhuy-sg/first-claude-app/api/internal/auditlog"
	"github.com/ndhuy-sg/first-claude-app/api/internal/auth"
	"github.com/ndhuy-sg/first-claude-app/api/internal/authapi"
	"github.com/ndhuy-sg/first-claude-app/api/internal/branches"
	"github.com/ndhuy-sg/first-claude-app/api/internal/events"
	"github.com/ndhuy-sg/first-claude-app/api/internal/menu"
	"github.com/ndhuy-sg/first-claude-app/api/internal/orders"
	"github.com/ndhuy-sg/first-claude-app/api/internal/platform"
	"github.com/ndhuy-sg/first-claude-app/api/internal/reports"
	"github.com/ndhuy-sg/first-claude-app/api/internal/store"
	"github.com/ndhuy-sg/first-claude-app/api/internal/users"
)

// NewRouter builds the API router. All routes live under /api/v1.
func NewRouter(d *platform.Deps) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery(), requestLog())

	api := r.Group("/api/v1")
	api.GET("/healthz", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if err := d.Pool.Ping(ctx); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "db_down"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	idle := func(ctx context.Context) int {
		s, err := store.New(d.Pool).GetSettings(ctx)
		if err != nil {
			return 15
		}
		return int(s.PosIdleLockMinutes)
	}
	api.Use(auth.Middleware(d.Pool, d.Clock.Now, idle))
	api.Use(auth.OriginCheck(d.Cfg.AllowedOrigins)) // CSRF: writes need an allowed Origin (A-15)
	api.Use(auth.Idempotency(d.Pool))

	// ---- Module registrations: each module adds exactly one line below. ----
	events.Register(api, d) // first: in tests it wraps d.Events with the SSE hub
	authapi.Register(api, d)
	users.Register(api, d)
	branches.Register(api, d)
	menu.Register(api, d)
	orders.Register(api, d)
	// payments.Register(api, d)
	// shifts.Register(api, d)
	// invoices.Register(api, d)
	reports.Register(api, d)
	auditlog.Register(api, d)

	return r
}

func requestLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		slog.Info("http", "method", c.Request.Method, "path", c.FullPath(), "status", c.Writer.Status(), "ms", time.Since(start).Milliseconds())
	}
}
