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

	homerun "github.com/stuttgart-things/homerun-library/v4"
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

	// Both are validated before anything else starts: an unknown value used to
	// fall into a switch default and run the wrong backend or HTTP surface while
	// every request answered 200 (#50).
	demoMode, err := config.LoadDemoMode()
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	pitcherTarget, err := config.LoadPitchTarget()
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	buildInfo := handlers.BuildInfo{Version: version, Commit: commit, Date: date}
	mux := http.NewServeMux()

	// Always register health endpoint.
	mux.HandleFunc("/health", handlers.NewHealthHandler(buildInfo))

	// Canceled on SIGINT/SIGTERM: ends the startup wait for Redis, then the server.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)

	// Build pitchers based on validated PITCH_TARGET.
	allPitchers := buildPitchers(ctx, pitcherTarget)

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

	default: // "api", the only value left after config.LoadDemoMode
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

	<-ctx.Done()
	stop()

	slog.Info("shutting down server")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("server forced to shutdown", "error", err)
		os.Exit(1)
	}
	slog.Info("server exited gracefully")
}

// buildPitchers creates all pitcher backends based on the target config.
func buildPitchers(ctx context.Context, target string) map[string]pitcher.Pitcher {
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
		// Redis. Waited for like the redis target: a failed first check used to
		// drop the Redis backend for the lifetime of the pod, which then pitched
		// to omni-pitcher only while reporting "both" (#47).
		redisConfig := config.LoadRedisConfig()
		waitForRedis(ctx, redisConfig)
		rp := &pitcher.RedisPitcher{Config: redisConfig}
		pitchers["redis"] = rp
		slog.Info("pitcher backend: redis", "addr", redisConfig.Addr, "port", redisConfig.Port)

		// HTTP / omni-pitcher
		hp := newHTTPPitcher()
		pitchers["omni-pitcher"] = hp
		slog.Info("pitcher backend: omni-pitcher", "endpoint", hp.Endpoint)

		// Multi-pitcher combining both
		pitchers["both"] = &pitcher.MultiPitcher{Pitchers: []pitcher.Pitcher{rp, hp}}

	default: // "redis", the only value left after config.LoadPitchTarget
		redisConfig := config.LoadRedisConfig()
		waitForRedis(ctx, redisConfig)
		rp := &pitcher.RedisPitcher{Config: redisConfig}
		pitchers["redis"] = rp
		slog.Info("pitcher backend: redis", "addr", redisConfig.Addr, "port", redisConfig.Port)
	}

	return pitchers
}

// waitForRedis blocks until Redis answers, for REDIS_STARTUP_TIMEOUT (default
// 120s), instead of the single 5s check that ended the process whenever Redis
// was still starting (#47). A shutdown signal during the wait exits 0; a timeout
// or an invalid REDIS_STARTUP_TIMEOUT exits 1.
func waitForRedis(ctx context.Context, rc homerun.RedisConfig) {
	timeout, err := homerun.LoadRedisStartupTimeout()
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	if err := homerun.WaitForRedisContext(ctx, rc, timeout); err != nil {
		if ctx.Err() != nil {
			slog.Info("shutdown requested while waiting for redis")
			os.Exit(0)
		}
		slog.Error("redis not reachable", "error", err, "addr", rc.Addr, "port", rc.Port, "startup_timeout", timeout.String())
		os.Exit(1)
	}
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
	genProfile := profileToGeneratorProfile(prof)

	// AI configuration (optional).
	aiCfg := generator.AIConfig{
		Enabled:  homerun.GetEnv("AI_ENABLED", "false") == "true",
		Provider: homerun.GetEnv("AI_PROVIDER", "ollama"),
		Model:    homerun.GetEnv("AI_MODEL", "llama3"),
		Endpoint: homerun.GetEnv("AI_ENDPOINT", "http://localhost:11434"),
		APIKey:   homerun.GetEnv("AI_API_KEY", ""),
	}

	// Create standard generator (always used for web handlers).
	gen := generator.New(genProfile)

	// Create AI generator for the scheduler when AI is enabled.
	aiGen := generator.NewAIGenerator(genProfile, aiCfg)
	if aiCfg.Enabled {
		slog.Info("AI-powered generation enabled", "provider", aiCfg.Provider, "model", aiCfg.Model)
	}

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

	// If profile has a target_url, use an HTTP pitcher for the scheduler.
	schedPitcher := primaryPitcher
	if prof.TargetURL != "" {
		schedPitcher = &pitcher.HTTPPitcher{
			Endpoint:   prof.TargetURL,
			APIPath:    homerun.GetEnv("OMNI_PITCHER_API_PATH", "generic"),
			AuthToken:  homerun.GetEnv("AUTH_TOKEN", ""),
			HTTPClient: pitcher.DefaultHTTPClient(),
		}
		slog.Info("scheduler using profile target_url", "url", prof.TargetURL)
	}

	var sched *scheduler.Scheduler
	if schedPitcher != nil {
		schedCfg := scheduler.Config{
			Interval:  interval,
			BurstSize: burst,
			Enabled:   enabled,
		}
		sched = scheduler.New(schedCfg, aiGen, schedPitcher)
		sched.Start()
		slog.Info("scheduler configured", "enabled", enabled, "interval", intervalStr, "burst", burst)
	}

	// Determine default target URL from profile or env.
	defaultTargetURL := homerun.GetEnv("TARGET_URL", prof.TargetURL)
	if defaultTargetURL == "" {
		defaultTargetURL = homerun.GetEnv("OMNI_PITCHER_URL", "http://localhost:4000")
	}

	// Web handlers.
	webHandlers := handlers.NewWebHandlers(
		web.TemplateFS,
		gen,
		allPitchers,
		sched,
		intervalStr,
		defaultTargetURL,
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
