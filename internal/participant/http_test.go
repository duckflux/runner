package participant

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/duckflux/runner/internal/model"
)

// newHTTP is a test helper that builds an HTTPParticipant backed by ts.
func newHTTP(def model.Participant, ts *httptest.Server) *HTTPParticipant {
	return NewHTTP(def, ts.Client())
}

func TestHTTPGetReturnsBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello http"))
	}))
	defer ts.Close()

	p := newHTTP(model.Participant{URL: ts.URL, Method: "GET"}, ts)
	out, err := p.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	got, ok := out.(string)
	if !ok {
		t.Fatalf("Execute() returned %T, want string", out)
	}
	if got != "hello http" {
		t.Errorf("Execute() = %q, want 'hello http'", got)
	}
}

func TestHTTPDefaultMethodIsGET(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(r.Method))
	}))
	defer ts.Close()

	// No method set — should default to GET.
	p := newHTTP(model.Participant{URL: ts.URL}, ts)
	out, err := p.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if out != "GET" {
		t.Errorf("Execute() = %q, want 'GET'", out)
	}
}

func TestHTTPPostWithStringBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "expected POST", http.StatusBadRequest)
			return
		}
		body, _ := io.ReadAll(r.Body)
		w.Write(body)
	}))
	defer ts.Close()

	p := newHTTP(model.Participant{URL: ts.URL, Method: "POST", Body: "ping"}, ts)
	out, err := p.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if out != "ping" {
		t.Errorf("Execute() = %q, want 'ping'", out)
	}
}

func TestHTTPPostWithMapBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var m map[string]any
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(m)
	}))
	defer ts.Close()

	body := map[string]any{"key": "value"}
	p := newHTTP(model.Participant{URL: ts.URL, Method: "POST", Body: body}, ts)
	out, err := p.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("Execute() returned %T, want map", out)
	}
	if m["key"] != "value" {
		t.Errorf("Execute() map = %v, want key=value", m)
	}
}

func TestHTTPInputUsedAsBodyWhenBodyNotSet(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Write(body)
	}))
	defer ts.Close()

	p := newHTTP(model.Participant{URL: ts.URL, Method: "POST"}, ts)
	out, err := p.Execute(context.Background(), "from-input")
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if out != "from-input" {
		t.Errorf("Execute() = %q, want 'from-input'", out)
	}
}

func TestHTTPCustomHeaders(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(r.Header.Get("X-Custom")))
	}))
	defer ts.Close()

	p := newHTTP(model.Participant{
		URL:     ts.URL,
		Headers: map[string]string{"X-Custom": "test-value"},
	}, ts)
	out, err := p.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if out != "test-value" {
		t.Errorf("Execute() = %q, want 'test-value'", out)
	}
}

func TestHTTPNon2xxStatusReturnsError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer ts.Close()

	p := newHTTP(model.Participant{URL: ts.URL}, ts)
	_, err := p.Execute(context.Background(), nil)
	if err == nil {
		t.Fatal("Execute() expected error for 404, got nil")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error = %q, expected it to contain status code 404", err.Error())
	}
}

func TestHTTPEmptyURLReturnsError(t *testing.T) {
	p := NewHTTP(model.Participant{}, nil)
	_, err := p.Execute(context.Background(), nil)
	if err == nil {
		t.Fatal("Execute() expected error for empty URL, got nil")
	}
}

func TestHTTPContextCancellation(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow server.
		select {
		case <-r.Context().Done():
		case <-time.After(10 * time.Second):
		}
		w.Write([]byte("too late"))
	}))
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	p := newHTTP(model.Participant{URL: ts.URL}, ts)
	start := time.Now()
	_, err := p.Execute(ctx, nil)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("Execute() expected error on context timeout, got nil")
	}
	if elapsed > 5*time.Second {
		t.Errorf("Execute() took %v; expected fast cancellation", elapsed)
	}
}

func TestHTTPJSONResponseAutoDecoded(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok","count":3}`))
	}))
	defer ts.Close()

	p := newHTTP(model.Participant{URL: ts.URL}, ts)
	out, err := p.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("Execute() returned %T, want map[string]any", out)
	}
	if m["status"] != "ok" {
		t.Errorf("Execute() map status = %v, want 'ok'", m["status"])
	}
}

func TestHTTPBodyDefinitionOverridesInput(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Write(body)
	}))
	defer ts.Close()

	// def body should win over the Execute input.
	p := newHTTP(model.Participant{URL: ts.URL, Method: "POST", Body: "from-def"}, ts)
	out, err := p.Execute(context.Background(), "from-input")
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if out != "from-def" {
		t.Errorf("Execute() = %q, want 'from-def'", out)
	}
}
