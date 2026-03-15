package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stuttgart-things/homerun2-demo-pitcher/internal/models"
)

var errPitchFailed = errors.New("pitch backend unavailable")

func TestHealthHandler(t *testing.T) {
	info := BuildInfo{Version: "1.0.0", Commit: "abc1234", Date: "2026-01-01"}
	handler := NewHealthHandler(info)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Fatalf("expected application/json, got %q", ct)
	}

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if body["status"] != "healthy" {
		t.Fatalf("expected status=healthy, got %q", body["status"])
	}
	if body["version"] != "1.0.0" {
		t.Fatalf("expected version=1.0.0, got %q", body["version"])
	}
	if body["commit"] != "abc1234" {
		t.Fatalf("expected commit=abc1234, got %q", body["commit"])
	}
	if body["date"] != "2026-01-01" {
		t.Fatalf("expected date=2026-01-01, got %q", body["date"])
	}
	if body["time"] == "" {
		t.Fatal("expected time to be set")
	}
}

func TestHealthHandler_MethodNotAllowed(t *testing.T) {
	info := BuildInfo{Version: "1.0.0", Commit: "abc1234", Date: "2026-01-01"}
	handler := NewHealthHandler(info)

	req := httptest.NewRequest(http.MethodPost, "/health", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestPitchHandler_ValidMessage(t *testing.T) {
	mp := &mockPitcher{returnID: "obj-42"}
	handler := NewPitchHandler(mp)

	body := `{"title":"Test Alert","message":"Something happened","severity":"error","author":"tester"}`
	req := httptest.NewRequest(http.MethodPost, "/pitch", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp models.PitchResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "success" {
		t.Fatalf("expected status=success, got %q", resp.Status)
	}
	if resp.ObjectID != "obj-42" {
		t.Fatalf("expected objectID=obj-42, got %q", resp.ObjectID)
	}
	if len(mp.calls) != 1 {
		t.Fatalf("expected 1 pitch call, got %d", len(mp.calls))
	}
}

func TestPitchHandler_MissingTitle(t *testing.T) {
	mp := &mockPitcher{returnID: "obj-1"}
	handler := NewPitchHandler(mp)

	body := `{"message":"Something happened"}`
	req := httptest.NewRequest(http.MethodPost, "/pitch", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}

	var resp models.PitchResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "error" {
		t.Fatalf("expected status=error, got %q", resp.Status)
	}
	if !strings.Contains(resp.Message, "Title") {
		t.Fatalf("expected error about title, got %q", resp.Message)
	}
}

func TestPitchHandler_MissingMessage(t *testing.T) {
	mp := &mockPitcher{returnID: "obj-1"}
	handler := NewPitchHandler(mp)

	body := `{"title":"Test Alert"}`
	req := httptest.NewRequest(http.MethodPost, "/pitch", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}

	var resp models.PitchResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !strings.Contains(resp.Message, "Message") {
		t.Fatalf("expected error about message, got %q", resp.Message)
	}
}

func TestPitchHandler_WrongMethod(t *testing.T) {
	mp := &mockPitcher{returnID: "obj-1"}
	handler := NewPitchHandler(mp)

	req := httptest.NewRequest(http.MethodGet, "/pitch", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestPitchHandler_InvalidJSON(t *testing.T) {
	mp := &mockPitcher{returnID: "obj-1"}
	handler := NewPitchHandler(mp)

	req := httptest.NewRequest(http.MethodPost, "/pitch", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestPitchHandler_PitchError(t *testing.T) {
	mp := &mockPitcher{err: errPitchFailed}
	handler := NewPitchHandler(mp)

	body := `{"title":"Test","message":"Body"}`
	req := httptest.NewRequest(http.MethodPost, "/pitch", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestPitchHandlerDefaults(t *testing.T) {
	mp := &mockPitcher{returnID: "obj-1"}
	handler := NewPitchHandler(mp)

	// Send only title and message, omit severity and author.
	body := `{"title":"Minimal Alert","message":"Something happened"}`
	req := httptest.NewRequest(http.MethodPost, "/pitch", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	if len(mp.calls) != 1 {
		t.Fatalf("expected 1 pitch call, got %d", len(mp.calls))
	}

	msg := mp.calls[0]

	if msg.Severity != "info" {
		t.Fatalf("expected default severity=info, got %q", msg.Severity)
	}
	if msg.Author != "unknown" {
		t.Fatalf("expected default author=unknown, got %q", msg.Author)
	}
	if msg.Timestamp == "" {
		t.Fatal("expected timestamp to be set")
	}
	if msg.System != "homerun2-demo-pitcher" {
		t.Fatalf("expected default system=homerun2-demo-pitcher, got %q", msg.System)
	}
}
