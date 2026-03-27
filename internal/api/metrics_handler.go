// Package api provides HTTP routing and handler wiring for the Aperture server.
// This file implements the metrics endpoint.
package api

import (
	"net/http"

	"github.com/ApertureHQ/aperture/internal/domain"
)

// MetricsHandler serves the GET /api/v1/metrics endpoint.
type MetricsHandler struct {
	collector domain.MetricsCollector
}

// NewMetricsHandler constructs a MetricsHandler backed by collector.
func NewMetricsHandler(collector domain.MetricsCollector) *MetricsHandler {
	return &MetricsHandler{collector: collector}
}

// GetMetrics handles GET /api/v1/metrics.
// It returns a JSON-encoded MetricsSnapshot of current runtime statistics.
func (h *MetricsHandler) GetMetrics(w http.ResponseWriter, r *http.Request) {
	snap := h.collector.Snapshot()
	writeJSON(w, http.StatusOK, snap)
}
