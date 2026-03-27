// Package vision provides screenshot analysis using vision-capable LLMs.
// LLMVisionAnalyzer sends page screenshots to a VisionLLMClient and parses
// the structured JSON response into a VisionResponse.
package vision

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ApertureHQ/aperture/internal/domain"
)

// LLMVisionAnalyzer implements domain.VisionAnalyzer using a vision-capable LLM.
type LLMVisionAnalyzer struct {
	client domain.VisionLLMClient
}

// NewLLMVisionAnalyzer returns an LLMVisionAnalyzer backed by client.
func NewLLMVisionAnalyzer(client domain.VisionLLMClient) *LLMVisionAnalyzer {
	return &LLMVisionAnalyzer{client: client}
}

// Compile-time check: LLMVisionAnalyzer must implement domain.VisionAnalyzer.
var _ domain.VisionAnalyzer = (*LLMVisionAnalyzer)(nil)

// visionLLMResponse mirrors the JSON structure requested from the LLM.
type visionLLMResponse struct {
	Description    string               `json:"description"`
	Elements       []domain.ElementDesc `json:"elements"`
	SuggestedSteps []string             `json:"suggested_steps"`
}

// Analyze encodes the screenshot as base64, sends it to the vision LLM with a
// structured prompt, and parses the response. Falls back to raw text if JSON
// parsing fails.
func (a *LLMVisionAnalyzer) Analyze(ctx context.Context, req *domain.VisionRequest) (*domain.VisionResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("vision: request must not be nil")
	}

	mimeType := detectMIMEType(req.Screenshot)
	imageBase64 := base64.StdEncoding.EncodeToString(req.Screenshot)

	prompt := buildVisionPrompt(req)

	raw, err := a.client.CompleteWithImage(ctx, prompt, imageBase64, mimeType)
	if err != nil {
		return nil, fmt.Errorf("vision: llm complete: %w", err)
	}

	return parseVisionResponse(raw), nil
}

// buildVisionPrompt constructs the structured analysis prompt sent to the vision LLM.
func buildVisionPrompt(req *domain.VisionRequest) string {
	var sb strings.Builder
	sb.WriteString("You are a browser automation assistant analyzing a screenshot.\n")
	if req.PageURL != "" {
		sb.WriteString(fmt.Sprintf("Page URL: %s\n", req.PageURL))
	}
	if req.PageTitle != "" {
		sb.WriteString(fmt.Sprintf("Page title: %s\n", req.PageTitle))
	}
	if req.Prompt != "" {
		sb.WriteString(fmt.Sprintf("Analysis goal: %s\n", req.Prompt))
	}
	sb.WriteString("\n")
	sb.WriteString("Analyze the screenshot and respond with a JSON object matching this schema:\n")
	sb.WriteString(`{
  "description": "a concise summary of what is visible on the page",
  "elements": [
    {
      "type": "button|input|link|select|textarea|other",
      "description": "human-readable label, e.g. 'Submit button' or 'Email input'",
      "selector": "suggested CSS selector, e.g. 'button[type=submit]'"
    }
  ],
  "suggested_steps": [
    "natural-language action that could be taken, e.g. 'Click the Login button'"
  ]
}`)
	sb.WriteString("\nRespond with only the JSON object, no prose or markdown fences.")
	return sb.String()
}

// parseVisionResponse attempts JSON parsing of raw; falls back to a text-only response.
func parseVisionResponse(raw string) *domain.VisionResponse {
	cleaned := stripMarkdownFences(strings.TrimSpace(raw))

	var parsed visionLLMResponse
	if err := json.Unmarshal([]byte(cleaned), &parsed); err == nil {
		return &domain.VisionResponse{
			Description:    parsed.Description,
			Elements:       parsed.Elements,
			SuggestedSteps: parsed.SuggestedSteps,
			Raw:            raw,
		}
	}

	// Fallback: treat the raw text as the description.
	return &domain.VisionResponse{
		Description: raw,
		Elements:    []domain.ElementDesc{},
		Raw:         raw,
	}
}

// detectMIMEType returns "image/png" or "image/jpeg" based on magic bytes.
func detectMIMEType(data []byte) string {
	if len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 {
		return "image/jpeg"
	}
	return "image/png"
}

// stripMarkdownFences removes surrounding ```json ... ``` or ``` ... ``` fences.
func stripMarkdownFences(s string) string {
	if !strings.HasPrefix(s, "```") {
		return s
	}
	// Remove first line (``` or ```json).
	idx := strings.Index(s, "\n")
	if idx == -1 {
		return s
	}
	s = s[idx+1:]
	s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	return strings.TrimSpace(s)
}
