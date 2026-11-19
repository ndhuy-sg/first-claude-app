// Command mocpos runs the Mộc POS backend.
//
//	mocpos serve    # HTTP API
//	mocpos worker   # background jobs (invoice submission, emails, alerts)
//	mocpos migrate  # apply database migrations
//	mocpos seed     # load demo data (5 branches, menu, staff) into an empty database
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/ndhuy-sg/first-claude-app/api/internal/app"
	"github.com/ndhuy-sg/first-claude-app/api/internal/config"
	"github.com/ndhuy-sg/first-claude-app/api/internal/events"
	"github.com/ndhuy-sg/first-claude-app/api/internal/platform"
	"github.com/ndhuy-sg/first-claude-app/api/internal/seed"
	"github.com/ndhuy-sg/first-claude-app/api/internal/worker"
	"github.com/ndhuy-sg/first-claude-app/api/migrations"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: mocpos serve|worker|migrate|seed")
		os.Exit(2)
	}
	cfg := config.Load()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var err error
	switch os.Args[1] {
	case "migrate":
		err = Migrate(cfg.DatabaseURL)
	case "seed":
		err = withPool(ctx, cfg, func(p *pgxpool.Pool) error {
			_, err := seed.Seed(ctx, p, seed.Options{})
			return err
		})
	case "serve":
		err = withPool(ctx, cfg, func(p *pgxpool.Pool) error { return serve(ctx, cfg, p) })
	case "worker":
		err = withPool(ctx, cfg, func(p *pgxpool.Pool) error {
			return worker.Run(ctx, &platform.Deps{Pool: p, Cfg: cfg, Clock: platform.SystemClock{}, Events: platform.NoopPublisher{}})
		})
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func withPool(ctx context.Context, cfg config.Config, fn func(*pgxpool.Pool) error) error {
	p, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer p.Close()
	return fn(p)
}

func serve(ctx context.Context, cfg config.Config, p *pgxpool.Pool) error {
	hub := events.NewHub() // SSE; events from the separate worker process are not delivered (phase 1)
	d := &platform.Deps{Pool: p, Cfg: cfg, Clock: platform.SystemClock{}, Events: hub}
	srv := &http.Server{Addr: cfg.HTTPAddr, Handler: app.NewRouter(d), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		hub.Close() // end SSE streams so Shutdown is not held up
		sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()
	slog.Info("listening", "addr", cfg.HTTPAddr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Migrate applies all embedded migrations.
func Migrate(dbURL string) error {
	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		return err
	}
	defer db.Close()
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	return goose.Up(db, ".")
}
