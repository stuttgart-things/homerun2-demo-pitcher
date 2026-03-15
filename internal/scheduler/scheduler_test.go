package scheduler

import (
	"sync"
	"testing"
	"time"

	homerun "github.com/stuttgart-things/homerun-library/v3"
)

// mockGenerator implements MessageGenerator for testing.
type mockGenerator struct {
	mu    sync.Mutex
	calls int
}

func (m *mockGenerator) Generate() homerun.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	return homerun.Message{}
}

func (m *mockGenerator) GenerateBatch(n int) []homerun.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	msgs := make([]homerun.Message, n)
	for i := range msgs {
		msgs[i] = homerun.Message{}
	}
	return msgs
}

func (m *mockGenerator) getCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

// mockPitcher implements MessagePitcher for testing.
type mockPitcher struct {
	mu      sync.Mutex
	pitched int
	failAt  int // if > 0, fail on every failAt-th call
}

func (m *mockPitcher) Pitch(msg homerun.Message) (string, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pitched++
	if m.failAt > 0 && m.pitched%m.failAt == 0 {
		return "", "", errPitchFailed
	}
	return "obj-1", "stream-1", nil
}

func (m *mockPitcher) getPitched() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.pitched
}

var errPitchFailed = &pitchError{}

type pitchError struct{}

func (e *pitchError) Error() string { return "mock pitch failure" }

func TestStartStop(t *testing.T) {
	cfg := Config{
		Interval:  10 * time.Millisecond,
		BurstSize: 1,
		Enabled:   true,
	}
	gen := &mockGenerator{}
	p := &mockPitcher{}
	s := New(cfg, gen, p)

	s.Start()

	stats := s.GetStats()
	if !stats.Running {
		t.Fatal("expected scheduler to be running")
	}
	if stats.StartedAt.IsZero() {
		t.Fatal("expected StartedAt to be set")
	}

	s.Stop()

	stats = s.GetStats()
	if stats.Running {
		t.Fatal("expected scheduler to be stopped")
	}
}

func TestPitching(t *testing.T) {
	cfg := Config{
		Interval:  10 * time.Millisecond,
		BurstSize: 1,
		Enabled:   true,
	}
	gen := &mockGenerator{}
	p := &mockPitcher{}
	s := New(cfg, gen, p)

	s.Start()
	time.Sleep(55 * time.Millisecond)
	s.Stop()

	if gen.getCalls() == 0 {
		t.Fatal("expected generator to be called at least once")
	}
	if p.getPitched() == 0 {
		t.Fatal("expected pitcher to be called at least once")
	}

	stats := s.GetStats()
	if stats.Pitched == 0 {
		t.Fatal("expected pitched count > 0")
	}
}

func TestBurstSize(t *testing.T) {
	burstSize := 5
	cfg := Config{
		Interval:  10 * time.Millisecond,
		BurstSize: burstSize,
		Enabled:   true,
	}
	gen := &mockGenerator{}
	p := &mockPitcher{}
	s := New(cfg, gen, p)

	s.Start()
	time.Sleep(35 * time.Millisecond)
	s.Stop()

	pitched := p.getPitched()
	if pitched == 0 {
		t.Fatal("expected pitcher to be called")
	}
	// Each tick should pitch exactly burstSize messages.
	// After at least one tick, pitched should be a multiple of burstSize.
	if pitched%burstSize != 0 {
		t.Fatalf("expected pitched count to be a multiple of %d, got %d", burstSize, pitched)
	}
}

func TestStats(t *testing.T) {
	cfg := Config{
		Interval:  10 * time.Millisecond,
		BurstSize: 2,
		Enabled:   true,
	}
	gen := &mockGenerator{}
	p := &mockPitcher{failAt: 2} // every 2nd pitch fails
	s := New(cfg, gen, p)

	s.Start()
	time.Sleep(35 * time.Millisecond)
	s.Stop()

	stats := s.GetStats()
	if stats.Pitched == 0 {
		t.Fatal("expected some successful pitches")
	}
	if stats.Failed == 0 {
		t.Fatal("expected some failed pitches")
	}
	if stats.LastPitchedAt.IsZero() {
		t.Fatal("expected LastPitchedAt to be set")
	}
}

func TestDisabled(t *testing.T) {
	cfg := Config{
		Interval:  10 * time.Millisecond,
		BurstSize: 1,
		Enabled:   false,
	}
	gen := &mockGenerator{}
	p := &mockPitcher{}
	s := New(cfg, gen, p)

	s.Start()
	time.Sleep(35 * time.Millisecond)
	s.Stop()

	if gen.getCalls() != 0 {
		t.Fatal("expected generator not to be called when disabled")
	}
	if p.getPitched() != 0 {
		t.Fatal("expected pitcher not to be called when disabled")
	}

	stats := s.GetStats()
	if stats.Pitched != 0 {
		t.Fatal("expected no pitched messages when disabled")
	}
}
