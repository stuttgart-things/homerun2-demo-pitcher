package profile

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFromFile(t *testing.T) {
	content := []byte(`
name: test-profile
description: A test profile
severities:
  error: 10
  info: 90
systems:
  - system-a
  - system-b
authors:
  - author-1
tags:
  - tag-1
templates:
  error:
    titles:
      - "Error in {{.System}}"
    messages:
      - "Something went wrong in {{.System}}"
  info:
    titles:
      - "Info from {{.System}}"
    messages:
      - "All good in {{.System}}"
`)

	tmpFile, err := os.CreateTemp("", "profile-*.yaml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(content); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tmpFile.Close()

	p, err := LoadFromFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("LoadFromFile failed: %v", err)
	}

	if p.Name != "test-profile" {
		t.Errorf("expected name 'test-profile', got %q", p.Name)
	}
	if p.Description != "A test profile" {
		t.Errorf("expected description 'A test profile', got %q", p.Description)
	}
	if len(p.Severities) != 2 {
		t.Errorf("expected 2 severities, got %d", len(p.Severities))
	}
	if p.Severities["error"] != 10 {
		t.Errorf("expected error weight 10, got %d", p.Severities["error"])
	}
	if len(p.Systems) != 2 {
		t.Errorf("expected 2 systems, got %d", len(p.Systems))
	}
	if len(p.Authors) != 1 {
		t.Errorf("expected 1 author, got %d", len(p.Authors))
	}
	if len(p.Templates) != 2 {
		t.Errorf("expected 2 template groups, got %d", len(p.Templates))
	}
	if len(p.Templates["error"].Titles) != 1 {
		t.Errorf("expected 1 error title, got %d", len(p.Templates["error"].Titles))
	}
}

func TestLoadFromFile_NotFound(t *testing.T) {
	_, err := LoadFromFile("/nonexistent/profile.yaml")
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}
}

func TestLoadFromDir(t *testing.T) {
	dir, err := os.MkdirTemp("", "profiles-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(dir)

	profile1 := []byte(`
name: alpha
description: Alpha profile
severities:
  info: 100
systems:
  - sys-1
authors:
  - auth-1
`)
	profile2 := []byte(`
name: beta
description: Beta profile
severities:
  warning: 50
  error: 50
systems:
  - sys-2
authors:
  - auth-2
`)

	os.WriteFile(filepath.Join(dir, "alpha.yaml"), profile1, 0644)
	os.WriteFile(filepath.Join(dir, "beta.yml"), profile2, 0644)
	// non-yaml file should be ignored
	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("ignore me"), 0644)

	profiles, err := LoadFromDir(dir)
	if err != nil {
		t.Fatalf("LoadFromDir failed: %v", err)
	}

	if len(profiles) != 2 {
		t.Errorf("expected 2 profiles, got %d", len(profiles))
	}
	if _, ok := profiles["alpha"]; !ok {
		t.Error("expected profile 'alpha' to be loaded")
	}
	if _, ok := profiles["beta"]; !ok {
		t.Error("expected profile 'beta' to be loaded")
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		profile Profile
		wantErr bool
	}{
		{
			name: "valid profile",
			profile: Profile{
				Name:       "valid",
				Severities: map[string]int{"info": 100},
				Systems:    []string{"sys-1"},
				Authors:    []string{"auth-1"},
			},
			wantErr: false,
		},
		{
			name: "missing name",
			profile: Profile{
				Severities: map[string]int{"info": 100},
				Systems:    []string{"sys-1"},
				Authors:    []string{"auth-1"},
			},
			wantErr: true,
		},
		{
			name: "missing severities",
			profile: Profile{
				Name:    "no-sev",
				Systems: []string{"sys-1"},
				Authors: []string{"auth-1"},
			},
			wantErr: true,
		},
		{
			name: "missing systems",
			profile: Profile{
				Name:       "no-sys",
				Severities: map[string]int{"info": 100},
				Authors:    []string{"auth-1"},
			},
			wantErr: true,
		},
		{
			name: "missing authors",
			profile: Profile{
				Name:       "no-auth",
				Severities: map[string]int{"info": 100},
				Systems:    []string{"sys-1"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.profile.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestWeightedSeverity(t *testing.T) {
	p := &Profile{
		Name: "weight-test",
		Severities: map[string]int{
			"error": 10,
			"info":  90,
		},
		Systems: []string{"sys"},
		Authors: []string{"auth"},
	}

	counts := make(map[string]int)
	iterations := 10000
	for i := 0; i < iterations; i++ {
		sev := p.WeightedSeverity()
		counts[sev]++
	}

	// With weights error:10, info:90, we expect roughly 10% error and 90% info.
	// Allow a generous tolerance of 5 percentage points.
	errorPct := float64(counts["error"]) / float64(iterations) * 100
	infoPct := float64(counts["info"]) / float64(iterations) * 100

	if math.Abs(errorPct-10) > 5 {
		t.Errorf("error percentage %.1f%% is too far from expected 10%%", errorPct)
	}
	if math.Abs(infoPct-90) > 5 {
		t.Errorf("info percentage %.1f%% is too far from expected 90%%", infoPct)
	}

	// Ensure only expected severities appear
	for sev := range counts {
		if sev != "error" && sev != "info" {
			t.Errorf("unexpected severity %q returned", sev)
		}
	}
}

func TestDefaultProfile(t *testing.T) {
	p := DefaultProfile()

	if p.Name != "default" {
		t.Errorf("expected name 'default', got %q", p.Name)
	}

	if err := p.Validate(); err != nil {
		t.Errorf("default profile should be valid, got: %v", err)
	}

	expectedSeverities := []string{"error", "warning", "info", "success", "debug"}
	for _, sev := range expectedSeverities {
		if _, ok := p.Severities[sev]; !ok {
			t.Errorf("default profile missing severity %q", sev)
		}
	}

	if len(p.Systems) < 5 {
		t.Errorf("expected at least 5 systems, got %d", len(p.Systems))
	}

	if len(p.Authors) < 3 {
		t.Errorf("expected at least 3 authors, got %d", len(p.Authors))
	}

	// Check templates exist for each severity
	for _, sev := range expectedSeverities {
		tmpl, ok := p.Templates[sev]
		if !ok {
			t.Errorf("default profile missing template for severity %q", sev)
			continue
		}
		if len(tmpl.Titles) == 0 {
			t.Errorf("default profile has no titles for severity %q", sev)
		}
		if len(tmpl.Messages) == 0 {
			t.Errorf("default profile has no messages for severity %q", sev)
		}
	}
}
