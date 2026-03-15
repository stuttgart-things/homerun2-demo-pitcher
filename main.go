package main

import (
	"context"
	"embed"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"github.com/stuttgart-things/homerun2-demo-pitcher/internal/banner"
	"github.com/stuttgart-things/homerun2-demo-pitcher/internal/config"
	"github.com/stuttgart-things/homerun2-demo-pitcher/internal/generator"
	"github.com/stuttgart-things/homerun2-demo-pitcher/internal/handlers"
	"github.com/stuttgart-things/homerun2-demo-pitcher/internal/middleware"
	"github.com/stuttgart-things/homerun2-demo-pitcher/internal/pitcher"
	"github.com/stuttgart-things/homerun2-demo-pitcher/internal/profile"
	"github.com/stuttgart-things/homerun2-demo-pitcher/internal/scheduler"
	"github.com/stuttgart-things/homerun2-demo-pitcher/internal/web"

	homerun "github.com/stuttgart-things/homerun-library/v3"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

//go:embed static/*
var staticFS embed.FS

func main() {
	banner.Show()
	config.SetupLogging()

	slog.Info("starting homerun2-demo-pitcher",
		"version", version,
		"commit", commit,
		"date", date,
		"go", runtime.Version(),
	)

	port := homerun.GetEnv("PORT", "8080")
	demoMode := homerun.GetEnv("DEMO_MODE", "api")

	buildInfo := handlers.BuildInfo{Version: version, Commit: commit, Date: date}
	mux := http.NewServeMux()

	// Always register health endpoint.
	mux.HandleFunc("/health", handlers.NewHealthHandler(buildInfo))

	// Build pitchers based on PITCH_TARGET.
	pitcherTarget := homerun.GetEnv("PITCH_TARGET", "redis")
	allPitchers := buildPitchers(pitcherTarget)

	// Pick the primary pitcher (for API and scheduler).
	primaryPitcher := pickPrimaryPitcher(allPitchers, pitcherTarget)

	switch demoMode {
	case "web":
		registerWebRoutes(mux, allPitchers, primaryPitcher, buildInfo)
		// In web mode, also register /pitch without auth for convenience.
		if primaryPitcher != nil {
			mux.HandleFunc("/pitch", handlers.NewPitchHandler(primaryPitcher))
		}

	case "full":
		registerWebRoutes(mux, allPitchers, primaryPitcher, buildInfo)
		// In full mode, /pitch requires auth.
		if primaryPitcher != nil {
			mux.HandleFunc("/pitch", middleware.TokenAuthMiddleware(handlers.NewPitchHandler(primaryPitcher)))
		}

	default: // "api"
		if primaryPitcher != nil {
			mux.HandleFunc("/pitch", middleware.TokenAuthMiddleware(handlers.NewPitchHandler(primaryPitcher)))
		}
	}

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: middleware.RequestLogging(mux),
	}

	go func() {
		slog.Info("server listening", "port", port, "mode", demoMode)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down server")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("server forced to shutdown", "error", err)
		os.Exit(1)
	}
	slog.Info("server exited gracefully")
}

// buildPitchers creates all pitcher backends based on the target config.
func buildPitchers(target string) map[string]pitcher.Pitcher {
	pitchers := make(map[string]pitcher.Pitcher)

	switch target {
	case "file":
		filePath := homerun.GetEnv("PITCHER_FILE", "pitched.log")
		pitchers["redis"] = &pitcher.FilePitcher{Path: filePath}
		slog.Info("pitcher backend: file", "path", filePath)

	case "omni-pitcher":
		hp := newHTTPPitcher()
		pitchers["omni-pitcher"] = hp
		slog.Info("pitcher backend: omni-pitcher", "endpoint", hp.Endpoint)

	case "both":
		// Redis
		redisConfig := config.LoadRedisConfig()
		rp := &pitcher.RedisPitcher{Config: redisConfig}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := rp.HealthCheck(ctx); err != nil {
			slog.Warn("redis health check failed for multi-pitcher, redis backend disabled", "error", err)
		} else {
			pitchers["redis"] = rp
			slog.Info("pitcher backend: redis", "addr", redisConfig.Addr, "port", redisConfig.Port)
		}
		cancel()

		// HTTP / omni-pitcher
		hp := newHTTPPitcher()
		pitchers["omni-pitcher"] = hp
		slog.Info("pitcher backend: omni-pitcher", "endpoint", hp.Endpoint)

		// Multi-pitcher combining both
		var backends []pitcher.Pitcher
		if rp, ok := pitchers["redis"]; ok {
			backends = append(backends, rp)
		}
		backends = append(backends, hp)
		pitchers["both"] = &pitcher.MultiPitcher{Pitchers: backends}

	default: // redis
		redisConfig := config.LoadRedisConfig()
		rp := &pitcher.RedisPitcher{Config: redisConfig}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := rp.HealthCheck(ctx); err != nil {
			slog.Error("redis health check failed", "error", err)
			cancel()
			os.Exit(1)
		}
		cancel()
		pitchers["redis"] = rp
		slog.Info("pitcher backend: redis", "addr", redisConfig.Addr, "port", redisConfig.Port)
	}

	return pitchers
}

