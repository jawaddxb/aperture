// Package domain defines core interfaces for Aperture.
// This file defines vision-related interfaces and types.
package domain

import "context"

// VisionRequest carries the screenshot and context to the vision analyzer.
type VisionRequest struct {
	// Screenshot is the raw PNG or JPEG bytes of the page.
	Screenshot []byte

	// Prompt describes what to analyze in the screenshot.
	Prompt string

	// PageURL provides URL context to the analyzer.
	PageURL string

	// PageTitle provides document title context.
	PageTitle string
}

// VisionResponse holds the structured output from a vision analysis.
type VisionResponse struct {
	// Description summarises what is visible on the page.
	Description string

	// Elements lists interactive elements the analyzer identified.
	Elements []ElementDesc

	// SuggestedSteps lists actions that could be taken from this page state.
	SuggestedSteps []string

	// Raw is the unprocessed LLM response text.
	Raw string
}

// ElementDesc describes a single interactive element identified by the vision analyzer.
type ElementDesc struct {
	// Type is the element kind: "button", "input", "link", etc.
	Type string

	// Description is a human-readable label: "Submit button", "Email input field".
	Description string

	// Selector is the suggested CSS selector for the element.
	Selector string
}

// VisionAnalyzer analyzes browser screenshots using a vision-capable LLM.
type VisionAnalyzer interface {
	// Analyze sends the screenshot to the LLM and returns structured observations.
	Analyze(ctx context.Context, req *VisionRequest) (*VisionResponse, error)
}

// VisionLLMClient extends LLMClient with image support for vision-capable models.
type VisionLLMClient interface {
	// CompleteWithImage sends prompt + an inline image to the model.
	// imageBase64 is the base64-encoded image; mimeType is e.g. "image/png".
	CompleteWithImage(ctx context.Context, prompt string, imageBase64 string, mimeType string) (string, error)
}
