// Package shutdown runs long-lived components (servers, workers, consumers) together
// and stops all of them gracefully on SIGINT/SIGTERM or when any of them fails.
package shutdown

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"
)

// SignalContext returns a context cancelled on SIGINT or SIGTERM.
func SignalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

// Group is an errgroup whose members each get a run and a stop function.
type Group struct {
	eg      *errgroup.Group
	ctx     context.Context
	timeout time.Duration
}

// New creates a Group bound to ctx. Each stop function gets at most timeout to finish.
func New(ctx context.Context, timeout time.Duration) *Group {
	eg, gctx := errgroup.WithContext(ctx)
	return &Group{eg: eg, ctx: gctx, timeout: timeout}
}

// Context is cancelled when the parent context is done or any member returns an error.
func (g *Group) Context() context.Context { return g.ctx }

// Go starts run in the background. Once the group context is done, stop (if not nil) is
// called so that run can return. run must return nil on a graceful stop.
func (g *Group) Go(run func(ctx context.Context) error, stop func(ctx context.Context) error) {
	g.eg.Go(func() error { return run(g.ctx) })
	if stop == nil {
		return
	}
	g.eg.Go(func() error {
		<-g.ctx.Done()
		ctx, cancel := context.WithTimeout(context.WithoutCancel(g.ctx), g.timeout)
		defer cancel()
		return stop(ctx)
	})
}

// Wait blocks until every member has returned and reports the first error.
func (g *Group) Wait() error { return g.eg.Wait() }
