package generator

import (
	"bytes"
	"math/rand/v2"
	"strings"
	"text/template"
	"time"

	homerun "github.com/stuttgart-things/homerun-library/v3"
)

// MessageProfile provides the data needed for message generation.
// This mirrors profile.Profile but avoids a circular dependency.
type MessageProfile struct {
	Name       string
	Severities map[string]int              // severity -> weight
	Systems    []string
	Authors    []string
	Tags       []string
	Assignees  []Assignee
	URLs       []string
	Artifacts  []string
	Templates  map[string]SeverityTemplate // severity -> templates
}

// SeverityTemplate holds title and message templates for a severity level.
type SeverityTemplate struct {
	Titles   []string
	Messages []string
}

// Assignee represents a notification recipient.
type Assignee struct {
	Name    string
	Address string
}

// templateData is passed into Go templates during rendering.
type templateData struct {
	System    string
	Author    string
	Severity  string
	Timestamp string
}

// Generator creates random messages from a profile.
type Generator struct {
	profile *MessageProfile
}

// New creates a Generator for the given profile.
func New(p *MessageProfile) *Generator {
	return &Generator{profile: p}
}

// Generate creates a single random message according to the profile.
// It picks a severity using weighted random selection, then delegates
// to GenerateWithSeverity.
func (g *Generator) Generate() homerun.Message {
	severity := g.pickWeightedSeverity()
	return g.GenerateWithSeverity(severity)
}

// GenerateBatch creates n random messages.
func (g *Generator) GenerateBatch(n int) []homerun.Message {
	msgs := make([]homerun.Message, n)
	for i := range n {
		msgs[i] = g.Generate()
	}
	return msgs
}

// GenerateWithSeverity creates a message with a specific severity.
func (g *Generator) GenerateWithSeverity(severity string) homerun.Message {
	now := time.Now().Format(time.RFC3339)
	system := pickRandom(g.profile.Systems)
	author := pickRandom(g.profile.Authors)

	data := templateData{
		System:    system,
		Author:    author,
		Severity:  severity,
		Timestamp: now,
	}

	title := g.renderTemplate(g.pickTitleTemplate(severity), data)
	message := g.renderTemplate(g.pickMessageTemplate(severity), data)

	// Pick 1-3 random tags, comma-joined.
	tags := g.pickTags()

	msg := homerun.Message{
		Title:     title,
		Message:   message,
		Severity:  severity,
		System:    system,
		Author:    author,
		Timestamp: now,
		Tags:      tags,
	}

	if len(g.profile.Assignees) > 0 {
		a := g.profile.Assignees[rand.IntN(len(g.profile.Assignees))]
		msg.AssigneeName = a.Name
		msg.AssigneeAddress = a.Address
	}

	if len(g.profile.URLs) > 0 {
		msg.Url = g.profile.URLs[rand.IntN(len(g.profile.URLs))]
	}

	if len(g.profile.Artifacts) > 0 {
		msg.Artifacts = g.profile.Artifacts[rand.IntN(len(g.profile.Artifacts))]
	}

	return msg
}

// pickWeightedSeverity selects a severity based on configured weights.
func (g *Generator) pickWeightedSeverity() string {
	totalWeight := 0
	for _, w := range g.profile.Severities {
		totalWeight += w
	}
	if totalWeight == 0 {
		// Fallback: pick any severity key.
		for sev := range g.profile.Severities {
			return sev
		}
		return "info"
	}

	r := rand.IntN(totalWeight)
	for sev, w := range g.profile.Severities {
		r -= w
		if r < 0 {
			return sev
		}
	}
	// Should not reach here, but return a default.
	return "info"
}

func (g *Generator) pickTitleTemplate(severity string) string {
	if st, ok := g.profile.Templates[severity]; ok && len(st.Titles) > 0 {
		return st.Titles[rand.IntN(len(st.Titles))]
	}
	// Fallback generic titles.
	fallback := []string{
		"{{.Severity}} event on {{.System}}",
		"[{{.Severity}}] Alert from {{.System}}",
	}
	return fallback[rand.IntN(len(fallback))]
}

func (g *Generator) pickMessageTemplate(severity string) string {
	if st, ok := g.profile.Templates[severity]; ok && len(st.Messages) > 0 {
		return st.Messages[rand.IntN(len(st.Messages))]
	}
	// Fallback generic messages.
	fallback := []string{
		"{{.Severity}} level event detected on {{.System}} by {{.Author}} at {{.Timestamp}}",
		"System {{.System}} reported a {{.Severity}} event",
	}
	return fallback[rand.IntN(len(fallback))]
}

func (g *Generator) renderTemplate(tmplStr string, data templateData) string {
	t, err := template.New("msg").Parse(tmplStr)
	if err != nil {
		return tmplStr
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return tmplStr
	}
	return buf.String()
}

func (g *Generator) pickTags() string {
	if len(g.profile.Tags) == 0 {
		return ""
	}
	count := rand.IntN(3) + 1 // 1-3 tags
	if count > len(g.profile.Tags) {
		count = len(g.profile.Tags)
	}
	// Shuffle a copy and take the first `count` elements.
	pool := make([]string, len(g.profile.Tags))
	copy(pool, g.profile.Tags)
	rand.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
	return strings.Join(pool[:count], ",")
}

func pickRandom(pool []string) string {
	if len(pool) == 0 {
		return ""
	}
	return pool[rand.IntN(len(pool))]
}

// DefaultProfile returns a sensible default MessageProfile for testing.
func DefaultProfile() *MessageProfile {
	return &MessageProfile{
		Name: "default-test-profile",
		Severities: map[string]int{
			"info":     50,
			"warning":  30,
			"critical": 20,
		},
		Systems: []string{
			"api-gateway",
			"auth-service",
			"payment-engine",
			"notification-hub",
		},
		Authors: []string{
			"monitoring-bot",
			"deploy-pipeline",
			"health-checker",
		},
		Tags: []string{
			"infrastructure",
			"security",
			"performance",
			"deployment",
			"database",
		},
		Assignees: []Assignee{
			{Name: "alice", Address: "alice@example.com"},
			{Name: "bob", Address: "bob@example.com"},
		},
		URLs: []string{
			"https://dashboard.example.com/incidents/123",
			"https://grafana.example.com/d/abc",
		},
		Artifacts: []string{
			"build-2024-001.tar.gz",
			"report-scan-latest.pdf",
		},
		Templates: map[string]SeverityTemplate{
			"info": {
				Titles: []string{
					"Info: {{.System}} health check passed",
					"[INFO] {{.System}} deployment complete",
				},
				Messages: []string{
					"Routine check on {{.System}} completed successfully by {{.Author}} at {{.Timestamp}}",
					"{{.Author}} reports {{.System}} is operating normally",
				},
			},
			"warning": {
				Titles: []string{
					"Warning: {{.System}} latency spike",
					"[WARN] {{.System}} resource usage high",
				},
				Messages: []string{
					"{{.System}} showing elevated latency, detected by {{.Author}} at {{.Timestamp}}",
					"Resource threshold exceeded on {{.System}}",
				},
			},
			"critical": {
				Titles: []string{
					"CRITICAL: {{.System}} is down",
					"[CRITICAL] {{.System}} unresponsive",
				},
				Messages: []string{
					"{{.System}} is unreachable since {{.Timestamp}}, reported by {{.Author}}",
					"Immediate attention required: {{.System}} failure detected",
				},
			},
		},
	}
}
