// Package llm provides LLM client implementations for OpenAI and Anthropic.
// This file implements the OpenAI-compatible chat completions client.
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
	openAIDefaultBaseURL = "https://api.openai.com"
	openAIDefaultTimeout = 60 * time.Second
	maxRetries           = 3
)

// OpenAIClient calls the OpenAI chat completions API.
// It also implements domain.VisionLLMClient via CompleteWithImage.
type OpenAIClient struct {
	cfg        Config
	httpClient *http.Client
	baseURL    string
}

// newOpenAIClient constructs an OpenAIClient from cfg.
func newOpenAIClient(cfg Config) *OpenAIClient {
	base := cfg.BaseURL
	if base == "" {
		base = openAIDefaultBaseURL
	}
	return &OpenAIClient{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: openAIDefaultTimeout},
		baseURL:    base,
	}
}

// openAIRequest is the request body sent to /v1/chat/completions.
type openAIRequest struct {
	Model       string          `json:"model"`
	Messages    []openAIMessage `json:"messages"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Temperature float64         `json:"temperature"`
}

// openAIMessage is a single message in the chat history.
type openAIMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"` // string or []openAIContentPart
}

// openAIContentPart represents a multimodal content part.
type openAIContentPart struct {
	Type     string            `json:"type"`
	Text     string            `json:"text,omitempty"`
	ImageURL *openAIImageURL   `json:"image_url,omitempty"`
}

// openAIImageURL holds the data URL for a vision part.
type openAIImageURL struct {
	URL string `json:"url"`
}

// openAIResponse is the decoded response from /v1/chat/completions.
type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *openAIError `json:"error,omitempty"`
}

// openAIError is the error envelope returned by the OpenAI API.
type openAIError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

// Complete sends a text-only prompt to the model and returns the completion.
// Implements domain.LLMClient.
func (c *OpenAIClient) Complete(ctx context.Context, prompt string) (string, error) {
	msgs := []openAIMessage{
		{Role: "user", Content: prompt},
	}
	return c.doChat(ctx, msgs)
}

// CompleteWithImage sends a prompt with an inline image to the model.
// Implements domain.VisionLLMClient.
func (c *OpenAIClient) CompleteWithImage(ctx context.Context, prompt, imageBase64, mimeType string) (string, error) {
	parts := []openAIContentPart{
		{Type: "text", Text: prompt},
		{
			Type: "image_url",
			ImageURL: &openAIImageURL{
				URL: "data:" + mimeType + ";base64," + imageBase64,
			},
		},
	}
	msgs := []openAIMessage{
		{Role: "user", Content: parts},
	}
	return c.doChat(ctx, msgs)
}

// doChat executes a chat completion with exponential backoff on 429 responses.
func (c *OpenAIClient) doChat(ctx context.Context, msgs []openAIMessage) (string, error) {
	reqBody := openAIRequest{
		Model:       c.cfg.Model,
		Messages:    msgs,
		MaxTokens:   c.cfg.MaxTokens,
		Temperature: c.cfg.Temperature,
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

		result, retry, err := c.sendChatRequest(ctx, reqBody)
		if err != nil && !retry {
			return "", err
		}
		if err != nil {
			lastErr = err
			continue
		}
		return result, nil
	}
	return "", fmt.Errorf("openai: max retries exceeded: %w", lastErr)
}

// sendChatRequest performs a single HTTP POST to the completions endpoint.
// Returns (result, retry, error) — retry=true means the caller should back off and retry.
func (c *OpenAIClient) sendChatRequest(ctx context.Context, reqBody openAIRequest) (string, bool, error) {
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", false, fmt.Errorf("openai: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", false, fmt.Errorf("openai: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", false, fmt.Errorf("openai: http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return "", true, fmt.Errorf("openai: rate limited (429)")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", false, fmt.Errorf("openai: read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", false, fmt.Errorf("openai: unexpected status %d: %s", resp.StatusCode, body)
	}

	var oaiResp openAIResponse
	if err := json.Unmarshal(body, &oaiResp); err != nil {
		return "", false, fmt.Errorf("openai: parse response: %w", err)
	}
	if oaiResp.Error != nil {
		return "", false, fmt.Errorf("openai: api error: %s", oaiResp.Error.Message)
	}
	if len(oaiResp.Choices) == 0 {
		return "", false, fmt.Errorf("openai: empty choices in response")
	}
	return oaiResp.Choices[0].Message.Content, false, nil
}

// backoffDuration returns 2^attempt * 500ms as the wait before retry attempt.
func backoffDuration(attempt int) time.Duration {
	ms := 500 * (1 << uint(attempt))
	return time.Duration(ms) * time.Millisecond
}
