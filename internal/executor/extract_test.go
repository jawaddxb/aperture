package executor_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/ApertureHQ/aperture/internal/executor"
)

// stubLLMClient satisfies domain.LLMClient for testing extraction.
type stubLLMClient struct {
	completeFn func(ctx context.Context, prompt string) (string, error)
}

func (s *stubLLMClient) Complete(ctx context.Context, prompt string) (string, error) {
	return s.completeFn(ctx, prompt)
}

func TestExtractExecutor_Success(t *testing.T) {
	inst := newTestBrowserInstance(t)
	defer inst.Close()

	html := `data:text/html,<html><body><div id="target"><h1>Product Title</h1><p>Price: $99.99</p></div></body></html>`
	nav := executor.NewNavigateExecutor()
	if _, err := nav.Execute(context.Background(), inst, map[string]interface{}{"url": html}); err != nil {
		t.Fatalf("navigate failed: %v", err)
	}

	mockLLM := &stubLLMClient{
		completeFn: func(_ context.Context, prompt string) (string, error) {
			if !strings.Contains(prompt, "Product Title") {
				return "", fmt.Errorf("prompt missing page content: %s", prompt)
			}
			if !strings.Contains(prompt, "JSON schema") {
				return "", fmt.Errorf("prompt missing schema: %s", prompt)
			}
			return `{"title": "Product Title", "price": 99.99}`, nil
		},
	}

	e := executor.NewExtractExecutor(mockLLM)
	result, err := e.Execute(context.Background(), inst, map[string]interface{}{
		"schema":   "JSON schema with title and price",
		"format":   "json",
		"selector": "#target",
	})

	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Execute failed: %s", result.Error)
	}

	want := `{"title": "Product Title", "price": 99.99}`
	if string(result.Data) != want {
		t.Errorf("got data %q, want %q", string(result.Data), want)
	}
}

func TestExtractExecutor_LLMFailure(t *testing.T) {
	inst := newTestBrowserInstance(t)
	defer inst.Close()

	mockLLM := &stubLLMClient{
		completeFn: func(_ context.Context, _ string) (string, error) {
			return "", fmt.Errorf("llm error")
		},
	}

	e := executor.NewExtractExecutor(mockLLM)
	result, err := e.Execute(context.Background(), inst, map[string]interface{}{
		"schema": "test schema",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Fatal("expected failure on LLM error")
	}
	if !strings.Contains(result.Error, "llm error") {
		t.Errorf("error %q should mention llm error", result.Error)
	}
}
