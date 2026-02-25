package httpserver

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
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

// Config holds TLS configuration flags for the HTTP server.
type Config struct {
	EnableTLS   bool
	TLSCertFile string
	TLSKeyFile  string
	TLSCAFile   string
}

// RegisterFlags registers TLS-related flags on the given FlagSet.
func (c *Config) RegisterFlags(fs *flag.FlagSet) {
	fs.BoolVar(&c.EnableTLS, "enable-tls", false, "Enable TLS on the HTTP server")
	fs.StringVar(&c.TLSCertFile, "tls-cert-file", "", "Path to TLS certificate PEM file")
	fs.StringVar(&c.TLSKeyFile, "tls-key-file", "", "Path to TLS private key PEM file")
	fs.StringVar(&c.TLSCAFile, "tls-ca-file", "", "Optional path to CA chain PEM file (intermediates)")
}

// Validate checks that TLS configuration is consistent and that referenced
// files exist, are readable, and contain valid cryptographic material.
func (c *Config) Validate() error {
	var errs []error

	if c.EnableTLS {
		if c.TLSCertFile == "" {
			errs = append(errs, fmt.Errorf("tls-cert-file is required when enable-tls is true"))
		}
		if c.TLSKeyFile == "" {
			errs = append(errs, fmt.Errorf("tls-key-file is required when enable-tls is true"))
		}
	}

	if c.TLSCertFile != "" && c.TLSKeyFile != "" {
		if _, err := tls.LoadX509KeyPair(c.TLSCertFile, c.TLSKeyFile); err != nil {
			errs = append(errs, fmt.Errorf("tls-cert-file/tls-key-file: %w", err))
		}
	}

	if c.TLSCAFile != "" {
		pem, err := os.ReadFile(c.TLSCAFile)
		if err != nil {
			errs = append(errs, fmt.Errorf("tls-ca-file: %w", err))
		} else {
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(pem) {
				errs = append(errs, fmt.Errorf("tls-ca-file: no valid PEM certificates found in %q", c.TLSCAFile))
			}
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// Options holds tunable parameters for the HTTP server.
type Options struct {
	TLSConfig *tls.Config // nil means plain HTTP
}

// ToOptions converts a validated Config into an Options struct.
func (c *Config) ToOptions() (*Options, error) {
	if !c.EnableTLS {
		return &Options{}, nil
	}

	cert, err := tls.LoadX509KeyPair(c.TLSCertFile, c.TLSKeyFile)
	if err != nil {
		return nil, fmt.Errorf("loading TLS keypair: %w", err)
	}

	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	if c.TLSCAFile != "" {
		pem, err := os.ReadFile(c.TLSCAFile)
		if err != nil {
			return nil, fmt.Errorf("reading CA file: %w", err)
		}
		pool := x509.NewCertPool()
		pool.AppendCertsFromPEM(pem)
		tlsCfg.RootCAs = pool
	}

	return &Options{TLSConfig: tlsCfg}, nil
}

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
