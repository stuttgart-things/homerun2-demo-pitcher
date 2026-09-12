package config

import (
	"strings"
	"testing"
)

func TestLoadRedisConfig(t *testing.T) {
	// Ensure defaults are returned when no env vars are set.
	// Note: these tests rely on REDIS_ADDR etc. NOT being set in the test env.
	cfg := LoadRedisConfig()

	if cfg.Addr != "localhost" {
		t.Fatalf("expected default addr=localhost, got %q", cfg.Addr)
	}
	if cfg.Port != "6379" {
		t.Fatalf("expected default port=6379, got %q", cfg.Port)
	}
	if cfg.Password != "" {
		t.Fatalf("expected default password to be empty, got %q", cfg.Password)
	}
	if cfg.Stream != "homerun" {
		t.Fatalf("expected default stream=homerun, got %q", cfg.Stream)
	}
}

func TestLoadRedisConfig_WithEnv(t *testing.T) {
	t.Setenv("REDIS_ADDR", "redis.example.com")
	t.Setenv("REDIS_PORT", "6380")
	t.Setenv("REDIS_PASSWORD", "secret")
	t.Setenv("REDIS_STREAM", "custom-stream")

	cfg := LoadRedisConfig()

	if cfg.Addr != "redis.example.com" {
		t.Fatalf("expected addr=redis.example.com, got %q", cfg.Addr)
	}
	if cfg.Port != "6380" {
		t.Fatalf("expected port=6380, got %q", cfg.Port)
	}
	if cfg.Password != "secret" {
		t.Fatalf("expected password=secret, got %q", cfg.Password)
	}
	if cfg.Stream != "custom-stream" {
		t.Fatalf("expected stream=custom-stream, got %q", cfg.Stream)
	}
}

func TestSetupLogging(t *testing.T) {
	// Verify SetupLogging does not panic for each combination of format and level.
	formats := []string{"json", "text", "JSON", "TEXT"}
	levels := []string{"debug", "info", "warn", "error", "DEBUG", "INFO", "WARN", "ERROR", "unknown"}

	for _, format := range formats {
		for _, level := range levels {
			t.Run(format+"/"+level, func(t *testing.T) {
				t.Setenv("LOG_FORMAT", format)
				t.Setenv("LOG_LEVEL", level)

				// Should not panic.
				SetupLogging()
			})
		}
	}
}

func TestParseEnum(t *testing.T) {
	cases := []struct {
		name    string
		value   string
		want    string
		wantErr bool
	}{
		{"unset uses default", "", "redis", false},
		{"redis", "redis", "redis", false},
		{"omni-pitcher", "omni-pitcher", "omni-pitcher", false},
		{"both", "both", "both", false},
		{"file", "file", "file", false},
		{"http was never valid", "http", "", true},
		{"case matters", "Redis", "", true},
		{"underscore variant", "omni_pitcher", "", true},
		{"surrounding space", " redis", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseEnum("PITCH_TARGET", tc.value, "redis", PitchTargets)
			if (err != nil) != tc.wantErr {
				t.Fatalf("parseEnum(%q) error = %v, wantErr %v", tc.value, err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("parseEnum(%q) = %q, want %q", tc.value, got, tc.want)
			}
		})
	}
}

func TestLoadPitchTargetAndDemoMode(t *testing.T) {
	t.Setenv("PITCH_TARGET", "")
	t.Setenv("DEMO_MODE", "")
	if got, err := LoadPitchTarget(); err != nil || got != "redis" {
		t.Fatalf("LoadPitchTarget() unset = %q, %v; want redis, nil", got, err)
	}
	if got, err := LoadDemoMode(); err != nil || got != "api" {
		t.Fatalf("LoadDemoMode() unset = %q, %v; want api, nil", got, err)
	}

	t.Setenv("PITCH_TARGET", "http")
	if _, err := LoadPitchTarget(); err == nil || !strings.Contains(err.Error(), "PITCH_TARGET") || !strings.Contains(err.Error(), "omni-pitcher") {
		t.Fatalf("LoadPitchTarget() http: error %v should name the variable and list the valid values", err)
	}

	t.Setenv("DEMO_MODE", "ui")
	if _, err := LoadDemoMode(); err == nil || !strings.Contains(err.Error(), "DEMO_MODE") || !strings.Contains(err.Error(), "full") {
		t.Fatalf("LoadDemoMode() ui: error %v should name the variable and list the valid values", err)
	}
}
