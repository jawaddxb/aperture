package llm_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/ApertureHQ/aperture/internal/llm"
)

// ─── factory tests ────────────────────────────────────────────────────────────

// TestNewClient_OpenAI verifies the factory returns a non-nil client for OpenAI.
func TestNewClient_OpenAI(t *testing.T) {
	c, err := llm.NewClient(llm.Config{
		Provider: llm.ProviderOpenAI,
		Model:    "gpt-4o",
		APIKey:   "test-key",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil client")
	}
	var _ domain.LLMClient = c
}

// TestNewClient_Anthropic verifies the factory returns a non-nil client for Anthropic.
func TestNewClient_Anthropic(t *testing.T) {
	c, err := llm.NewClient(llm.Config{
		Provider: llm.ProviderAnthropic,
		Model:    "claude-3-5-sonnet-20241022",
		APIKey:   "test-key",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil client")
	}
}

// TestNewClient_UnknownProvider verifies an error is returned for unknown providers.
func TestNewClient_UnknownProvider(t *testing.T) {
	_, err := llm.NewClient(llm.Config{
		Provider: "unknown",
		APIKey:   "key",
	})
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

// TestNewClient_EmptyAPIKey verifies an error is returned when APIKey is empty.
func TestNewClient_EmptyAPIKey(t *testing.T) {
	_, err := llm.NewClient(llm.Config{
		Provider: llm.ProviderOpenAI,
		Model:    "gpt-4o",
	})
	if err == nil {
		t.Fatal("expected error for empty api key")
	}
}

// ─── OpenAI client tests ──────────────────────────────────────────────────────

// TestOpenAIClient_Complete verifies a successful completion via mock server.
func TestOpenAIClient_Complete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Authorization") == "" {
			t.Error("missing Authorization header")
		}
		resp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]interface{}{"content": "hello from openai"}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c, err := llm.NewClient(llm.Config{
		Provider: llm.ProviderOpenAI,
		Model:    "gpt-4o",
		APIKey:   "test-key",
		BaseURL:  srv.URL,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	got, err := c.Complete(context.Background(), "say hi")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got != "hello from openai" {
		t.Errorf("expected 'hello from openai', got %q", got)
	}
}

// TestOpenAIClient_429Retry verifies that a 429 triggers retries and succeeds on second attempt.
func TestOpenAIClient_429Retry(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		resp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]interface{}{"content": "retry worked"}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c, err := llm.NewClient(llm.Config{
		Provider: llm.ProviderOpenAI,
		Model:    "gpt-4o",
		APIKey:   "test-key",
		BaseURL:  srv.URL,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	got, err := c.Complete(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Complete after retry: %v", err)
	}
	if got != "retry worked" {
		t.Errorf("unexpected response: %q", got)
	}
	if calls.Load() != 2 {
		t.Errorf("expected 2 calls, got %d", calls.Load())
	}
}

// TestOpenAIClient_VisionRequestFormat verifies the request body structure for vision.
func TestOpenAIClient_VisionRequestFormat(t *testing.T) {
	var capturedBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedBody)
		resp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]interface{}{"content": "i see a page"}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c, err := llm.NewClient(llm.Config{
		Provider: llm.ProviderOpenAI,
		Model:    "gpt-4o",
		APIKey:   "test-key",
		BaseURL:  srv.URL,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	// OpenAIClient also implements VisionLLMClient.
	vc, ok := c.(domain.VisionLLMClient)
	if !ok {
		t.Fatal("OpenAIClient does not implement domain.VisionLLMClient")
	}

	_, err = vc.CompleteWithImage(context.Background(), "what do you see?", "abc123", "image/png")
	if err != nil {
		t.Fatalf("CompleteWithImage: %v", err)
	}

	messages, _ := capturedBody["messages"].([]interface{})
	if len(messages) == 0 {
		t.Fatal("expected messages in request body")
	}
	msg := messages[0].(map[string]interface{})
	content, _ := msg["content"].([]interface{})
	if len(content) < 2 {
		t.Fatalf("expected 2 content parts, got %d", len(content))
	}

	imagePart := content[1].(map[string]interface{})
	if imagePart["type"] != "image_url" {
		t.Errorf("expected type=image_url, got %v", imagePart["type"])
	}
	imageURL := imagePart["image_url"].(map[string]interface{})
	if imageURL["url"] != "data:image/png;base64,abc123" {
		t.Errorf("unexpected image URL: %v", imageURL["url"])
	}
}

// ─── Anthropic client tests ───────────────────────────────────────────────────

// TestAnthropicClient_Complete verifies a successful completion via mock server.
func TestAnthropicClient_Complete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") == "" {
			t.Error("missing x-api-key header")
		}
		if r.Header.Get("anthropic-version") != "2023-06-01" {
			t.Errorf("unexpected anthropic-version: %s", r.Header.Get("anthropic-version"))
		}
		resp := map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": "hello from anthropic"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c, err := llm.NewClient(llm.Config{
		Provider: llm.ProviderAnthropic,
		Model:    "claude-3-5-sonnet-20241022",
		APIKey:   "test-key",
		BaseURL:  srv.URL,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	got, err := c.Complete(context.Background(), "say hi")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got != "hello from anthropic" {
		t.Errorf("expected 'hello from anthropic', got %q", got)
	}
}

// TestAnthropicClient_429Retry verifies that a 429 triggers retries.
func TestAnthropicClient_429Retry(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		resp := map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": "anthropic retry worked"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c, err := llm.NewClient(llm.Config{
		Provider: llm.ProviderAnthropic,
		Model:    "claude-3-5-sonnet-20241022",
		APIKey:   "test-key",
		BaseURL:  srv.URL,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	got, err := c.Complete(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Complete after retry: %v", err)
	}
	if got != "anthropic retry worked" {
		t.Errorf("unexpected response: %q", got)
	}
}

// TestAnthropicClient_VisionCompleteWithImage verifies the vision interface.
func TestAnthropicClient_VisionCompleteWithImage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": "i see a button"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c, err := llm.NewClient(llm.Config{
		Provider: llm.ProviderAnthropic,
		Model:    "claude-3-5-sonnet-20241022",
		APIKey:   "test-key",
		BaseURL:  srv.URL,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	vc, ok := c.(domain.VisionLLMClient)
	if !ok {
		t.Fatal("AnthropicClient does not implement domain.VisionLLMClient")
	}

	got, err := vc.CompleteWithImage(context.Background(), "what do you see?", "imgdata", "image/jpeg")
	if err != nil {
		t.Fatalf("CompleteWithImage: %v", err)
	}
	if got != "i see a button" {
		t.Errorf("unexpected response: %q", got)
	}
}
