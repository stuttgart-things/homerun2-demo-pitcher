package generator

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAIGeneratorFallback(t *testing.T) {
	profile := DefaultProfile()
	cfg := AIConfig{
		Enabled:  false,
		Provider: "ollama",
		Model:    "test-model",
		Endpoint: "http://localhost:11434",
	}

	gen := NewAIGenerator(profile, cfg)

	msg := gen.Generate()
	if msg.Title == "" {
		t.Error("expected non-empty title from fallback generator")
	}
	if msg.System == "" {
		t.Error("expected non-empty system from fallback generator")
	}
	if msg.Severity == "" {
		t.Error("expected non-empty severity from fallback generator")
	}
	if msg.Author == "" {
		t.Error("expected non-empty author from fallback generator")
	}

	// Test GenerateWithSeverity falls back too.
	msg2 := gen.GenerateWithSeverity("critical")
	if msg2.Severity != "critical" {
		t.Errorf("expected severity 'critical', got %q", msg2.Severity)
	}

	// Test GenerateBatch falls back.
	batch := gen.GenerateBatch(5)
	if len(batch) != 5 {
		t.Errorf("expected 5 messages, got %d", len(batch))
	}
}

func TestAIGeneratorWithMockServer(t *testing.T) {
	// Mock an Ollama-like endpoint.
	aiTitle := "Mock Alert: api-gateway latency spike"
	aiMessage := "The api-gateway service is experiencing increased response times."

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/generate" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		innerJSON, _ := json.Marshal(map[string]string{
			"title":   aiTitle,
			"message": aiMessage,
		})

		resp, _ := json.Marshal(map[string]any{
			"response": string(innerJSON),
		})

		w.Header().Set("Content-Type", "application/json")
		w.Write(resp)
	}))
	defer server.Close()

	profile := DefaultProfile()
	cfg := AIConfig{
		Enabled:  true,
		Provider: "ollama",
		Model:    "test-model",
		Endpoint: server.URL,
	}

	gen := NewAIGenerator(profile, cfg)

	msg := gen.GenerateWithSeverity("warning")
	if msg.Title != aiTitle {
		t.Errorf("expected AI title %q, got %q", aiTitle, msg.Title)
	}
	if msg.Message != aiMessage {
		t.Errorf("expected AI message %q, got %q", aiMessage, msg.Message)
	}
	if msg.Severity != "warning" {
		t.Errorf("expected severity 'warning', got %q", msg.Severity)
	}
	// Standard fields should still be populated.
	if msg.Author == "" {
		t.Error("expected non-empty author from standard generator fields")
	}
	if msg.Tags == "" {
		t.Error("expected non-empty tags from standard generator fields")
	}
}

func TestAIGeneratorError(t *testing.T) {
	// Mock server that returns an error.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer server.Close()

	profile := DefaultProfile()
	cfg := AIConfig{
		Enabled:  true,
		Provider: "ollama",
		Model:    "test-model",
		Endpoint: server.URL,
	}

	gen := NewAIGenerator(profile, cfg)

	// Should fall back gracefully to standard generator.
	msg := gen.Generate()
	if msg.Title == "" {
		t.Error("expected non-empty title from fallback after AI error")
	}
	if msg.System == "" {
		t.Error("expected non-empty system from fallback after AI error")
	}
	if msg.Severity == "" {
		t.Error("expected non-empty severity from fallback after AI error")
	}

	// Also test with invalid JSON response.
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp, _ := json.Marshal(map[string]string{
			"response": "this is not valid json",
		})
		w.Header().Set("Content-Type", "application/json")
		w.Write(resp)
	}))
	defer server2.Close()

	cfg2 := AIConfig{
		Enabled:  true,
		Provider: "ollama",
		Model:    "test-model",
		Endpoint: server2.URL,
	}
	gen2 := NewAIGenerator(profile, cfg2)

	msg2 := gen2.GenerateWithSeverity("info")
	if msg2.Title == "" {
		t.Error("expected non-empty title from fallback after invalid AI response")
	}
	if msg2.Severity != "info" {
		t.Errorf("expected severity 'info', got %q", msg2.Severity)
	}
}
