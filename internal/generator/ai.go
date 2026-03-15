package generator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	homerun "github.com/stuttgart-things/homerun-library/v3"
)

// AIConfig holds configuration for AI-powered generation.
type AIConfig struct {
	Enabled  bool
	Provider string // "ollama", "claude", "openai"
	Model    string
	Endpoint string
	APIKey   string
}

// AIGenerator wraps a standard Generator and optionally uses AI for message content.
type AIGenerator struct {
	fallback *Generator
	config   AIConfig
	client   *http.Client
}

// NewAIGenerator creates an AIGenerator that delegates to AI when enabled,
// falling back to the standard Generator on errors or when AI is disabled.
func NewAIGenerator(profile *MessageProfile, cfg AIConfig) *AIGenerator {
	return &AIGenerator{
		fallback: New(profile),
		config:   cfg,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Fallback returns the underlying standard Generator.
func (a *AIGenerator) Fallback() *Generator {
	return a.fallback
}

// Generate creates a single message, using AI if enabled.
func (a *AIGenerator) Generate() homerun.Message {
	severity := a.fallback.pickWeightedSeverity()
	return a.GenerateWithSeverity(severity)
}

// GenerateWithSeverity creates a message with a specific severity, using AI if enabled.
func (a *AIGenerator) GenerateWithSeverity(severity string) homerun.Message {
	if !a.config.Enabled {
		return a.fallback.GenerateWithSeverity(severity)
	}

	system := pickRandom(a.fallback.profile.Systems)
	title, message, err := a.callAI(severity, system)
	if err != nil {
		slog.Warn("AI generation failed, falling back to standard generator", "error", err)
		return a.fallback.GenerateWithSeverity(severity)
	}

	// Build the message using AI-generated title/message but standard fields.
	msg := a.fallback.GenerateWithSeverity(severity)
	msg.Title = title
	msg.Message = message
	msg.System = system
	return msg
}

// GenerateBatch creates n messages.
func (a *AIGenerator) GenerateBatch(n int) []homerun.Message {
	msgs := make([]homerun.Message, n)
	for i := range n {
		msgs[i] = a.Generate()
	}
	return msgs
}

// aiResponse is the expected JSON structure from the AI.
type aiResponse struct {
	Title   string `json:"title"`
	Message string `json:"message"`
}

// callAI calls the configured AI provider and returns title and message.
func (a *AIGenerator) callAI(severity, system string) (string, string, error) {
	prompt := fmt.Sprintf(
		"Generate a realistic %s level monitoring/ops message for system '%s'. "+
			"Return JSON with fields: title, message. Keep it concise and realistic.",
		severity, system,
	)

	var body []byte
	var err error

	switch strings.ToLower(a.config.Provider) {
	case "ollama":
		body, err = a.callOllama(prompt)
	case "claude":
		body, err = a.callClaude(prompt)
	case "openai":
		body, err = a.callOpenAI(prompt)
	default:
		return "", "", fmt.Errorf("unsupported AI provider: %s", a.config.Provider)
	}

	if err != nil {
		return "", "", err
	}

	return a.parseAIResponse(body)
}

func (a *AIGenerator) callOllama(prompt string) ([]byte, error) {
	reqBody, _ := json.Marshal(map[string]any{
		"model":  a.config.Model,
		"prompt": prompt,
		"stream": false,
		"format": "json",
	})

	req, err := http.NewRequest(http.MethodPost, a.config.Endpoint+"/api/generate", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("creating ollama request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama returned status %d", resp.StatusCode)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading ollama response: %w", err)
	}

	// Ollama wraps the response in {"response": "..."}
	var ollamaResp struct {
		Response string `json:"response"`
	}
	if err := json.Unmarshal(respBody, &ollamaResp); err != nil {
		return nil, fmt.Errorf("parsing ollama response wrapper: %w", err)
	}

	return []byte(ollamaResp.Response), nil
}

func (a *AIGenerator) callClaude(prompt string) ([]byte, error) {
	reqBody, _ := json.Marshal(map[string]any{
		"model":      a.config.Model,
		"max_tokens": 256,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	})

	req, err := http.NewRequest(http.MethodPost, a.config.Endpoint+"/v1/messages", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("creating claude request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", a.config.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("claude request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("claude returned status %d", resp.StatusCode)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading claude response: %w", err)
	}

	// Claude wraps the response in {"content": [{"text": "..."}]}
	var claudeResp struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(respBody, &claudeResp); err != nil {
		return nil, fmt.Errorf("parsing claude response: %w", err)
	}
	if len(claudeResp.Content) == 0 {
		return nil, fmt.Errorf("claude returned empty content")
	}

	return []byte(claudeResp.Content[0].Text), nil
}

func (a *AIGenerator) callOpenAI(prompt string) ([]byte, error) {
	reqBody, _ := json.Marshal(map[string]any{
		"model": a.config.Model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"max_tokens": 256,
	})

	req, err := http.NewRequest(http.MethodPost, a.config.Endpoint+"/v1/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("creating openai request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.config.APIKey)

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai returned status %d", resp.StatusCode)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading openai response: %w", err)
	}

	// OpenAI wraps the response in {"choices": [{"message": {"content": "..."}}]}
	var openaiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &openaiResp); err != nil {
		return nil, fmt.Errorf("parsing openai response: %w", err)
	}
	if len(openaiResp.Choices) == 0 {
		return nil, fmt.Errorf("openai returned no choices")
	}

	return []byte(openaiResp.Choices[0].Message.Content), nil
}

// parseAIResponse extracts title and message from the AI's JSON output.
func (a *AIGenerator) parseAIResponse(body []byte) (string, string, error) {
	// Try to find JSON in the response (AI might wrap it in markdown code blocks).
	text := string(body)
	text = strings.TrimSpace(text)

	// Strip markdown code fences if present.
	if idx := strings.Index(text, "{"); idx >= 0 {
		text = text[idx:]
	}
	if idx := strings.LastIndex(text, "}"); idx >= 0 {
		text = text[:idx+1]
	}

	var parsed aiResponse
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		return "", "", fmt.Errorf("parsing AI JSON response: %w (raw: %s)", err, string(body))
	}

	if parsed.Title == "" || parsed.Message == "" {
		return "", "", fmt.Errorf("AI response missing title or message")
	}

	return parsed.Title, parsed.Message, nil
}
