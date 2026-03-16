package handlers

import (
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/stuttgart-things/homerun2-demo-pitcher/internal/generator"
	"github.com/stuttgart-things/homerun2-demo-pitcher/internal/pitcher"
	"github.com/stuttgart-things/homerun2-demo-pitcher/internal/scheduler"

	homerun "github.com/stuttgart-things/homerun-library/v3"
)

// SentEntry records a sent message for the log.
type SentEntry struct {
	Time     string
	Title    string
	Severity string
	Status   string
}

// SentLog is a thread-safe, bounded log of sent messages.
type SentLog struct {
	mu      sync.RWMutex
	entries []SentEntry
	max     int
}

// NewSentLog creates a new SentLog with the given capacity.
func NewSentLog(max int) *SentLog {
	return &SentLog{max: max}
}

// Add records a new entry at the front of the log.
func (l *SentLog) Add(e SentEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append([]SentEntry{e}, l.entries...)
	if len(l.entries) > l.max {
		l.entries = l.entries[:l.max]
	}
}

// Entries returns a copy of all entries.
func (l *SentLog) Entries() []SentEntry {
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make([]SentEntry, len(l.entries))
	copy(out, l.entries)
	return out
}

// FieldsData holds pre-filled form field values.
type FieldsData struct {
	Title          string
	Message        string
	System         string
	Author         string
	Tags           string
	AssigneeName   string
	AssigneeAddress string
	Artifacts      string
	Url            string
}

// IndexData is the template data for the main page.
type IndexData struct {
	CurrentSeverity   string
	Fields            FieldsData
	SentLog           LogData
	SchedulerRunning  bool
	SchedulerInterval string
	SchedulerPitched  int64
	TargetURL         string
	Version           string
	Commit            string
	Date              string
}

// LogData holds entries for the log partial.
type LogData struct {
	Entries []SentEntry
}

// ToastData holds data for the toast partial.
type ToastData struct {
	Status  string
	Message string
}

// WebHandlers groups the web UI handlers and their shared state.
type WebHandlers struct {
	templates        *template.Template
	gen              *generator.Generator
	pitchers         map[string]pitcher.Pitcher // "redis", "omni-pitcher", "both"
	sentLog          *SentLog
	sched            *scheduler.Scheduler
	interval         string
	defaultTargetURL string
	build            BuildInfo
}

// NewWebHandlers creates the web handler group.
func NewWebHandlers(
	tmplFS fs.FS,
	gen *generator.Generator,
	pitchers map[string]pitcher.Pitcher,
	sched *scheduler.Scheduler,
	interval string,
	defaultTargetURL string,
	build BuildInfo,
) *WebHandlers {
	tmpl := template.Must(template.ParseFS(tmplFS, "templates/*.html"))
	return &WebHandlers{
		templates:        tmpl,
		gen:              gen,
		pitchers:         pitchers,
		sentLog:          NewSentLog(100),
		sched:            sched,
		interval:         interval,
		defaultTargetURL: defaultTargetURL,
		build:            build,
	}
}

