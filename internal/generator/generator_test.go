package generator

import (
	"math"
	"strings"
	"testing"
)

func TestGenerate(t *testing.T) {
	g := New(DefaultProfile())
	msg := g.Generate()

	if msg.Title == "" {
		t.Error("expected Title to be populated")
	}
	if msg.Message == "" {
		t.Error("expected Message to be populated")
	}
	if msg.Severity == "" {
		t.Error("expected Severity to be populated")
	}
	if msg.System == "" {
		t.Error("expected System to be populated")
	}
	if msg.Author == "" {
		t.Error("expected Author to be populated")
	}
	if msg.Timestamp == "" {
		t.Error("expected Timestamp to be populated")
	}
	if msg.Tags == "" {
		t.Error("expected Tags to be populated")
	}
	if msg.AssigneeName == "" {
		t.Error("expected AssigneeName to be populated")
	}
	if msg.AssigneeAddress == "" {
		t.Error("expected AssigneeAddress to be populated")
	}
	if msg.Url == "" {
		t.Error("expected Url to be populated")
	}
	if msg.Artifacts == "" {
		t.Error("expected Artifacts to be populated")
	}

	// Verify severity is one of the expected values.
	validSeverities := map[string]bool{"info": true, "warning": true, "critical": true}
	if !validSeverities[msg.Severity] {
		t.Errorf("unexpected severity %q", msg.Severity)
	}
}

func TestGenerateWithSeverity(t *testing.T) {
	g := New(DefaultProfile())

	for _, sev := range []string{"info", "warning", "critical"} {
		msg := g.GenerateWithSeverity(sev)
		if msg.Severity != sev {
			t.Errorf("expected severity %q, got %q", sev, msg.Severity)
		}
		if msg.Title == "" {
			t.Errorf("expected Title to be populated for severity %q", sev)
		}
		if msg.Message == "" {
			t.Errorf("expected Message to be populated for severity %q", sev)
		}
	}
}

func TestGenerateBatch(t *testing.T) {
	g := New(DefaultProfile())

	for _, n := range []int{0, 1, 5, 100} {
		msgs := g.GenerateBatch(n)
		if len(msgs) != n {
			t.Errorf("GenerateBatch(%d) returned %d messages", n, len(msgs))
		}
	}
}

func TestWeightedDistribution(t *testing.T) {
	g := New(DefaultProfile())

	counts := map[string]int{}
	total := 10000
	for range total {
		msg := g.Generate()
		counts[msg.Severity]++
	}

	// Expected weights: info=50, warning=30, critical=20 (total=100)
	// So expected ratios: info=0.50, warning=0.30, critical=0.20
	expected := map[string]float64{
		"info":     0.50,
		"warning":  0.30,
		"critical": 0.20,
	}

	for sev, expectedRatio := range expected {
		actualRatio := float64(counts[sev]) / float64(total)
		if math.Abs(actualRatio-expectedRatio) > 0.05 {
			t.Errorf("severity %q: expected ratio ~%.2f, got %.2f (count=%d/%d)",
				sev, expectedRatio, actualRatio, counts[sev], total)
		}
	}
}

func TestTemplateRendering(t *testing.T) {
	g := New(DefaultProfile())

	msg := g.GenerateWithSeverity("info")

	// Title should not contain unrendered template directives.
	if strings.Contains(msg.Title, "{{") {
		t.Errorf("title contains unrendered template: %q", msg.Title)
	}
	if strings.Contains(msg.Message, "{{") {
		t.Errorf("message contains unrendered template: %q", msg.Message)
	}

	// The rendered title should contain the system name (all info templates include {{.System}}).
	if msg.System != "" && !strings.Contains(msg.Title, msg.System) {
		t.Errorf("expected title to contain system %q, got %q", msg.System, msg.Title)
	}
}

func TestFallbackTemplates(t *testing.T) {
	// Profile with a severity that has no templates configured.
	p := &MessageProfile{
		Name:       "fallback-test",
		Severities: map[string]int{"unknown": 100},
		Systems:    []string{"test-system"},
		Authors:    []string{"test-author"},
		Tags:       []string{"test-tag"},
		Templates:  map[string]SeverityTemplate{},
	}
	g := New(p)

	msg := g.GenerateWithSeverity("unknown")
	if msg.Title == "" {
		t.Error("expected fallback title to be populated")
	}
	if msg.Message == "" {
		t.Error("expected fallback message to be populated")
	}
	if strings.Contains(msg.Title, "{{") {
		t.Errorf("fallback title contains unrendered template: %q", msg.Title)
	}
}

func TestEmptyPools(t *testing.T) {
	// Profile with empty optional pools.
	p := &MessageProfile{
		Name:       "minimal",
		Severities: map[string]int{"info": 100},
		Systems:    []string{"sys"},
		Authors:    []string{"auth"},
		Tags:       []string{},
		Assignees:  nil,
		URLs:       nil,
		Artifacts:  nil,
		Templates:  map[string]SeverityTemplate{},
	}
	g := New(p)

	msg := g.Generate()
	if msg.Tags != "" {
		t.Errorf("expected empty tags, got %q", msg.Tags)
	}
	if msg.AssigneeName != "" {
		t.Errorf("expected empty AssigneeName, got %q", msg.AssigneeName)
	}
	if msg.Url != "" {
		t.Errorf("expected empty Url, got %q", msg.Url)
	}
	if msg.Artifacts != "" {
		t.Errorf("expected empty Artifacts, got %q", msg.Artifacts)
	}
}
