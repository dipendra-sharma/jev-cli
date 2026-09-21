package jev

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultBaseURL       = "https://openrouter.ai/api/v1"
	decisionPath         = "/systemone"
	modelsPath           = "/models"
	maxErrorBodyBytes    = 8 << 10
	maxErrorMessageChars = 400
	baseBackoff          = 400 * time.Millisecond
	maxBackoff           = 8 * time.Second
	maxRetryAfter        = 30 * time.Second
	defaultHTTPTimeout   = 60 * time.Second
)

type Client struct {
	BaseURL string
	APIKey  string
	Retries int
	HTTP    *http.Client
}

func NewClient(baseURL, apiKey string, retries int, timeout time.Duration) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if timeout <= 0 {
		timeout = defaultHTTPTimeout
	}
	return &Client{
		BaseURL: baseURL,
		APIKey:  apiKey,
		Retries: retries,
		HTTP:    &http.Client{Timeout: timeout},
	}
}

func (c *Client) Decide(ctx context.Context, body any) (*Response, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encoding request: %w", err)
	}
	raw, err := c.post(ctx, decisionPath, payload)
	if err != nil {
		return nil, err
	}
	var out Response
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	if len(out.Answers) == 0 {
		return nil, errors.New("the model returned no answers")
	}
	return &out, nil
}

func (c *Client) DecideRaw(ctx context.Context, body any) (json.RawMessage, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encoding request: %w", err)
	}
	return c.post(ctx, decisionPath, payload)
}

func (c *Client) DecisionModels(ctx context.Context) ([]Model, error) {
	endpoint := c.BaseURL + modelsPath + "?" + url.Values{"output_modalities": {"decisions"}}.Encode()
	raw, err := c.get(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	var out struct {
		Data []Model `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decoding model list: %w", err)
	}
	return out.Data, nil
}

func (c *Client) post(ctx context.Context, path string, payload []byte) (json.RawMessage, error) {
	return c.withRetries(ctx, func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		return req, nil
	})
}

func (c *Client) get(ctx context.Context, endpoint string) (json.RawMessage, error) {
	return c.withRetries(ctx, func() (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	})
}

func (c *Client) withRetries(ctx context.Context, build func() (*http.Request, error)) (json.RawMessage, error) {
	var lastErr error
	for attempt := 0; attempt <= c.Retries; attempt++ {
		if attempt > 0 {
			if err := sleep(ctx, backoffFor(attempt, lastErr)); err != nil {
				return nil, err
			}
		}
		body, err := c.attempt(build)
		if err == nil {
			return body, nil
		}
		lastErr = err
		var apiErr *APIError
		if !errors.As(err, &apiErr) || !apiErr.Retryable() {
			return nil, err
		}
	}
	return nil, fmt.Errorf("giving up after %d attempts: %w", c.Retries+1, lastErr)
}

func (c *Client) attempt(build func() (*http.Request, error)) (json.RawMessage, error) {
	req, err := build()
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("X-Title", "jev-cli")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling openrouter: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &APIError{
			StatusCode: resp.StatusCode,
			RetryAfter: parseRetryAfter(resp.Header),
			Message:    readErrorMessage(resp),
		}
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}
	return body, nil
}

func readErrorMessage(resp *http.Response) string {
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
	if err != nil || len(raw) == 0 {
		return resp.Status
	}
	var wrapped struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &wrapped) == nil && wrapped.Error.Message != "" {
		return collapseWhitespace(wrapped.Error.Message)
	}
	return collapseWhitespace(string(raw))
}

func collapseWhitespace(s string) string {
	joined := strings.Join(strings.Fields(s), " ")
	runes := []rune(joined)
	if len(runes) > maxErrorMessageChars {
		return string(runes[:maxErrorMessageChars]) + "..."
	}
	return joined
}

func backoffFor(attempt int, lastErr error) time.Duration {
	if apiErr, ok := errors.AsType[*APIError](lastErr); ok && apiErr.RetryAfter > 0 {
		return min(apiErr.RetryAfter, maxRetryAfter)
	}
	shift := min(attempt-1, 16)
	delay := min(baseBackoff<<shift, maxBackoff)
	return delay + time.Duration(rand.Int64N(int64(delay/2)+1))
}

func parseRetryAfter(header http.Header) time.Duration {
	if ms, err := strconv.Atoi(header.Get("retry-after-ms")); err == nil && ms > 0 {
		return time.Duration(ms) * time.Millisecond
	}
	value := header.Get("Retry-After")
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds <= 0 {
			return 0
		}
		return time.Duration(seconds) * time.Second
	}
	deadline, err := http.ParseTime(value)
	if err != nil {
		return 0
	}
	return max(time.Until(deadline), 0)
}

func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
