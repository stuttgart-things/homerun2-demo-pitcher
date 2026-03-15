
package pitcher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	homerun "github.com/stuttgart-things/homerun-library/v3"
)

// HTTPPitcher forwards messages to a remote pitcher via HTTP POST.
type HTTPPitcher struct {
	Endpoint   string       // e.g., "http://localhost:4000"
	APIPath    string       // e.g., "generic"
	AuthToken  string
	HTTPClient *http.Client
}

// httpResponse represents the JSON response from the remote pitcher.
type httpResponse struct {
	ObjectID string `json:"objectId"`
	StreamID string `json:"streamId"`
	Status   string `json:"status"`
	Message  string `json:"message,omitempty"`
}

func (p *HTTPPitcher) client() *http.Client {
	if p.HTTPClient != nil {
		return p.HTTPClient
	}
	return http.DefaultClient
}

// Pitch forwards a message to the remote pitcher via HTTP POST.
// It retries up to 3 times with exponential backoff on failure.
func (p *HTTPPitcher) Pitch(msg homerun.Message) (string, string, error) {
	body, err := json.Marshal(msg)
	if err != nil {
		return "", "", fmt.Errorf("http pitcher: failed to marshal message: %w", err)
	}

	url := fmt.Sprintf("%s/%s", p.Endpoint, p.APIPath)

	backoffs := []time.Duration{100 * time.Millisecond, 200 * time.Millisecond, 400 * time.Millisecond}
	maxAttempts := len(backoffs) + 1
	var lastErr error

	for attempt := range maxAttempts {
		resp, err := p.doPost(url, body)
		if err != nil {
			lastErr = err
			slog.Warn("http pitcher: request failed",
				"attempt", attempt+1,
				"error", err,
			)
			if attempt < len(backoffs) {
				time.Sleep(backoffs[attempt])
			}
			continue
		}

		return resp.ObjectID, resp.StreamID, nil
	}

	return "", "", fmt.Errorf("http pitcher: all %d attempts failed: %w", maxAttempts, lastErr)
}

func (p *HTTPPitcher) doPost(url string, body []byte) (*httpResponse, error) {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if p.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+p.AuthToken)
	}

	resp, err := p.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	var result httpResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

// HealthCheck verifies the remote pitcher is reachable.
func (p *HTTPPitcher) HealthCheck(ctx context.Context) error {
	url := fmt.Sprintf("%s/health", p.Endpoint)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("http pitcher health check: failed to create request: %w", err)
	}

	resp, err := p.client().Do(req)
	if err != nil {
		return fmt.Errorf("http pitcher health check: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("http pitcher health check: unexpected status %d", resp.StatusCode)
	}

	return nil
}
