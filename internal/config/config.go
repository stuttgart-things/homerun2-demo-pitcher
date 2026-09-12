package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	homerun "github.com/stuttgart-things/homerun-library/v4"
)

func LoadRedisConfig() homerun.RedisConfig {
	return homerun.RedisConfig{
		Addr:     homerun.GetEnv("REDIS_ADDR", "localhost"),
		Port:     homerun.GetEnv("REDIS_PORT", "6379"),
		Password: homerun.GetEnv("REDIS_PASSWORD", ""),
		Stream:   homerun.GetEnv("REDIS_STREAM", "homerun"),
	}
}

// PitchTargets are the backends PITCH_TARGET selects, and DemoModes the HTTP
// surfaces DEMO_MODE selects. Both used to be matched by a switch whose default
// swallowed any other value: PITCH_TARGET=http pitched to Redis instead of
// omni-pitcher, with every request answering 200 (#50).
var (
	PitchTargets = []string{"redis", "omni-pitcher", "both", "file"}
	DemoModes    = []string{"api", "web", "full"}
)

// LoadPitchTarget reads PITCH_TARGET; unset means "redis".
func LoadPitchTarget() (string, error) {
	return parseEnum("PITCH_TARGET", os.Getenv("PITCH_TARGET"), "redis", PitchTargets)
}

// LoadDemoMode reads DEMO_MODE; unset means "api".
func LoadDemoMode() (string, error) {
	return parseEnum("DEMO_MODE", os.Getenv("DEMO_MODE"), "api", DemoModes)
}

// parseEnum returns def for an empty value and an error naming the variable and
// the valid values for anything outside allowed. The match is exact: "Redis" or
// "omni_pitcher" are rejected rather than guessed at.
func parseEnum(name, value, def string, allowed []string) (string, error) {
	if value == "" {
		return def, nil
	}
	for _, a := range allowed {
		if value == a {
			return value, nil
		}
	}
	return "", fmt.Errorf("%s %q is not one of %s", name, value, strings.Join(allowed, ", "))
}

// SetupLogging configures slog as the default logger based on LOG_FORMAT and LOG_LEVEL env vars.
func SetupLogging() {
	format := strings.ToLower(homerun.GetEnv("LOG_FORMAT", "json"))
	levelStr := strings.ToLower(homerun.GetEnv("LOG_LEVEL", "info"))

	var level slog.Level
	switch levelStr {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler
	if format == "text" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	slog.SetDefault(slog.New(handler))

	// homerun-library is silent by default as of v3.2.0. Routing it through the
	// same logger means its records arrive in this service's format and at this
	// service's level, instead of the pterm-decorated stdout writes it used to
	// interleave into the log stream.
	homerun.SetLogger(slog.Default())
}

// ParsePitchTarget is the #50 helper kept as an alias of LoadPitchTarget.
func ParsePitchTarget() (string, error) { return LoadPitchTarget() }

// ParseDemoMode is the #50 helper kept as an alias of LoadDemoMode.
func ParseDemoMode() (string, error) { return LoadDemoMode() }
