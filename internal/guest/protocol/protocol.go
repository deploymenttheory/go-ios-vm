// Package protocol is the wire contract between the host and the Go guest
// daemon, ported from the initial part of vphone-cli's vphoned API at the
// pinned revision: HTTP/1.1 over vsock, GET /v1/health, and POST /v1/rpc
// carrying {"method", "params", "id"} with a {"type": "response", "id",
// "result" | "error"} reply. Only ping, device.info and system.shutdown are
// implemented; every other method is refused as unsupported.
package protocol

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/deploymenttheory/go-ios-vm/internal/guest"
)

// Port is the daemon's vsock port, as vphoned uses.
const Port = 1339

// Limits on requests.
const (
	MaxBody   = 1 << 20
	MaxMethod = 128
)

// Methods the daemon implements.
const (
	MethodPing       = "ping"
	MethodDeviceInfo = "device.info"
	MethodShutdown   = "system.shutdown"
)

// Error codes.
const (
	CodeBadRequest        = "bad_request"
	CodeInvalidRequest    = "invalid_request"
	CodeUnsupportedMethod = "unsupported_method"
	CodeOperationFailed   = "operation_failed"
)

// Error is an error reply.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// Request is one RPC request.
type Request struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
	ID     json.RawMessage `json:"id,omitempty"`
}

// Response is one RPC reply.
type Response struct {
	Type   string          `json:"type"`
	ID     json.RawMessage `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *Error          `json:"error,omitempty"`
}

// Decode reads and validates a request of at most MaxBody bytes.
func Decode(body []byte) (Request, error) {
	var req Request
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		return Request{}, &Error{Code: CodeInvalidRequest, Message: "expected {method, params?, id?}: " + err.Error()}
	}
	if dec.More() {
		return Request{}, &Error{Code: CodeInvalidRequest, Message: "trailing data after the request"}
	}
	if req.Method == "" || len(req.Method) > MaxMethod {
		return Request{}, &Error{Code: CodeInvalidRequest, Message: fmt.Sprintf("method must be 1 to %d bytes", MaxMethod)}
	}
	if p := bytes.TrimSpace(req.Params); len(p) > 0 && !bytes.Equal(p, []byte("null")) && p[0] != '{' {
		return Request{}, &Error{Code: CodeInvalidRequest, Message: "params must be an object"}
	}
	if id := bytes.TrimSpace(req.ID); len(id) > 0 && !bytes.Equal(id, []byte("null")) && id[0] != '"' && (id[0] < '0' || id[0] > '9') && id[0] != '-' {
		return Request{}, &Error{Code: CodeInvalidRequest, Message: "id must be a string or number"}
	}
	return req, nil
}

// PingResult is the readiness answer. Ready is true only when the daemon's
// own checks pass; Reasons says why not.
type PingResult struct {
	Ready     bool             `json:"ready"`
	Reasons   []string         `json:"reasons,omitempty"`
	Daemon    DaemonInfo       `json:"daemon"`
	Identity  guest.DeviceInfo `json:"identity"`
	CheckedAt time.Time        `json:"checked_at"`
}

// DaemonInfo identifies the running daemon.
type DaemonInfo struct {
	Version   string    `json:"version"`
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"started_at"`
}

// ShutdownResult acknowledges a shutdown request. The machine stopping, not
// this acknowledgment, is the evidence that the guest shut down.
type ShutdownResult struct {
	Accepted   bool   `json:"accepted"`
	Completion string `json:"completion"`
}

// ShutdownParams are system.shutdown's parameters.
type ShutdownParams struct {
	// Force must be true, as vphoned requires, so a stray request cannot halt
	// the guest.
	Force bool `json:"force"`
}

// Service is what the daemon does for each method.
type Service interface {
	Ping(ctx context.Context) (PingResult, error)
	DeviceInfo(ctx context.Context) (guest.DeviceInfo, error)
	// Shutdown validates the request and returns the action that halts the
	// guest. The handler runs it only after the acknowledgment is sent.
	Shutdown(ctx context.Context, p ShutdownParams) (ShutdownResult, func(), error)
}

