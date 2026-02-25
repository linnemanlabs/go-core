package httpserver

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/linnemanlabs/go-core/log"
)

// test helpers

// getFreePort finds a free TCP port.
func getFreePort(t *testing.T) int {
	t.Helper()
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp4", ":0")
	if err != nil {
		t.Fatalf("find free port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}

// generateSelfSignedCert creates a self-signed certificate and key,
// writes them to temp files, and returns the file paths.
func generateSelfSignedCert(t *testing.T) (certFile, keyFile string) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}

	dir := t.TempDir()
	certFile = filepath.Join(dir, "cert.pem")
	keyFile = filepath.Join(dir, "key.pem")

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	if err := os.WriteFile(certFile, certPEM, 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	if err := os.WriteFile(keyFile, keyPEM, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	return certFile, keyFile
}

// TestDefaults verifies that the exported default constants have their expected values.
func TestDefaults(t *testing.T) {
	tests := []struct {
		name string
		got  any
		want any
	}{
		{"ReadHeaderTimeout", DefaultReadHeaderTimeout, 5 * time.Second},
		{"ReadTimeout", DefaultReadTimeout, 10 * time.Second},
		{"WriteTimeout", DefaultWriteTimeout, 10 * time.Second},
		{"IdleTimeout", DefaultIdleTimeout, 60 * time.Second},
		{"MaxHeaderBytes", DefaultMaxHeaderBytes, 1 << 20},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.want)
		}
	}
}

// NewServer

func TestNewServer_Addr(t *testing.T) {
	srv := NewServer(":9090", http.NotFoundHandler())
	if srv.Addr != ":9090" {
		t.Fatalf("Addr = %q, want %q", srv.Addr, ":9090")
	}
}

func TestNewServer_Handler(t *testing.T) {
	h := http.NotFoundHandler()
	srv := NewServer(":0", h)
	if srv.Handler == nil {
		t.Fatal("Handler is nil")
	}
}

func TestNewServer_Timeouts(t *testing.T) {
	srv := NewServer(":0", http.NotFoundHandler())

	if srv.ReadHeaderTimeout != DefaultReadHeaderTimeout {
		t.Fatalf("ReadHeaderTimeout = %v, want %v", srv.ReadHeaderTimeout, DefaultReadHeaderTimeout)
	}
	if srv.ReadTimeout != DefaultReadTimeout {
		t.Fatalf("ReadTimeout = %v, want %v", srv.ReadTimeout, DefaultReadTimeout)
	}
	if srv.WriteTimeout != DefaultWriteTimeout {
		t.Fatalf("WriteTimeout = %v, want %v", srv.WriteTimeout, DefaultWriteTimeout)
	}
	if srv.IdleTimeout != DefaultIdleTimeout {
		t.Fatalf("IdleTimeout = %v, want %v", srv.IdleTimeout, DefaultIdleTimeout)
	}
}

func TestNewServer_MaxHeaderBytes(t *testing.T) {
	srv := NewServer(":0", http.NotFoundHandler())
	if srv.MaxHeaderBytes != 1<<20 {
		t.Fatalf("MaxHeaderBytes = %d, want %d", srv.MaxHeaderBytes, 1<<20)
	}
}

func TestNewServer_TimeoutsNonZero(t *testing.T) {
	srv := NewServer(":0", http.NotFoundHandler())

	if srv.ReadHeaderTimeout == 0 {
		t.Fatal("ReadHeaderTimeout is zero - vulnerable to slowloris")
	}
	if srv.ReadTimeout == 0 {
		t.Fatal("ReadTimeout is zero")
	}
	if srv.WriteTimeout == 0 {
		t.Fatal("WriteTimeout is zero")
	}
	if srv.IdleTimeout == 0 {
		t.Fatal("IdleTimeout is zero")
	}
}

// Start

func TestStart_NilHandler(t *testing.T) {
	_, err := Start(context.Background(), ":0", nil, log.Nop(), nil)
	if err == nil {
		t.Fatal("expected error for nil handler")
	}
}

func TestStart_NilLogger(t *testing.T) {
	port := getFreePort(t)
	addr := fmt.Sprintf(":%d", port)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	stop, err := Start(context.Background(), addr, handler, nil, nil)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer stop(context.Background())
}

func TestStart_ListensAndServes(t *testing.T) {
	port := getFreePort(t)
	addr := fmt.Sprintf(":%d", port)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello"))
	})

	stop, err := Start(context.Background(), addr, handler, log.Nop(), nil)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer stop(context.Background())

	url := fmt.Sprintf("http://127.0.0.1:%d/", port)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, http.NoBody)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if string(body) != "hello" {
		t.Fatalf("body = %q, want %q", string(body), "hello")
	}
}

