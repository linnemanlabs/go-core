package httpserver

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
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
	_, err := Start(context.Background(), ":0", nil, log.Nop())
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

	stop, err := Start(context.Background(), addr, handler, nil)
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

	stop, err := Start(context.Background(), addr, handler, log.Nop())
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
	stop, err := Start(ctx, addr, handler, log.Nop())
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
	stop, err := Start(ctx, addr, http.NotFoundHandler(), log.Nop())
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
	stop1, err := Start(ctx, addr, http.NotFoundHandler(), log.Nop())
	if err != nil {
		t.Fatalf("first Start: %v", err)
	}
	defer stop1(ctx)

	_, err = Start(ctx, addr, http.NotFoundHandler(), log.Nop())
	if err == nil {
		t.Fatal("expected error for port conflict")
	}
}

func TestStart_InvalidAddr(t *testing.T) {
	_, err := Start(context.Background(), ":-1", http.NotFoundHandler(), log.Nop())
	if err == nil {
		t.Fatal("expected error for invalid address")
	}
}

func TestStart_TCP4Only(t *testing.T) {
	port := getFreePort(t)
	addr := fmt.Sprintf(":%d", port)

	ctx := context.Background()
	stop, err := Start(ctx, addr, http.NotFoundHandler(), log.Nop())
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
