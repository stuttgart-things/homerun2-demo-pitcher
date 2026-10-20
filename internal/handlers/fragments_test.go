package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stuttgart-things/homerun2-demo-pitcher/internal/scheduler"
)

func TestManifestListsThePitchesFragment(t *testing.T) {
	h := newTestWebHandlers(&mockPitcher{})
	h.SetBasePath("/pitch")

	rec := httptest.NewRecorder()
	h.ManifestHandler(rec, httptest.NewRequest(http.MethodGet, "/ui/manifest.json", nil))
	var m Manifest
	if err := json.NewDecoder(rec.Body).Decode(&m); err != nil {
		t.Fatal(err)
	}
	if m.Version != "test" || m.BasePath != "/pitch" || len(m.Fragments) != 1 {
		t.Fatalf("manifest = %+v", m)
	}
	if f := m.Fragments[0]; f.ID != "pitches" || f.URL != "/pitch/ui/fragment/pitches" || f.RefreshSeconds < 2 {
		t.Errorf("fragment = %+v", f)
	}
}

func TestPitchesFragment(t *testing.T) {
	h := newTestWebHandlers(&mockPitcher{})
	h.SetBasePath("/pitch")
	for i := range 8 {
		h.sentLog.Add(SentEntry{Time: "12:00:00", Title: fmt.Sprintf("old %d", i), Severity: "info", Status: "sent"})
	}
	h.sentLog.Add(SentEntry{Time: "12:00:01", Title: "<b>disk</b> full", Severity: "ERROR", Status: "failed"})

	rec := httptest.NewRecorder()
	h.PitchesFragmentHandler(rec, httptest.NewRequest(http.MethodGet, "/ui/fragment/pitches", nil))
	body := rec.Body.String()

	if strings.Contains(body, "<html") || strings.Contains(body, "<script") {
		t.Fatal("a fragment is bare HTML")
	}
	if n := strings.Count(body, `<li class="hr-item">`); n != pitchesFragmentLimit {
		t.Errorf("%d items, want %d", n, pitchesFragmentLimit)
	}
	for _, want := range []string{
		`<span class="hr-sev hr-sev-error">ERROR</span>`,
		"&lt;b&gt;disk&lt;/b&gt; full", // titles are escaped
		`<span class="hr-sev hr-sev-error">FAILED</span>`,
		`<span class="hr-system">sent</span>`,
		`href="/pitch/"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("fragment lacks %s:\n%s", want, body)
		}
	}
	if strings.Contains(body, "Scheduler") {
		t.Error("the fragment reports a scheduler that is not configured")
	}
	if i := strings.Index(body, "disk"); i < 0 || i > strings.Index(body, "old") {
		t.Error("the newest entry is not first")
	}
}

func TestPitchesFragmentEmpty(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestWebHandlers(&mockPitcher{}).PitchesFragmentHandler(rec, httptest.NewRequest(http.MethodGet, "/ui/fragment/pitches", nil))
	if body := rec.Body.String(); !strings.Contains(body, "Nothing sent from the composer yet.") || !strings.Contains(body, `href="/"`) {
		t.Errorf("empty fragment = %s", body)
	}
}

func TestPitchesFragmentShowsTheScheduler(t *testing.T) {
	h := newTestWebHandlers(&mockPitcher{})
	h.sched = scheduler.New(scheduler.Config{}, nil, nil)

	rec := httptest.NewRecorder()
	h.PitchesFragmentHandler(rec, httptest.NewRequest(http.MethodGet, "/ui/fragment/pitches", nil))
	if body := rec.Body.String(); !strings.Contains(body, "Scheduler stopped · 0 pitched</span>") {
		t.Errorf("fragment lacks the scheduler state:\n%s", body)
	}
}
