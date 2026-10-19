package handlers

import (
	"encoding/json"
	"html/template"
	"log/slog"
	"net/http"
	"strings"

	"github.com/stuttgart-things/homerun2-demo-pitcher/internal/scheduler"
)

// Micro-frontend fragments for homerun2-portal. The portal reads
// <base>/ui/manifest.json to learn which fragments a service offers and loads
// each <base>/ui/fragment/<id> with htmx into its overview. A fragment is bare
// HTML styled only by the portal's hr-* classes (static/homerun2.css in
// homerun2-portal); every URL in it carries the base path, because it is
// served on the portal's origin.

const (
	pitchesFragmentLimit   = 6
	pitchesFragmentRefresh = 5
)

// Fragment is one entry of the manifest.
type Fragment struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	URL            string `json:"url"`
	RefreshSeconds int    `json:"refreshSeconds"`
}

// Manifest lists the fragments the service offers to the portal.
type Manifest struct {
	Name      string     `json:"name"`
	Title     string     `json:"title"`
	Version   string     `json:"version"`
	BasePath  string     `json:"basePath"`
	Fragments []Fragment `json:"fragments"`
}

// ManifestHandler serves GET /ui/manifest.json.
func (h *WebHandlers) ManifestHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(Manifest{
		Name:     "homerun2-demo-pitcher",
		Title:    "Demo Pitcher",
		Version:  h.build.Version,
		BasePath: h.basePath,
		Fragments: []Fragment{{
			ID:             "pitches",
			Title:          "Pitches",
			URL:            h.basePath + "/ui/fragment/pitches",
			RefreshSeconds: pitchesFragmentRefresh,
		}},
	})
}

// html/template escapes every field: titles are generated or typed into the
// composer, and the portal shows the fragment to every viewer.
var pitchesFragment = template.Must(template.New("pitches").Funcs(template.FuncMap{
	"hrSeverity": hrSeverity,
	"upper":      strings.ToUpper,
}).Parse(`<div class="hr-frag">
{{- with .Stats}}<span class="hr-muted">Scheduler {{if .Running}}running every {{$.Interval}}{{else}}stopped{{end}} · {{.Pitched}} pitched
{{- if .Failed}}, {{.Failed}} failed{{end}}{{if not .LastPitchedAt.IsZero}}, last at {{.LastPitchedAt.Format "15:04:05"}}{{end}}</span>{{end -}}
<ul class="hr-list">
{{- range .Entries}}<li class="hr-item"><span class="hr-time">{{.Time}}</span><span class="hr-sev hr-sev-{{hrSeverity .Severity}}">{{upper .Severity}}</span><span class="hr-title">{{.Title}}</span>
{{- if eq .Status "sent"}}<span class="hr-system">sent</span>{{else}}<span class="hr-sev hr-sev-error">{{upper .Status}}</span>{{end}}</li>
{{- else}}<li class="hr-item hr-muted">Nothing sent from the composer yet.</li>{{end -}}
</ul><a class="hr-frag-more" href="{{.BasePath}}/">Open composer →</a></div>`))

// PitchesFragmentHandler serves GET /ui/fragment/pitches: the scheduler's
// state and the latest messages sent from the composer.
func (h *WebHandlers) PitchesFragmentHandler(w http.ResponseWriter, r *http.Request) {
	entries := h.sentLog.Entries()
	if len(entries) > pitchesFragmentLimit {
		entries = entries[:pitchesFragmentLimit]
	}
	data := struct {
		Stats    *scheduler.Stats // nil without a scheduler
		Interval string
		Entries  []SentEntry
		BasePath string
	}{Interval: h.interval, Entries: entries, BasePath: h.basePath}
	if h.sched != nil {
		stats := h.sched.GetStats()
		data.Stats = &stats
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pitchesFragment.Execute(w, data); err != nil {
		slog.Error("template render error", "error", err)
	}
}

// hrSeverity maps a severity to the portal's hr-sev-* classes.
func hrSeverity(severity string) string {
	switch s := strings.ToLower(severity); s {
	case "error", "critical":
		return "error"
	case "warning", "success", "debug":
		return s
	default:
		return "info"
	}
}
