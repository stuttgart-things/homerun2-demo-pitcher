package config

import (
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
