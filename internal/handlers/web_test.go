package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	homerun "github.com/stuttgart-things/homerun-library/v3"

	"github.com/stuttgart-things/homerun2-demo-pitcher/internal/generator"
	"github.com/stuttgart-things/homerun2-demo-pitcher/internal/pitcher"
	"github.com/stuttgart-things/homerun2-demo-pitcher/internal/web"
)

// mockPitcher records calls to Pitch for verification in tests.
type mockPitcher struct {
	calls    []homerun.Message
	returnID string
	err      error
}

func (m *mockPitcher) Pitch(msg homerun.Message) (string, string, error) {
	m.calls = append(m.calls, msg)
	return m.returnID, "stream-1", m.err
}

// newTestWebHandlers creates WebHandlers wired with a mock pitcher and minimal generator.
func newTestWebHandlers(mp *mockPitcher) *WebHandlers {
	gen := generator.New(generator.DefaultProfile())
	pitchers := map[string]pitcher.Pitcher{
		"redis": mp,
	}
	build := BuildInfo{Version: "test", Commit: "abc1234", Date: "2026-01-01"}

	return NewWebHandlers(
		web.TemplateFS,
		gen,
		pitchers,
		nil, // no scheduler in tests
		"10s",
		"http://localhost:4000",
		build,
	)
}

func TestIndexHandler(t *testing.T) {
	mp := &mockPitcher{returnID: "obj-1"}
	h := newTestWebHandlers(mp)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	h.IndexHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Fatalf("expected text/html content type, got %q", ct)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "HOMERUN") {
		t.Fatalf("expected body to contain HOMERUN, got %q", body[:min(200, len(body))])
	}
}

func TestIndexHandler_NotFoundForOtherPaths(t *testing.T) {
	mp := &mockPitcher{returnID: "obj-1"}
	h := newTestWebHandlers(mp)

	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	rec := httptest.NewRecorder()

	h.IndexHandler(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestComposerFieldsHandler(t *testing.T) {
	mp := &mockPitcher{returnID: "obj-1"}
	h := newTestWebHandlers(mp)

	severities := []string{"info", "warning", "error", "success", "debug", ""}
	for _, sev := range severities {
		t.Run("severity="+sev, func(t *testing.T) {
			target := "/composer/fields"
			if sev != "" {
				target += "?severity=" + sev
			}
			req := httptest.NewRequest(http.MethodGet, target, nil)
			rec := httptest.NewRecorder()

			h.ComposerFieldsHandler(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200 for severity %q, got %d", sev, rec.Code)
			}

			ct := rec.Header().Get("Content-Type")
			if !strings.Contains(ct, "text/html") {
				t.Fatalf("expected text/html, got %q", ct)
			}

			body := rec.Body.String()
			// The fields partial should contain form inputs.
			if !strings.Contains(body, "name=\"title\"") {
				t.Fatalf("expected fields partial to contain title input, got %q", body[:min(200, len(body))])
			}
		})
	}
}

func TestComposerSendHandler(t *testing.T) {
	mp := &mockPitcher{returnID: "obj-1"}
	h := newTestWebHandlers(mp)

	form := url.Values{
		"title":    {"Test Title"},
		"message":  {"Test message body"},
		"severity": {"info"},
		"system":   {"test-system"},
		"author":   {"test-author"},
		"tags":     {"tag1,tag2"},
		"target":   {"redis"},
	}

	req := httptest.NewRequest(http.MethodPost, "/composer/send", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	h.ComposerSendHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	if len(mp.calls) != 1 {
		t.Fatalf("expected 1 pitch call, got %d", len(mp.calls))
	}

	if mp.calls[0].Title != "Test Title" {
		t.Fatalf("expected title %q, got %q", "Test Title", mp.calls[0].Title)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "success") {
		t.Fatalf("expected toast to contain 'success', got %q", body)
	}

	// Verify HX-Trigger header is set for log refresh.
	if rec.Header().Get("HX-Trigger") != "sentMessage" {
		t.Fatalf("expected HX-Trigger=sentMessage, got %q", rec.Header().Get("HX-Trigger"))
	}
}

func TestComposerSendHandler_MethodNotAllowed(t *testing.T) {
	mp := &mockPitcher{returnID: "obj-1"}
	h := newTestWebHandlers(mp)

	req := httptest.NewRequest(http.MethodGet, "/composer/send", nil)
	rec := httptest.NewRecorder()

	h.ComposerSendHandler(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestComposerSendHandler_PitchError(t *testing.T) {
	mp := &mockPitcher{err: errPitchFailed}
	h := newTestWebHandlers(mp)

	form := url.Values{
		"title":   {"Test"},
		"message": {"Body"},
	}

	req := httptest.NewRequest(http.MethodPost, "/composer/send", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	h.ComposerSendHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 (toast rendered), got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "error") {
		t.Fatalf("expected toast to contain 'error', got %q", body)
	}
}

func TestSentLogHandler(t *testing.T) {
	mp := &mockPitcher{returnID: "obj-1"}
	h := newTestWebHandlers(mp)

	// Add some entries to the log.
	h.sentLog.Add(SentEntry{
		Time:     "12:00:00",
		Title:    "Test Message",
		Severity: "info",
		Status:   "sent",
	})

	req := httptest.NewRequest(http.MethodGet, "/log", nil)
	rec := httptest.NewRecorder()

	h.SentLogHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Fatalf("expected text/html, got %q", ct)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Test Message") {
		t.Fatalf("expected log to contain 'Test Message', got %q", body)
	}
	if !strings.Contains(body, "<table>") {
		t.Fatalf("expected log to contain table, got %q", body)
	}
}

func TestSentLogHandler_Empty(t *testing.T) {
	mp := &mockPitcher{returnID: "obj-1"}
	h := newTestWebHandlers(mp)

	req := httptest.NewRequest(http.MethodGet, "/log", nil)
	rec := httptest.NewRecorder()

	h.SentLogHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "No messages sent yet") {
		t.Fatalf("expected empty log message, got %q", body)
	}
}
