package vision_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/ApertureHQ/aperture/internal/llm"
	"github.com/ApertureHQ/aperture/internal/vision"
)

// ─── mock vision client ───────────────────────────────────────────────────────

type mockVisionClient struct {
	response string
	err      error
}

func (m *mockVisionClient) CompleteWithImage(_ context.Context, _, _, _ string) (string, error) {
	return m.response, m.err
}

// ─── tests ────────────────────────────────────────────────────────────────────

// TestLLMVisionAnalyzer_ParsedResponse verifies structured JSON is parsed correctly.
func TestLLMVisionAnalyzer_ParsedResponse(t *testing.T) {
	resp := map[string]interface{}{
		"description": "A login page with email and password fields",
		"elements": []map[string]interface{}{
			{"type": "input", "description": "Email input", "selector": "input[type=email]"},
			{"type": "button", "description": "Submit button", "selector": "button[type=submit]"},
		},
		"suggested_steps": []string{"Type email", "Click Submit"},
	}
	raw, _ := json.Marshal(resp)

	analyzer := vision.NewLLMVisionAnalyzer(&mockVisionClient{response: string(raw)})
	result, err := analyzer.Analyze(context.Background(), &domain.VisionRequest{
		Screenshot: []byte{0x89, 0x50, 0x4E, 0x47}, // PNG magic bytes
		Prompt:     "Identify interactive elements",
		PageURL:    "https://example.com/login",
		PageTitle:  "Login",
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if result.Description != "A login page with email and password fields" {
		t.Errorf("unexpected description: %q", result.Description)
	}
	if len(result.Elements) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(result.Elements))
	}
	if result.Elements[0].Type != "input" {
		t.Errorf("expected type=input, got %q", result.Elements[0].Type)
	}
	if len(result.SuggestedSteps) != 2 {
		t.Errorf("expected 2 suggested steps, got %d", len(result.SuggestedSteps))
	}
	if result.Raw != string(raw) {
		t.Error("Raw field should contain original LLM response")
	}
}

// TestLLMVisionAnalyzer_MalformedResponse verifies graceful fallback on bad JSON.
func TestLLMVisionAnalyzer_MalformedResponse(t *testing.T) {
	analyzer := vision.NewLLMVisionAnalyzer(&mockVisionClient{response: "This is not JSON at all"})
	result, err := analyzer.Analyze(context.Background(), &domain.VisionRequest{
		Screenshot: []byte{0x89, 0x50},
		Prompt:     "anything",
	})
	if err != nil {
		t.Fatalf("expected no error on malformed response, got: %v", err)
	}
	if result.Description != "This is not JSON at all" {
		t.Errorf("expected raw text as description, got %q", result.Description)
	}
	if result.Elements == nil {
		t.Error("Elements should be non-nil even on fallback")
	}
}

// TestLLMVisionAnalyzer_MarkdownFencedResponse verifies fenced JSON is handled.
func TestLLMVisionAnalyzer_MarkdownFencedResponse(t *testing.T) {
	inner := `{"description":"A search page","elements":[],"suggested_steps":["Type a query"]}`
	fenced := "```json\n" + inner + "\n```"

	analyzer := vision.NewLLMVisionAnalyzer(&mockVisionClient{response: fenced})
	result, err := analyzer.Analyze(context.Background(), &domain.VisionRequest{
		Screenshot: []byte{0x89, 0x50},
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if result.Description != "A search page" {
		t.Errorf("expected 'A search page', got %q", result.Description)
	}
}

// TestLLMVisionAnalyzer_ClientError verifies error propagation.
func TestLLMVisionAnalyzer_ClientError(t *testing.T) {
	analyzer := vision.NewLLMVisionAnalyzer(&mockVisionClient{err: context.DeadlineExceeded})
	_, err := analyzer.Analyze(context.Background(), &domain.VisionRequest{
		Screenshot: []byte{0x89, 0x50},
	})
	if err == nil {
		t.Fatal("expected error from failing client")
	}
}

// TestOpenAIVisionRequestFormat verifies the vision request body via httptest server.
func TestOpenAIVisionRequestFormat(t *testing.T) {
	var capturedBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedBody)
		resp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]interface{}{
					"content": `{"description":"a page","elements":[],"suggested_steps":[]}`,
				}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client, err := llm.NewClient(llm.Config{
		Provider: llm.ProviderOpenAI,
		Model:    "gpt-4o",
		APIKey:   "test-key",
		BaseURL:  srv.URL,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	vc, ok := client.(domain.VisionLLMClient)
	if !ok {
		t.Fatal("OpenAIClient must implement VisionLLMClient")
	}

	imgBytes := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A} // PNG header
	analyzer := vision.NewLLMVisionAnalyzer(vc)
	_, err = analyzer.Analyze(context.Background(), &domain.VisionRequest{
		Screenshot: imgBytes,
		Prompt:     "what do you see",
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	// Verify request shape sent to OpenAI.
	messages, _ := capturedBody["messages"].([]interface{})
	if len(messages) == 0 {
		t.Fatal("no messages in captured body")
	}
	msg := messages[0].(map[string]interface{})
	content, _ := msg["content"].([]interface{})
	if len(content) < 2 {
		t.Fatalf("expected at least 2 content parts, got %d", len(content))
	}

	imagePart := content[1].(map[string]interface{})
	if imagePart["type"] != "image_url" {
		t.Errorf("expected image_url part, got %v", imagePart["type"])
	}
	imageURL := imagePart["image_url"].(map[string]interface{})
	expectedURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(imgBytes)
	if imageURL["url"] != expectedURL {
		t.Errorf("unexpected image URL format")
	}
}
