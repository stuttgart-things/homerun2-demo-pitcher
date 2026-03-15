package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	homerun "github.com/stuttgart-things/homerun-library/v3"
)

// MessageGenerator is the interface for generating messages.
type MessageGenerator interface {
	Generate() homerun.Message
	GenerateBatch(n int) []homerun.Message
}

// MessagePitcher is the interface for delivering messages.
type MessagePitcher interface {
	Pitch(msg homerun.Message) (objectID, streamID string, err error)
}

// Config holds scheduler configuration.
type Config struct {
	Interval  time.Duration
	BurstSize int
	Enabled   bool
}

// Stats holds runtime statistics.
type Stats struct {
	mu            sync.RWMutex
	Pitched       int64
	Failed        int64
	LastPitchedAt time.Time
	StartedAt     time.Time
	Running       bool
}

// Scheduler periodically generates and pitches messages.
type Scheduler struct {
	config    Config
	generator MessageGenerator
	pitcher   MessagePitcher
	stats     *Stats
	cancel    context.CancelFunc
	done      chan struct{}
}

// New creates a new Scheduler with the given configuration, generator, and pitcher.
func New(cfg Config, gen MessageGenerator, p MessagePitcher) *Scheduler {
	return &Scheduler{
		config:    cfg,
		generator: gen,
		pitcher:   p,
		stats:     &Stats{},
		done:      make(chan struct{}),
	}
}

// Start begins the scheduling loop. Non-blocking.
func (s *Scheduler) Start() {
	if !s.config.Enabled {
		slog.Info("scheduler disabled, not starting")
		close(s.done)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel

	s.stats.mu.Lock()
	s.stats.StartedAt = time.Now()
	s.stats.Running = true
	s.stats.mu.Unlock()

	go s.run(ctx)
}

func (s *Scheduler) run(ctx context.Context) {
	defer close(s.done)

	ticker := time.NewTicker(s.config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.stats.mu.Lock()
			s.stats.Running = false
			s.stats.mu.Unlock()
			slog.Info("scheduler stopped")
			return
		case <-ticker.C:
			s.pitchBatch()
		}
	}
}

func (s *Scheduler) pitchBatch() {
	messages := s.generator.GenerateBatch(s.config.BurstSize)

	var pitched, failed int64
	for _, msg := range messages {
		objectID, streamID, err := s.pitcher.Pitch(msg)
		if err != nil {
			failed++
			slog.Error("failed to pitch message", "error", err)
			continue
		}
		pitched++
		slog.Debug("message pitched", "objectID", objectID, "streamID", streamID)
	}

	s.stats.mu.Lock()
	s.stats.Pitched += pitched
	s.stats.Failed += failed
	if pitched > 0 {
		s.stats.LastPitchedAt = time.Now()
	}
	s.stats.mu.Unlock()

	slog.Info("pitch cycle complete", "pitched", pitched, "failed", failed, "burstSize", s.config.BurstSize)
}

// Stop gracefully stops the scheduler.
func (s *Scheduler) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	<-s.done
}

// GetStats returns current runtime statistics.
func (s *Scheduler) GetStats() Stats {
	s.stats.mu.RLock()
	defer s.stats.mu.RUnlock()

	return Stats{
		Pitched:       s.stats.Pitched,
		Failed:        s.stats.Failed,
		LastPitchedAt: s.stats.LastPitchedAt,
		StartedAt:     s.stats.StartedAt,
		Running:       s.stats.Running,
	}
}
