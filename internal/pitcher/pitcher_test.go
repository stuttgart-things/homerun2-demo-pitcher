
package pitcher

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	homerun "github.com/stuttgart-things/homerun-library/v3"
)

// stubPitcher is a test double that records calls and returns preset values.
type stubPitcher struct {
	called   atomic.Int32
	objectID string
	streamID string
	err      error
}

func (s *stubPitcher) Pitch(msg homerun.Message) (string, string, error) {
	s.called.Add(1)
	return s.objectID, s.streamID, s.err
}

func TestHTTPPitcher(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", ct)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-token" {
			t.Errorf("expected Authorization Bearer test-token, got %s", auth)
		}

		var msg homerun.Message
		if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		if msg.Title != "test-title" {
			t.Errorf("expected title test-title, got %s", msg.Title)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(httpResponse{
			ObjectID: "obj-123",
			StreamID: "stream-456",
			Status:   "success",
		})
	}))
	defer srv.Close()

	p := &HTTPPitcher{
		Endpoint:   srv.URL,
		APIPath:    "generic",
		AuthToken:  "test-token",
		HTTPClient: srv.Client(),
	}

	msg := homerun.Message{
		Title:   "test-title",
		Message: "test-body",
	}

	objectID, streamID, err := p.Pitch(msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if objectID != "obj-123" {
		t.Errorf("expected objectID obj-123, got %s", objectID)
	}
	if streamID != "stream-456" {
		t.Errorf("expected streamID stream-456, got %s", streamID)
	}
}

func TestHTTPPitcherRetry(t *testing.T) {
	var attempts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n <= 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("unavailable"))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(httpResponse{
			ObjectID: "retry-obj",
			StreamID: "retry-stream",
			Status:   "success",
		})
	}))
	defer srv.Close()

	p := &HTTPPitcher{
		Endpoint:   srv.URL,
		APIPath:    "generic",
		HTTPClient: srv.Client(),
	}

	msg := homerun.Message{
		Title:   "retry-test",
		Message: "body",
	}

	objectID, streamID, err := p.Pitch(msg)
	if err != nil {
		t.Fatalf("expected success after retries, got error: %v", err)
	}
	if objectID != "retry-obj" {
		t.Errorf("expected objectID retry-obj, got %s", objectID)
	}
	if streamID != "retry-stream" {
		t.Errorf("expected streamID retry-stream, got %s", streamID)
	}
	if got := attempts.Load(); got != 3 {
		t.Errorf("expected 3 attempts, got %d", got)
	}
}

func TestHTTPPitcherHealthCheck(t *testing.T) {
	t.Run("healthy", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/health" {
				t.Errorf("expected path /health, got %s", r.URL.Path)
			}
			if r.Method != http.MethodGet {
				t.Errorf("expected GET, got %s", r.Method)
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"ok"}`))
		}))
		defer srv.Close()

		p := &HTTPPitcher{
			Endpoint:   srv.URL,
			HTTPClient: srv.Client(),
		}

		if err := p.HealthCheck(context.Background()); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("unhealthy", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer srv.Close()

		p := &HTTPPitcher{
			Endpoint:   srv.URL,
			HTTPClient: srv.Client(),
		}

		if err := p.HealthCheck(context.Background()); err == nil {
			t.Fatal("expected error for unhealthy endpoint, got nil")
		}
	})
}

func TestMultiPitcher(t *testing.T) {
	s1 := &stubPitcher{objectID: "obj-1", streamID: "stream-1"}
	s2 := &stubPitcher{objectID: "obj-2", streamID: "stream-2"}

	mp := &MultiPitcher{
		Pitchers: []Pitcher{s1, s2},
	}

	msg := homerun.Message{Title: "multi", Message: "test"}

	objectID, streamID, err := mp.Pitch(msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should return the first successful result.
	if objectID != "obj-1" {
		t.Errorf("expected objectID obj-1, got %s", objectID)
	}
	if streamID != "stream-1" {
		t.Errorf("expected streamID stream-1, got %s", streamID)
	}

	// Both backends must have been called.
	if got := s1.called.Load(); got != 1 {
		t.Errorf("expected backend 0 called once, got %d", got)
	}
	if got := s2.called.Load(); got != 1 {
		t.Errorf("expected backend 1 called once, got %d", got)
	}
}

func TestMultiPitcherPartialFailure(t *testing.T) {
	failing := &stubPitcher{err: fmt.Errorf("connection refused")}
	working := &stubPitcher{objectID: "obj-ok", streamID: "stream-ok"}

	mp := &MultiPitcher{
		Pitchers: []Pitcher{failing, working},
	}

	msg := homerun.Message{Title: "partial", Message: "test"}

	objectID, streamID, err := mp.Pitch(msg)
	if err != nil {
		t.Fatalf("expected success despite partial failure, got error: %v", err)
	}
	if objectID != "obj-ok" {
		t.Errorf("expected objectID obj-ok, got %s", objectID)
	}
	if streamID != "stream-ok" {
		t.Errorf("expected streamID stream-ok, got %s", streamID)
	}

	// Both backends must have been called.
	if got := failing.called.Load(); got != 1 {
		t.Errorf("expected failing backend called once, got %d", got)
	}
	if got := working.called.Load(); got != 1 {
		t.Errorf("expected working backend called once, got %d", got)
	}
}

func TestMultiPitcherAllFail(t *testing.T) {
	s1 := &stubPitcher{err: fmt.Errorf("error 1")}
	s2 := &stubPitcher{err: fmt.Errorf("error 2")}

	mp := &MultiPitcher{
		Pitchers: []Pitcher{s1, s2},
	}

	msg := homerun.Message{Title: "allfail", Message: "test"}

	_, _, err := mp.Pitch(msg)
	if err == nil {
		t.Fatal("expected error when all backends fail, got nil")
	}
}
