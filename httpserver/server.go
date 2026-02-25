package httpserver

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/linnemanlabs/go-core/log"
	"github.com/linnemanlabs/go-core/xerrors"
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

// Start HTTP server.
// When opts is non-nil and opts.TLSConfig is set, the server uses TLS.
// Returns stop(ctx) for graceful shutdown.
func Start(ctx context.Context, addr string, handler http.Handler, logger log.Logger, opts *Options) (func(context.Context) error, error) {
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

	useTLS := opts != nil && opts.TLSConfig != nil
	if useTLS {
		srv.TLSConfig = opts.TLSConfig
		ln = tls.NewListener(ln, srv.TLSConfig)
		logger.Info(ctx, "http server listening", "addr", addr, "tls", true)
	} else {
		logger.Info(ctx, "http server listening", "addr", addr)
	}

	go func() {
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
