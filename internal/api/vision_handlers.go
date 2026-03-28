// Package api provides HTTP routing and handler wiring for the Aperture server.
// This file contains the handler for vision-based screenshot analysis.
package api

import (
	"net/http"

	"github.com/ApertureHQ/aperture/internal/domain"
)

// VisionHandlers groups HTTP handlers for vision analysis endpoints.
type VisionHandlers struct {
	screenshot domain.ScreenshotService
	vision     domain.VisionAnalyzer
}

// NewVisionHandlers constructs VisionHandlers.
func NewVisionHandlers(screenshot domain.ScreenshotService, vision domain.VisionAnalyzer) *VisionHandlers {
	return &VisionHandlers{screenshot: screenshot, vision: vision}
}

// VisionRequest is the JSON body for POST /api/v1/actions/vision.
type VisionRequest struct {
	URL    string `json:"url"`
	Prompt string `json:"prompt,omitempty"`
}

// VisionResponse is the JSON body returned by the vision endpoint.
type VisionAPIResponse struct {
	Description    string              `json:"description"`
	Elements       []domain.ElementDesc `json:"elements"`
	SuggestedSteps []string            `json:"suggested_steps"`
}

// Analyze handles POST /api/v1/actions/vision.
// Takes a screenshot of the URL and analyzes it with the vision LLM.
func (h *VisionHandlers) Analyze(w http.ResponseWriter, r *http.Request) {
	var req VisionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	if req.URL == "" {
		writeError(w, http.StatusBadRequest, "MISSING_URL", "field 'url' is required")
		return
	}

	if h.screenshot == nil {
		writeError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "screenshot service not configured")
		return
	}
	if h.vision == nil {
		writeError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "vision analyzer not configured")
		return
	}

	ctx := r.Context()

	buf, err := h.screenshot.Screenshot(ctx, req.URL, false)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SCREENSHOT_FAILED", err.Error())
		return
	}

	visionReq := &domain.VisionRequest{
		Screenshot: buf,
		Prompt:     req.Prompt,
		PageURL:    req.URL,
	}

	resp, err := h.vision.Analyze(ctx, visionReq)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "VISION_FAILED", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, VisionAPIResponse{
		Description:    resp.Description,
		Elements:       resp.Elements,
		SuggestedSteps: resp.SuggestedSteps,
	})
}