func newHTTPPitcher() *pitcher.HTTPPitcher {
	return &pitcher.HTTPPitcher{
		Endpoint:  homerun.GetEnv("OMNI_PITCHER_URL", "http://localhost:4000"),
		APIPath:   homerun.GetEnv("OMNI_PITCHER_API_PATH", "generic"),
		AuthToken: homerun.GetEnv("AUTH_TOKEN", ""),
	}
}

func pickPrimaryPitcher(pitchers map[string]pitcher.Pitcher, target string) pitcher.Pitcher {
	if p, ok := pitchers[target]; ok {
		return p
	}
	// Fallback: return any available pitcher.
	for _, p := range pitchers {
		return p
	}
	return nil
}

// registerWebRoutes sets up the web UI, scheduler, and static file routes.
func registerWebRoutes(
	mux *http.ServeMux,
	allPitchers map[string]pitcher.Pitcher,
	primaryPitcher pitcher.Pitcher,
	buildInfo handlers.BuildInfo,
) {
	// Load profile.
	prof := loadProfile()
	gen := generator.New(profileToGeneratorProfile(prof))

	// Scheduler config.
	intervalStr := homerun.GetEnv("PITCH_INTERVAL", "10s")
	interval, err := time.ParseDuration(intervalStr)
	if err != nil {
		slog.Warn("invalid PITCH_INTERVAL, using 10s", "value", intervalStr)
		interval = 10 * time.Second
		intervalStr = "10s"
	}

	burstStr := homerun.GetEnv("PITCH_BURST_SIZE", "1")
	burst, err := strconv.Atoi(burstStr)
	if err != nil || burst < 1 {
		burst = 1
	}

	enabled := homerun.GetEnv("PITCH_ENABLED", "false") == "true"

	var sched *scheduler.Scheduler
	if primaryPitcher != nil {
		schedCfg := scheduler.Config{
			Interval:  interval,
			BurstSize: burst,
			Enabled:   enabled,
		}
		sched = scheduler.New(schedCfg, gen, primaryPitcher)
		sched.Start()
		slog.Info("scheduler configured", "enabled", enabled, "interval", intervalStr, "burst", burst)
	}

	// Web handlers.
	webHandlers := handlers.NewWebHandlers(
		web.TemplateFS,
		gen,
		allPitchers,
		sched,
		intervalStr,
		buildInfo,
	)

	mux.HandleFunc("/", webHandlers.IndexHandler)
	mux.HandleFunc("/composer/fields", webHandlers.ComposerFieldsHandler)
	mux.HandleFunc("/composer/send", webHandlers.ComposerSendHandler)
	mux.HandleFunc("/log", webHandlers.SentLogHandler)

	// Static files (htmx.min.js).
	staticSub, err := fs.Sub(staticFS, "static")
	if err != nil {
		slog.Error("failed to create static sub-filesystem", "error", err)
		os.Exit(1)
	}
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))

	slog.Info("web UI routes registered")
}

// loadProfile loads a profile from disk or returns the default.
func loadProfile() *profile.Profile {
	profileDir := homerun.GetEnv("PITCH_PROFILE_DIR", "profiles")
	profileName := homerun.GetEnv("PITCH_PROFILE", "default")

	profiles, err := profile.LoadFromDir(profileDir)
	if err != nil {
		slog.Info("could not load profiles from dir, using default", "dir", profileDir, "error", err)
		return profile.DefaultProfile()
	}

	if p, ok := profiles[profileName]; ok {
		slog.Info("loaded profile", "name", profileName)
		return p
	}

	slog.Info("profile not found, using default", "name", profileName)
	return profile.DefaultProfile()
}

// profileToGeneratorProfile converts a profile.Profile to a generator.MessageProfile.
func profileToGeneratorProfile(p *profile.Profile) *generator.MessageProfile {
	mp := &generator.MessageProfile{
		Name:       p.Name,
		Severities: p.Severities,
		Systems:    p.Systems,
		Authors:    p.Authors,
		Tags:       p.Tags,
		URLs:       p.URLs,
		Artifacts:  p.Artifacts,
		Templates:  make(map[string]generator.SeverityTemplate),
	}

	for sev, st := range p.Templates {
		mp.Templates[sev] = generator.SeverityTemplate{
			Titles:   st.Titles,
			Messages: st.Messages,
		}
	}

	for _, a := range p.Assignees {
		mp.Assignees = append(mp.Assignees, generator.Assignee{
			Name:    a.Name,
			Address: a.Address,
		})
	}

	return mp
}