func TestStart_GracefulShutdown(t *testing.T) {
	port := getFreePort(t)
	addr := fmt.Sprintf(":%d", port)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	ctx := context.Background()
	stop, err := Start(ctx, addr, handler, log.Nop(), nil)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Verify server is accepting connections.
	url := fmt.Sprintf("http://127.0.0.1:%d/", port)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, http.NoBody)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("server not accepting: %v", err)
	}
	resp.Body.Close()

	// Stop and verify connections are refused.
	if err := stop(ctx); err != nil {
		t.Fatalf("stop: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	req, err = http.NewRequestWithContext(t.Context(), http.MethodGet, url, http.NoBody)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err = http.DefaultClient.Do(req)
	if err == nil {
		resp.Body.Close()
		t.Fatal("server still accepting connections after shutdown")
	}
}

func TestStart_StopIdempotent(t *testing.T) {
	port := getFreePort(t)
	addr := fmt.Sprintf(":%d", port)

	ctx := context.Background()
	stop, err := Start(ctx, addr, http.NotFoundHandler(), log.Nop(), nil)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	if err := stop(ctx); err != nil {
		t.Fatalf("first stop: %v", err)
	}
	if err := stop(ctx); err != nil {
		t.Fatalf("second stop: %v", err)
	}
	if err := stop(ctx); err != nil {
		t.Fatalf("third stop: %v", err)
	}
}

func TestStart_PortConflict(t *testing.T) {
	port := getFreePort(t)
	addr := fmt.Sprintf(":%d", port)

	ctx := context.Background()
	stop1, err := Start(ctx, addr, http.NotFoundHandler(), log.Nop(), nil)
	if err != nil {
		t.Fatalf("first Start: %v", err)
	}
	defer stop1(ctx)

	_, err = Start(ctx, addr, http.NotFoundHandler(), log.Nop(), nil)
	if err == nil {
		t.Fatal("expected error for port conflict")
	}
}

func TestStart_InvalidAddr(t *testing.T) {
	_, err := Start(context.Background(), ":-1", http.NotFoundHandler(), log.Nop(), nil)
	if err == nil {
		t.Fatal("expected error for invalid address")
	}
}

func TestStart_TCP4Only(t *testing.T) {
	port := getFreePort(t)
	addr := fmt.Sprintf(":%d", port)

	ctx := context.Background()
	stop, err := Start(ctx, addr, http.NotFoundHandler(), log.Nop(), nil)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer stop(ctx)

	// Connect via IPv4 should succeed.
	conn, err := net.DialTimeout("tcp4", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	if err != nil {
		t.Fatalf("tcp4 dial to 127.0.0.1:%d failed: %v", port, err)
	}
	conn.Close()
}

// Config

func TestConfig_RegisterFlags(t *testing.T) {
	var c Config
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	c.RegisterFlags(fs)

	if err := fs.Parse([]string{"-enable-tls", "-tls-cert-file", "cert.pem", "-tls-key-file", "key.pem", "-tls-ca-file", "ca.pem"}); err != nil {
		t.Fatalf("parse: %v", err)
	}

	if !c.EnableTLS {
		t.Fatal("EnableTLS should be true")
	}
	if c.TLSCertFile != "cert.pem" {
		t.Fatalf("TLSCertFile = %q, want %q", c.TLSCertFile, "cert.pem")
	}
	if c.TLSKeyFile != "key.pem" {
		t.Fatalf("TLSKeyFile = %q, want %q", c.TLSKeyFile, "key.pem")
	}
	if c.TLSCAFile != "ca.pem" {
		t.Fatalf("TLSCAFile = %q, want %q", c.TLSCAFile, "ca.pem")
	}
}

func TestConfig_Validate(t *testing.T) {
	certFile, keyFile := generateSelfSignedCert(t)

	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name:    "disabled no files",
			config:  Config{EnableTLS: false},
			wantErr: false,
		},
		{
			name:    "enabled no cert",
			config:  Config{EnableTLS: true, TLSKeyFile: keyFile},
			wantErr: true,
		},
		{
			name:    "enabled no key",
			config:  Config{EnableTLS: true, TLSCertFile: certFile},
			wantErr: true,
		},
		{
			name:    "enabled valid cert and key",
			config:  Config{EnableTLS: true, TLSCertFile: certFile, TLSKeyFile: keyFile},
			wantErr: false,
		},
		{
			name:    "invalid cert path",
			config:  Config{EnableTLS: true, TLSCertFile: "/nonexistent/cert.pem", TLSKeyFile: keyFile},
			wantErr: true,
		},
		{
			name:    "invalid key path",
			config:  Config{EnableTLS: true, TLSCertFile: certFile, TLSKeyFile: "/nonexistent/key.pem"},
			wantErr: true,
		},
		{
			name:    "valid CA file",
			config:  Config{EnableTLS: true, TLSCertFile: certFile, TLSKeyFile: keyFile, TLSCAFile: certFile},
			wantErr: false,
		},
		{
			name:    "invalid CA path",
			config:  Config{TLSCAFile: "/nonexistent/ca.pem"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConfig_Validate_GarbageCA(t *testing.T) {
	dir := t.TempDir()
	caFile := filepath.Join(dir, "garbage.pem")
	if err := os.WriteFile(caFile, []byte("not a cert"), 0o600); err != nil {
		t.Fatalf("write garbage CA: %v", err)
	}

	c := Config{TLSCAFile: caFile}
	if err := c.Validate(); err == nil {
		t.Fatal("expected error for garbage CA file")
	}
}

func TestConfig_ToOptions_Disabled(t *testing.T) {
	c := Config{EnableTLS: false}
	opts, err := c.ToOptions()
	if err != nil {
		t.Fatalf("ToOptions: %v", err)
	}
	if opts.TLSConfig != nil {
		t.Fatal("TLSConfig should be nil when TLS is disabled")
	}
}

func TestConfig_ToOptions_Enabled(t *testing.T) {
	certFile, keyFile := generateSelfSignedCert(t)

	c := Config{EnableTLS: true, TLSCertFile: certFile, TLSKeyFile: keyFile}
	opts, err := c.ToOptions()
	if err != nil {
		t.Fatalf("ToOptions: %v", err)
	}
	if opts.TLSConfig == nil {
		t.Fatal("TLSConfig should be non-nil when TLS is enabled")
	}
	if len(opts.TLSConfig.Certificates) != 1 {
		t.Fatalf("Certificates len = %d, want 1", len(opts.TLSConfig.Certificates))
	}
}

func TestConfig_ToOptions_WithCA(t *testing.T) {
	certFile, keyFile := generateSelfSignedCert(t)

	c := Config{EnableTLS: true, TLSCertFile: certFile, TLSKeyFile: keyFile, TLSCAFile: certFile}
	opts, err := c.ToOptions()
	if err != nil {
		t.Fatalf("ToOptions: %v", err)
	}
	if opts.TLSConfig == nil {
		t.Fatal("TLSConfig should be non-nil")
	}
	if opts.TLSConfig.RootCAs == nil {
		t.Fatal("RootCAs should be non-nil when CA file is provided")
	}
}

// TLS integration

func TestStart_TLS(t *testing.T) {
	certFile, keyFile := generateSelfSignedCert(t)

	c := Config{EnableTLS: true, TLSCertFile: certFile, TLSKeyFile: keyFile}
	opts, err := c.ToOptions()
	if err != nil {
		t.Fatalf("ToOptions: %v", err)
	}

	port := getFreePort(t)
	addr := fmt.Sprintf(":%d", port)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("secure"))
	})

	stop, err := Start(context.Background(), addr, handler, log.Nop(), opts)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer stop(context.Background())

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	url := fmt.Sprintf("https://127.0.0.1:%d/", port)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, http.NoBody)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if string(body) != "secure" {
		t.Fatalf("body = %q, want %q", string(body), "secure")
	}
	if resp.TLS == nil {
		t.Fatal("expected TLS connection")
	}
}
