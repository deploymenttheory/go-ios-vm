package webui

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestLoopbackAddress(t *testing.T) {
	for _, tc := range []struct {
		address string
		allowed bool
	}{
		{"127.0.0.1:8765", true},
		{"127.2.3.4:0", true},
		{"[::1]:8765", true},
		{"0.0.0.0:8765", false},
		{"[::]:8765", false},
		{"192.168.1.1:8765", false},
		{"localhost:8765", false},
		{":8765", false},
		{"127.0.0.1:missing", false},
		{"127.0.0.1:65536", false},
	} {
		if got := LoopbackAddress(tc.address); got != tc.allowed {
			t.Errorf("LoopbackAddress(%q) = %v", tc.address, got)
		}
	}
}

func TestReadOnlyRoutesAndSafeDatabaseFailure(t *testing.T) {
	var available atomic.Bool
	available.Store(true)
	handler := NewHandler(func(context.Context) error {
		if !available.Load() {
			return errors.New("postgres://user:secret@localhost/database")
		}
		return nil
	})
	server := httptest.NewServer(handler)
	defer server.Close()
	client := server.Client()
	for _, path := range []string{"/", "/assets/app.js", "/assets/style.css", "/api/v1", "/api/v1/health"} {
		resp, err := client.Get(server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil || resp.StatusCode != http.StatusOK || len(data) == 0 {
			t.Fatalf("GET %s: status=%d body=%q err=%v", path, resp.StatusCode, data, err)
		}
		if path == "/api/v1" && (!strings.Contains(string(data), `"api_version":"v1"`) ||
			!strings.Contains(string(data), `"read_only":true`)) {
			t.Fatalf("API boundary: %s", data)
		}
	}
	request, err := http.NewRequest(http.MethodHead, server.URL+"/api/v1/health", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil || resp.StatusCode != http.StatusOK || len(data) != 0 {
		t.Fatalf("HEAD health: status=%d body=%q err=%v", resp.StatusCode, data, err)
	}
	request, err = http.NewRequest(http.MethodPost, server.URL+"/api/v1/health", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed || resp.Header.Get("Allow") != "GET, HEAD" {
		t.Fatalf("write method accepted: %d", resp.StatusCode)
	}
	resp, err = client.Get(server.URL + "/api/v1/unknown")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unexpected API route: %d", resp.StatusCode)
	}
	available.Store(false)
	resp, err = client.Get(server.URL + "/api/v1/health")
	if err != nil {
		t.Fatal(err)
	}
	data, err = io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil || resp.StatusCode != http.StatusServiceUnavailable ||
		!strings.Contains(string(data), `"code":"storage_failure"`) ||
		strings.Contains(string(data), "secret") {
		t.Fatalf("unsafe health response: status=%d body=%q err=%v", resp.StatusCode, data, err)
	}
}

func TestHandlerRefusesRemoteAddressAndForeignHost(t *testing.T) {
	handler := NewHandler(func(context.Context) error { return nil })
	for _, tc := range []struct {
		remote string
		host   string
	}{
		{"192.0.2.12:54321", "127.0.0.1:8765"},
		{"127.0.0.1:54321", "example.com:8765"},
	} {
		r := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8765/api/v1/health", nil)
		r.RemoteAddr = tc.remote
		r.Host = tc.host
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != http.StatusForbidden {
			t.Fatalf("remote=%s host=%s status=%d", tc.remote, tc.host, w.Code)
		}
	}
}

func TestServeShutdownReleasesListener(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, listener, NewHandler(func(context.Context) error { return nil })) }()
	client := &http.Client{Timeout: time.Second}
	resp, err := client.Get("http://" + address + "/api/v1/health")
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		cancel()
		t.Fatalf("status=%d", resp.StatusCode)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shut down")
	}
	rebound, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatalf("listener not released: %v", err)
	}
	rebound.Close()
}
