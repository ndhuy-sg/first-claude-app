// Package worker runs background jobs from the jobs table.
// Owned by the payments & invoices agent (job loop, retries, invoice submission).
package worker

import (
	"context"

	"github.com/ndhuy-sg/first-claude-app/api/internal/platform"
)

// Run processes jobs until ctx is cancelled.
func Run(ctx context.Context, d *platform.Deps) error {
	<-ctx.Done()
	return nil
}
