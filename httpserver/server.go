package httpserver

import (
	"context"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/linnemanlabs/go-core/log"
	"github.com/linnemanlabs/go-core/xerrors"
)

// Server timeout defaults, shared with opshttp.
const (
	DefaultReadHeaderTimeout = 5 * time.Second
	DefaultReadTimeout       = 10 * time.Second
	DefaultWriteTimeout      = 10 * time.Second
	DefaultIdleTimeout       = 60 * time.Second
	DefaultMaxHeaderBytes    = 1 << 20 // 1 MB
)

// NewServer creates a new http.Server with the given address and handler, and default timeouts.
func NewServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: DefaultReadHeaderTimeout,
		ReadTimeout:       DefaultReadTimeout,
		WriteTimeout:      DefaultWriteTimeout,
		IdleTimeout:       DefaultIdleTimeout,
		MaxHeaderBytes:    DefaultMaxHeaderBytes,
	}
}

// Start HTTP server
// Returns stop(ctx) for graceful shutdown
func Start(ctx context.Context, addr string, handler http.Handler, logger log.Logger) (func(context.Context) error, error) {
	if handler == nil {
		return nil, xerrors.New("handler is required")
	}
	if logger == nil {
		logger = log.Nop()
	}

	// Listen on TCP4 to avoid dual-stack issues, our infra only supports IPv4 by design
	srv := NewServer(addr, handler)
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp4", addr)
	if err != nil {
		return nil, xerrors.EnsureTrace(err)
	}

	go func() {
		logger.Info(ctx, "http server listening", "addr", addr)
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			logger.Error(ctx, err, "http server error")
		}
	}()

	var once sync.Once
	stop := func(sctx context.Context) (retErr error) {
		once.Do(func() {
			logger.Info(sctx, "http server shutting down")
			c, cancel := context.WithTimeout(sctx, 5*time.Second)
			defer cancel()
			retErr = srv.Shutdown(c)
		})
		return retErr
	}
	return stop, nil
}