// NewHandler serves the protocol for svc.
func NewHandler(svc Service) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", func(w http.ResponseWriter, r *http.Request) {
		if !LocalClient(r) {
			writeJSON(w, http.StatusForbidden, Response{Type: "response", Error: &Error{Code: CodeBadRequest, Message: "request is not from a local client"}})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "serving"})
	})
	mux.HandleFunc("/v1/rpc", func(w http.ResponseWriter, r *http.Request) { serveRPC(w, r, svc) })
	return mux
}

func serveRPC(w http.ResponseWriter, r *http.Request, svc Service) {
	fail := func(status int, code, message string) {
		writeJSON(w, status, Response{Type: "response", ID: json.RawMessage("null"), Error: &Error{Code: code, Message: message}})
	}
	switch {
	case r.Method != http.MethodPost:
		w.Header().Set("Allow", http.MethodPost)
		fail(http.StatusMethodNotAllowed, CodeBadRequest, "use POST")
		return
	case !LocalClient(r):
		fail(http.StatusForbidden, CodeBadRequest, "request is not from a local client")
		return
	case !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json"):
		fail(http.StatusUnsupportedMediaType, CodeBadRequest, "POST requires Content-Type: application/json")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, MaxBody+1))
	if err != nil {
		fail(http.StatusBadRequest, CodeBadRequest, "read request: "+err.Error())
		return
	}
	if len(body) > MaxBody {
		fail(http.StatusRequestEntityTooLarge, CodeBadRequest, "JSON body exceeds 1 MiB")
		return
	}
	req, err := Decode(body)
	if err != nil {
		var pe *Error
		errors.As(err, &pe)
		fail(http.StatusBadRequest, pe.Code, pe.Message)
		return
	}
	id := req.ID
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	result, after, err := dispatch(r.Context(), svc, req)
	if err != nil {
		var pe *Error
		if !errors.As(err, &pe) {
			pe = &Error{Code: CodeOperationFailed, Message: err.Error()}
		}
		writeJSON(w, http.StatusBadRequest, Response{Type: "response", ID: id, Error: pe})
		return
	}
	data, err := json.Marshal(result)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, Response{Type: "response", ID: id, Error: &Error{Code: CodeOperationFailed, Message: "encode result: " + err.Error()}})
		return
	}
	writeJSON(w, http.StatusOK, Response{Type: "response", ID: id, Result: data})
	if after != nil {
		// The acknowledgment reaches the host before the guest halts.
		_ = http.NewResponseController(w).Flush()
		after()
	}
}

func dispatch(ctx context.Context, svc Service, req Request) (any, func(), error) {
	switch req.Method {
	case MethodPing:
		res, err := svc.Ping(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("ping: %w", err)
		}
		return res, nil, nil
	case MethodDeviceInfo:
		res, err := svc.DeviceInfo(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("device.info: %w", err)
		}
		return res, nil, nil
	case MethodShutdown:
		var p ShutdownParams
		if len(req.Params) > 0 && !bytes.Equal(bytes.TrimSpace(req.Params), []byte("null")) {
			dec := json.NewDecoder(bytes.NewReader(req.Params))
			dec.DisallowUnknownFields()
			if err := dec.Decode(&p); err != nil {
				return nil, nil, &Error{Code: CodeInvalidRequest, Message: "system.shutdown params: " + err.Error()}
			}
		}
		res, after, err := svc.Shutdown(ctx, p)
		if err != nil {
			return nil, nil, fmt.Errorf("system.shutdown: %w", err)
		}
		return res, after, nil
	default:
		return nil, nil, &Error{Code: CodeUnsupportedMethod, Message: fmt.Sprintf("%q is not implemented by the Go guest daemon", req.Method)}
	}
}

// LocalClient reports whether a request comes from a first-party client
// rather than a web page: it carries no Origin and names no host other than
// loopback, as vphoned checks.
func LocalClient(r *http.Request) bool {
	if r.Header.Get("Origin") != "" {
		return false
	}
	host := r.Host
	if host == "" {
		return true
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.Trim(strings.ToLower(host), "[]")
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		data = []byte(`{"error":{"code":"operation_failed","message":"could not encode response"}}`)
		status = http.StatusInternalServerError
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", fmt.Sprint(len(data)))
	w.WriteHeader(status)
	_, _ = w.Write(data)
}
