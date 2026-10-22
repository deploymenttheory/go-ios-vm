// Package webui serves the local, read-only inspection application.
package webui

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/deploymenttheory/go-ios-vm/internal/postgres"
)

//go:embed assets/*
var assets embed.FS

const shutdownTimeout = 5 * time.Second

type diagnostic struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type response struct {
	APIVersion string      `json:"api_version"`
	State      string      `json:"state"`
	ReadOnly   bool        `json:"read_only"`
	Endpoints  []string    `json:"endpoints,omitempty"`
	Data       any         `json:"data,omitempty"`
	Diagnostic *diagnostic `json:"diagnostic,omitempty"`
}

// LoopbackAddress accepts only numeric TCP loopback addresses. Resolving a
// hostname at listen time could select a non-loopback interface.
func LoopbackAddress(address string) bool {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 0 || number > 65535 {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func localRequest(r *http.Request) bool {
	remoteHost, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(remoteHost)
	if ip == nil || !ip.IsLoopback() {
		return false
	}
	host := r.Host
	if parsed, _, splitErr := net.SplitHostPort(host); splitErr == nil {
		host = parsed
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip = net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func jsonResponse(w http.ResponseWriter, r *http.Request, status int, body response) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if r.Method != http.MethodHead {
		_ = json.NewEncoder(w).Encode(body)
	}
}

// NewHandler exposes only the versioned read API, its health check and the
// bundled browser shell. The check callback must not return secrets to clients.
func NewHandler(check func(context.Context) error) http.Handler {
	return newHandler(check, nil)
}

// NewReadHandler adds bounded PostgreSQL browse routes to the local shell.
func NewReadHandler(store *postgres.Store) http.Handler {
	return newHandler(store.Ping, store)
}

func newHandler(check func(context.Context) error, store *postgres.Store) http.Handler {
	static, _ := fs.Sub(assets, "assets")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy",
			"default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; base-uri 'none'; form-action 'none'")
		if !localRequest(r) {
			http.Error(w, "local access only", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "read-only endpoint", http.StatusMethodNotAllowed)
			return
		}
		switch r.URL.Path {
		case "/api/v1", "/api/v1/":
			endpoints := []string{"/api/v1/health"}
			if store != nil {
				endpoints = append(endpoints,
					"/api/v1/artifacts", "/api/v1/targets",
					"/api/v1/acquisitions", "/api/v1/runs", "/api/v1/progress",
					"/api/v1/acquisitions/{attempt_id}/progress",
					"/api/v1/acquisitions/{attempt_id}/progress/events",
					"/api/v1/runs/{run_id}/progress",
					"/api/v1/runs/{run_id}/progress/events",
					"/api/v1/findings", "/api/v1/observations", "/api/v1/graph")
			}
			jsonResponse(w, r, http.StatusOK, response{
				APIVersion: "v1", State: "ready", ReadOnly: true,
				Endpoints: endpoints,
			})
		case "/api/v1/health":
			checkCtx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()
			if check == nil || check(checkCtx) != nil {
				jsonResponse(w, r, http.StatusServiceUnavailable, response{
					APIVersion: "v1", State: "unavailable", ReadOnly: true,
					Diagnostic: &diagnostic{
						Code: "storage_failure", Message: "Evidence database is unavailable",
					},
				})
				return
			}
			jsonResponse(w, r, http.StatusOK, response{
				APIVersion: "v1", State: "ready", ReadOnly: true,
			})
		case "/":
			serveAsset(w, r, static, "index.html", "text/html; charset=utf-8")
		case "/assets/app.js":
			serveAsset(w, r, static, "app.js", "text/javascript; charset=utf-8")
		case "/assets/style.css":
			serveAsset(w, r, static, "style.css", "text/css; charset=utf-8")
		default:
			if strings.HasPrefix(r.URL.Path, "/api/") {
				if serveProgress(w, r, store) {
					return
				}
				if store != nil && serveProgressList(w, r, store) {
					return
				}
				if store != nil && serveBrowse(w, r, store) {
					return
				}
				if serveFindings(w, r, store) {
					return
				}
				if serveGraph(w, r, store) {
					return
				}
				jsonResponse(w, r, http.StatusNotFound, response{
					APIVersion: "v1", State: "not_found", ReadOnly: true,
				})
				return
			}
			http.NotFound(w, r)
		}
	})
}

func serveAsset(w http.ResponseWriter, r *http.Request, static fs.FS, name, contentType string) {
	data, err := fs.ReadFile(static, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(data)
	}
}

// Serve stops accepting connections when ctx is cancelled and waits for active
// requests to finish before returning. The caller owns the database pool.
func Serve(ctx context.Context, listener net.Listener, handler http.Handler) error {
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	served := make(chan error, 1)
	go func() { served <- server.Serve(listener) }()
	select {
	case err := <-served:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
		defer cancel()
		shutdownErr := server.Shutdown(shutdownCtx)
		serveErr := <-served
		if shutdownErr != nil {
			_ = server.Close()
			return fmt.Errorf("shutdown UI server: %w", shutdownErr)
		}
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			return serveErr
		}
		return nil
	}
}