// IndexHandler serves the main page.
func (h *WebHandlers) IndexHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	severity := "info"
	msg := h.gen.GenerateWithSeverity(severity)
	fields := messageToFields(msg)

	var stats scheduler.Stats
	if h.sched != nil {
		stats = h.sched.GetStats()
	}

	data := IndexData{
		CurrentSeverity:   severity,
		Fields:            fields,
		SentLog:           LogData{Entries: h.sentLog.Entries()},
		SchedulerRunning:  stats.Running,
		SchedulerInterval: h.interval,
		SchedulerPitched:  stats.Pitched,
		TargetURL:         h.defaultTargetURL,
		Version:           h.build.Version,
		Commit:            h.build.Commit,
		Date:              h.build.Date,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.ExecuteTemplate(w, "index.html", data); err != nil {
		slog.Error("template render error", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// ComposerFieldsHandler returns the fields partial with random values for the given severity.
func (h *WebHandlers) ComposerFieldsHandler(w http.ResponseWriter, r *http.Request) {
	severity := r.URL.Query().Get("severity")
	if severity == "" {
		severity = "info"
	}

	msg := h.gen.GenerateWithSeverity(severity)
	fields := messageToFields(msg)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.ExecuteTemplate(w, "fields.html", fields); err != nil {
		slog.Error("template render error", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// ComposerSendHandler processes the composer form submission, pitches the message, and returns a toast.
func (h *WebHandlers) ComposerSendHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.renderToast(w, "error", "Failed to parse form")
		return
	}

	msg := homerun.Message{
		Title:          r.FormValue("title"),
		Message:        r.FormValue("message"),
		Severity:       r.FormValue("severity"),
		System:         r.FormValue("system"),
		Author:         r.FormValue("author"),
		Tags:           r.FormValue("tags"),
		AssigneeName:   r.FormValue("assignee_name"),
		AssigneeAddress: r.FormValue("assignee_address"),
		Artifacts:      r.FormValue("artifacts"),
		Url:            r.FormValue("url"),
		Timestamp:      time.Now().Format(time.RFC3339),
	}

	targetURL := r.FormValue("target_url")

	// Determine which pitcher to use.
	var p pitcher.Pitcher
	if targetURL != "" {
		// User specified a URL — create an ad-hoc HTTP pitcher.
		apiPath := r.FormValue("api_path")
		if apiPath == "" {
			apiPath = homerun.GetEnv("OMNI_PITCHER_API_PATH", "generic")
		}
		p = &pitcher.HTTPPitcher{
			Endpoint:   targetURL,
			APIPath:    apiPath,
			HTTPClient: pitcher.DefaultHTTPClient(),
		}
	} else {
		// Fall back to pre-configured pitchers map.
		target := r.FormValue("target")
		if target == "" {
			target = "redis"
		}
		var ok bool
		p, ok = h.pitchers[target]
		if !ok {
			for _, v := range h.pitchers {
				p = v
				break
			}
		}
	}

	if p == nil {
		h.renderToast(w, "error", "No pitcher backend configured")
		return
	}

	_, _, err := p.Pitch(msg)
	if err != nil {
		slog.Error("failed to pitch message from composer", "error", err)
		h.sentLog.Add(SentEntry{
			Time:     time.Now().Format("15:04:05"),
			Title:    msg.Title,
			Severity: msg.Severity,
			Status:   "failed",
		})
		w.Header().Set("HX-Trigger", "sentMessage")
		h.renderToast(w, "error", "Pitch failed: "+err.Error())
		return
	}

	h.sentLog.Add(SentEntry{
		Time:     time.Now().Format("15:04:05"),
		Title:    msg.Title,
		Severity: msg.Severity,
		Status:   "sent",
	})

	logTarget := targetURL
	if logTarget == "" {
		logTarget = "configured-backend"
	}
	slog.Info("message pitched from composer", "title", msg.Title, "severity", msg.Severity, "target", logTarget)
	w.Header().Set("HX-Trigger", "sentMessage")
	h.renderToast(w, "success", "Message pitched successfully")
}

// SentLogHandler returns the sent log partial.
func (h *WebHandlers) SentLogHandler(w http.ResponseWriter, r *http.Request) {
	data := LogData{Entries: h.sentLog.Entries()}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.ExecuteTemplate(w, "log.html", data); err != nil {
		slog.Error("template render error", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func (h *WebHandlers) renderToast(w http.ResponseWriter, status, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.ExecuteTemplate(w, "toast.html", ToastData{Status: status, Message: message}); err != nil {
		slog.Error("template render error", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func messageToFields(msg homerun.Message) FieldsData {
	return FieldsData{
		Title:          msg.Title,
		Message:        msg.Message,
		System:         msg.System,
		Author:         msg.Author,
		Tags:           msg.Tags,
		AssigneeName:   msg.AssigneeName,
		AssigneeAddress: msg.AssigneeAddress,
		Artifacts:      msg.Artifacts,
		Url:            msg.Url,
	}
}
