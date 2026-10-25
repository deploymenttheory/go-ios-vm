package protocol

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deploymenttheory/go-ios-vm/internal/guest"
)

type fakeService struct {
	ping     PingResult
	shutdown func(ShutdownParams) (ShutdownResult, func(), error)
}

func (f fakeService) Ping(context.Context) (PingResult, error) { return f.ping, nil }
func (f fakeService) DeviceInfo(context.Context) (guest.DeviceInfo, error) {
	return guest.DeviceInfo{}, errors.New("identity unavailable")
}
func (f fakeService) Shutdown(_ context.Context, p ShutdownParams) (ShutdownResult, func(), error) {
	return f.shutdown(p)
}

func post(t *testing.T, h http.Handler, body string, headers map[string]string) (int, Response) {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "http://localhost/v1/rpc", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		if v == "" {
			req.Header.Del(k)
		} else {
			req.Header.Set(k, v)
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	var resp Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("reply is not JSON: %v: %s", err, w.Body.String())
	}
	return w.Code, resp
}

func TestRPCFramingAndValidation(t *testing.T) {
	h := NewHandler(fakeService{ping: PingResult{Ready: true}})
	tests := []struct {
		name    string
		body    string
		headers map[string]string
		status  int
		code    string
	}{
		{"not json", "ping", nil, 400, CodeInvalidRequest},
		{"no method", `{"params":{}}`, nil, 400, CodeInvalidRequest},
		{"long method", `{"method":"` + strings.Repeat("m", MaxMethod+1) + `"}`, nil, 400, CodeInvalidRequest},
		{"params array", `{"method":"ping","params":[1]}`, nil, 400, CodeInvalidRequest},
		{"bool id", `{"method":"ping","id":true}`, nil, 400, CodeInvalidRequest},
		{"unknown field", `{"method":"ping","token":"x"}`, nil, 400, CodeInvalidRequest},
		{"two requests", `{"method":"ping"}{"method":"ping"}`, nil, 400, CodeInvalidRequest},
		{"unsupported method", `{"method":"apps.list","id":7}`, nil, 400, CodeUnsupportedMethod},
		{"oversized", `{"method":"ping","params":{"x":"` + strings.Repeat("a", MaxBody) + `"}}`, nil, 413, CodeBadRequest},
		{"no content type", `{"method":"ping"}`, map[string]string{"Content-Type": ""}, 415, CodeBadRequest},
		{"browser origin", `{"method":"ping"}`, map[string]string{"Origin": "https://example.com"}, 403, CodeBadRequest},
		{"device info failure", `{"method":"device.info","id":"a"}`, nil, 400, CodeOperationFailed},
	}
	for _, tt := range tests {
		status, resp := post(t, h, tt.body, tt.headers)
		if status != tt.status || resp.Error == nil || resp.Error.Code != tt.code || resp.Type != "response" {
			t.Errorf("%s: %d %+v, want %d %s", tt.name, status, resp, tt.status, tt.code)
		}
	}
	status, resp := post(t, h, `{"method":"ping","id":"req-1"}`, nil)
	var ping PingResult
	if status != 200 || resp.Error != nil || string(resp.ID) != `"req-1"` || json.Unmarshal(resp.Result, &ping) != nil || !ping.Ready {
		t.Fatalf("ping: %d %+v", status, resp)
	}
	if _, resp := post(t, h, `{"method":"apps.list","id":7}`, nil); string(resp.ID) != "7" {
		t.Errorf("error reply id %s, want 7", resp.ID)
	}
}

func TestOnlyLocalClients(t *testing.T) {
	h := NewHandler(fakeService{})
	for host, want := range map[string]int{"localhost": 200, "127.0.0.1:1339": 200, "[::1]:1339": 200, "evil.example": 403, "192.168.64.1": 403} {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://placeholder/v1/health", nil)
		req.Host = host
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != want {
			t.Errorf("Host %q: %d, want %d", host, w.Code, want)
		}
	}
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://localhost/v1/rpc", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /v1/rpc: %d", w.Code)
	}
}

// TestShutdownAcknowledgesBeforeHalting checks over a real connection that the
// client has the full acknowledgment while the halt action is still blocked.
func TestShutdownAcknowledgesBeforeHalting(t *testing.T) {
	halting := make(chan struct{})
	release := make(chan struct{})
	svc := fakeService{shutdown: func(p ShutdownParams) (ShutdownResult, func(), error) {
		if !p.Force {
			return ShutdownResult{}, nil, &Error{Code: CodeInvalidRequest, Message: "pass force: true"}
		}
		return ShutdownResult{Accepted: true, Completion: "the virtual machine stops"}, func() {
			close(halting)
			<-release
		}, nil
	}}
	srv := httptest.NewServer(NewHandler(svc))
	defer srv.Close()
	defer close(release)

	call := func(body string) (int, Response) {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL+"/v1/rpc", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Host = "localhost"
		res, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		data, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		var resp Response
		if err := json.Unmarshal(data, &resp); err != nil {
			t.Fatal(err)
		}
		return res.StatusCode, resp
	}
	if status, resp := call(`{"method":"system.shutdown"}`); status != 400 || resp.Error == nil {
		t.Fatalf("shutdown without force: %d %+v", status, resp)
	}
	select {
	case <-halting:
		t.Fatal("halted without force")
	default:
	}
	status, resp := call(`{"method":"system.shutdown","params":{"force":true},"id":1}`)
	var ack ShutdownResult
	if status != 200 || json.Unmarshal(resp.Result, &ack) != nil || !ack.Accepted {
		t.Fatalf("shutdown: %d %+v", status, resp)
	}
	<-halting // the halt runs after the client already holds the acknowledgment
}
