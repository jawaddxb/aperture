// Package llm provides LLM client implementations for OpenAI and Anthropic.
// This file implements the Anthropic Messages API client.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	anthropicDefaultBaseURL = "https://api.anthropic.com"
	anthropicVersion        = "2023-06-01"
	anthropicDefaultTimeout = 60 * time.Second
)

// AnthropicClient calls the Anthropic Messages API.
// It also implements domain.VisionLLMClient via CompleteWithImage.
type AnthropicClient struct {
	cfg        Config
	httpClient *http.Client
	baseURL    string
}

// newAnthropicClient constructs an AnthropicClient from cfg.
func newAnthropicClient(cfg Config) *AnthropicClient {
	base := cfg.BaseURL
	if base == "" {
		base = anthropicDefaultBaseURL
	}
	return &AnthropicClient{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: anthropicDefaultTimeout},
		baseURL:    base,
	}
}

// anthropicRequest is the request body for POST /v1/messages.
type anthropicRequest struct {
	Model     string              `json:"model"`
	MaxTokens int                 `json:"max_tokens"`
	Messages  []anthropicMessage  `json:"messages"`
}

// anthropicMessage is a single turn in the Anthropic conversation.
type anthropicMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"` // string or []anthropicContentBlock
}

// anthropicContentBlock represents a multimodal content block.
type anthropicContentBlock struct {
	Type   string                 `json:"type"`
	Text   string                 `json:"text,omitempty"`
	Source *anthropicImageSource  `json:"source,omitempty"`
}

// anthropicImageSource holds the base64 image data for vision requests.
type anthropicImageSource struct {
	Type      string `json:"type"`       // "base64"
	MediaType string `json:"media_type"` // e.g. "image/png"
	Data      string `json:"data"`       // base64-encoded bytes
}

// anthropicResponse is the decoded response from the Messages API.
type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Error *anthropicAPIError `json:"error,omitempty"`
}

// anthropicAPIError is the error envelope returned by the Anthropic API.
type anthropicAPIError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// Complete sends a text-only prompt and returns the model completion.
// Implements domain.LLMClient.
func (c *AnthropicClient) Complete(ctx context.Context, prompt string) (string, error) {
	msgs := []anthropicMessage{
		{Role: "user", Content: prompt},
	}
	return c.doMessages(ctx, msgs)
}

// CompleteWithImage sends a prompt with an inline image to the model.
// Implements domain.VisionLLMClient.
func (c *AnthropicClient) CompleteWithImage(ctx context.Context, prompt, imageBase64, mimeType string) (string, error) {
	blocks := []anthropicContentBlock{
		{
			Type: "image",
			Source: &anthropicImageSource{
				Type:      "base64",
				MediaType: mimeType,
				Data:      imageBase64,
			},
		},
		{Type: "text", Text: prompt},
	}
	msgs := []anthropicMessage{
		{Role: "user", Content: blocks},
	}
	return c.doMessages(ctx, msgs)
}

// doMessages executes a Messages API call with backoff on 429.
func (c *AnthropicClient) doMessages(ctx context.Context, msgs []anthropicMessage) (string, error) {
	reqBody := anthropicRequest{
		Model:     c.cfg.Model,
		MaxTokens: c.cfg.MaxTokens,
		Messages:  msgs,
	}

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			wait := backoffDuration(attempt)
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(wait):
			}
		}

		result, retry, err := c.sendMessagesRequest(ctx, reqBody)
		if err != nil && !retry {
			return "", err
		}
		if err != nil {
			lastErr = err
			continue
		}
		return result, nil
	}
	return "", fmt.Errorf("anthropic: max retries exceeded: %w", lastErr)
}

// sendMessagesRequest performs a single HTTP POST to the Messages endpoint.
// Returns (result, retry, error) — retry=true signals a rate-limit retry.
func (c *AnthropicClient) sendMessagesRequest(ctx context.Context, reqBody anthropicRequest) (string, bool, error) {
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", false, fmt.Errorf("anthropic: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/v1/messages", bytes.NewReader(payload))
	if err != nil {
		return "", false, fmt.Errorf("anthropic: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.cfg.APIKey)
	req.Header.Set("anthropic-version", anthropicVersion)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", false, fmt.Errorf("anthropic: http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return "", true, fmt.Errorf("anthropic: rate limited (429)")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", false, fmt.Errorf("anthropic: read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", false, fmt.Errorf("anthropic: unexpected status %d: %s", resp.StatusCode, body)
	}

	var antResp anthropicResponse
	if err := json.Unmarshal(body, &antResp); err != nil {
		return "", false, fmt.Errorf("anthropic: parse response: %w", err)
	}
	if antResp.Error != nil {
		return "", false, fmt.Errorf("anthropic: api error: %s", antResp.Error.Message)
	}
	if len(antResp.Content) == 0 {
		return "", false, fmt.Errorf("anthropic: empty content in response")
	}
	return antResp.Content[0].Text, false, nil
}
