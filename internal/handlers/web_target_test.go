package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/stuttgart-things/homerun2-demo-pitcher/internal/generator"
	"github.com/stuttgart-things/homerun2-demo-pitcher/internal/pitcher"
	"github.com/stuttgart-things/homerun2-demo-pitcher/internal/web"
)

// targetServer records the Authorization header of every request it gets.
type targetServer struct {
	*httptest.Server
	mu    sync.Mutex
	auths []string
}

func newTargetServer(t *testing.T) *targetServer {
	t.Helper()
	ts := &targetServer{}
	ts.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ts.mu.Lock()
		ts.auths = append(ts.auths, r.Header.Get("Authorization"))
		ts.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"objectId":"obj-1","streamId":"s-1"}`))
	}))
	t.Cleanup(ts.Close)
	return ts
}

func (ts *targetServer) hits() []string {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return append([]string(nil), ts.auths...)
}

func newTargetWebHandlers(mp *mockPitcher, defaultTarget string) *WebHandlers {
	return NewWebHandlers(
		web.TemplateFS,
		generator.New(generator.DefaultProfile()),
		map[string]pitcher.Pitcher{"redis": mp},
		nil,
		"10s",
		defaultTarget,
		BuildInfo{Version: "test"},
	)
}

func postComposer(h *WebHandlers, targetURL string) *httptest.ResponseRecorder {
	form := url.Values{"title": {"t"}, "message": {"m"}, "severity": {"info"}, "target_url": {targetURL}}
	req := httptest.NewRequest(http.MethodPost, "/composer/send", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ComposerSendHandler(rec, req)
	return rec
}

// #65: a target_url from the form that is not configured gets no request at
// all -- above all not one carrying AUTH_TOKEN.
func TestComposerSend_UnknownTargetURLRejected(t *testing.T) {
	t.Setenv("AUTH_TOKEN", "secret-token")
	attacker := newTargetServer(t)
	mp := &mockPitcher{returnID: "obj-1"}
	h := newTargetWebHandlers(mp, "http://omni.invalid")

	rec := postComposer(h, attacker.URL)

	if n := len(attacker.hits()); n != 0 {
		t.Fatalf("expected no request to the unconfigured target, got %d", n)
	}
	if len(mp.calls) != 0 {
		t.Fatalf("expected no fallback pitch, got %d", len(mp.calls))
	}
	if body := rec.Body.String(); !strings.Contains(body, "not allowed") {
		t.Fatalf("expected a 'not allowed' toast, got %q", body)
	}
}

func TestComposerSend_DefaultTargetGetsToken(t *testing.T) {
	t.Setenv("AUTH_TOKEN", "secret-token")
	omni := newTargetServer(t)
	h := newTargetWebHandlers(&mockPitcher{}, omni.URL)

	// Trailing slash and blanks as a browser might send them.
	rec := postComposer(h, " "+omni.URL+"/ ")

	hits := omni.hits()
	if len(hits) != 1 || hits[0] != "Bearer secret-token" {
		t.Fatalf("expected one request with the token, got %q", hits)
	}
	if body := rec.Body.String(); !strings.Contains(body, "success") {
		t.Fatalf("expected a success toast, got %q", body)
	}
}

func TestComposerSend_AllowTargetURLs(t *testing.T) {
	t.Setenv("AUTH_TOKEN", "secret-token")
	other := newTargetServer(t)
	h := newTargetWebHandlers(&mockPitcher{}, "http://omni.invalid")
	h.AllowTargetURLs("", other.URL+"/")

	postComposer(h, other.URL)

	if n := len(other.hits()); n != 1 {
		t.Fatalf("expected one request to the allowed target, got %d", n)
	}
}
