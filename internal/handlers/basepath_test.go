package handlers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func serve(t *testing.T, h http.Handler, path string) (code int, body, location string) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, http.NoBody))
	b, _ := io.ReadAll(rec.Body)
	return rec.Code, string(b), rec.Header().Get("Location")
}

func TestMountUnderBasePath(t *testing.T) {
	app := http.NewServeMux()
	app.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "app:"+r.URL.Path) })
	health := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "healthy") })
	h := Mount("/pitch", app, health)

	for path, want := range map[string]string{
		"/pitch/":      "app:/",
		"/pitch/pitch": "app:/pitch", // the API moves with the prefix
		"/pitch/log":   "app:/log",
		"/health":      "healthy", // probes and the portal, at the root
	} {
		if code, body, _ := serve(t, h, path); code != http.StatusOK || body != want {
			t.Errorf("GET %s = %d %q, want %q", path, code, body, want)
		}
	}
	for _, path := range []string{"/", "/pitch"} {
		if code, _, loc := serve(t, h, path); code/100 != 3 || loc != "/pitch/" {
			t.Errorf("GET %s = %d → %q, want a redirect to /pitch/", path, code, loc)
		}
	}
	if code, _, _ := serve(t, h, "/log"); code != http.StatusNotFound {
		t.Errorf("GET /log without the prefix = %d, want 404", code)
	}
}

func TestMountWithoutBasePathServesAppAsIs(t *testing.T) {
	app := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "app:"+r.URL.Path) })
	if code, body, _ := serve(t, Mount("", app, http.NotFoundHandler()), "/log"); code != http.StatusOK || body != "app:/log" {
		t.Errorf("GET /log = %d %q", code, body)
	}
}

// Same-origin URLs in the page: attribute values that start with a single "/".
var rootURL = regexp.MustCompile(`(?:href|src|hx-get|hx-post|action)="(/[^/"][^"]*|/)"`)

func TestIndexPutsTheBasePathOnEveryURL(t *testing.T) {
	h := newTestWebHandlers(&mockPitcher{returnID: "obj-1"})
	h.SetBasePath("/pitch")
	rec := httptest.NewRecorder()
	h.IndexHandler(rec, httptest.NewRequest(http.MethodGet, "/", http.NoBody))
	page := rec.Body.String()

	for _, want := range []string{
		`src="/pitch/static/htmx.min.js"`, `hx-get="/pitch/composer/fields"`,
		`hx-post="/pitch/composer/send"`, `hx-get="/pitch/log"`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %s", want)
		}
	}
	for _, m := range rootURL.FindAllStringSubmatch(page, -1) {
		if !strings.HasPrefix(m[1], "/pitch/") {
			t.Errorf("page links outside /pitch/: %s", m[1])
		}
	}
}

func TestIndexWithoutBasePathIsUnchanged(t *testing.T) {
	h := newTestWebHandlers(&mockPitcher{returnID: "obj-1"})
	rec := httptest.NewRecorder()
	h.IndexHandler(rec, httptest.NewRequest(http.MethodGet, "/", http.NoBody))
	if page := rec.Body.String(); !strings.Contains(page, `src="/static/htmx.min.js"`) || !strings.Contains(page, `hx-get="/log"`) {
		t.Error("root URLs changed without a base path")
	}
}
