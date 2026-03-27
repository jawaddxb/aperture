// Package api provides HTTP routing and handler wiring for the Aperture server.
// This file implements the WebSocket streaming endpoint for session progress events.
package api

import (
	"context"
	"net/http"

	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/go-chi/chi/v5"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

// StreamHandler handles WebSocket connections for session progress streaming.
// It depends on a domain.ProgressEmitter to receive events.
type StreamHandler struct {
	emitter domain.ProgressEmitter
}

// NewStreamHandler constructs a StreamHandler with the given emitter.
func NewStreamHandler(emitter domain.ProgressEmitter) *StreamHandler {
	return &StreamHandler{emitter: emitter}
}

// Stream handles GET /api/v1/sessions/:id/stream.
// It upgrades the connection to WebSocket, subscribes to session events,
// and forwards them as JSON until the session completes or the client disconnects.
func (h *StreamHandler) Stream(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "id")
	if sessionID == "" {
		http.Error(w, "missing session id", http.StatusBadRequest)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // Allow all origins in development.
	})
	if err != nil {
		return // Accept already wrote an HTTP error response.
	}
	defer conn.CloseNow() //nolint:errcheck

	ch := h.emitter.Subscribe(sessionID)
	defer h.emitter.Unsubscribe(sessionID, ch)

	h.forwardEvents(r.Context(), conn, ch)
}

// forwardEvents reads events from ch and writes them to the WebSocket connection.
// Returns when ctx is done, the client disconnects, or a "done" event is received.
func (h *StreamHandler) forwardEvents(
	ctx context.Context,
	conn *websocket.Conn,
	ch <-chan domain.ProgressEvent,
) {
	for {
		select {
		case <-ctx.Done():
			conn.Close(websocket.StatusNormalClosure, "context cancelled") //nolint:errcheck
			return
		case evt, ok := <-ch:
			if !ok {
				conn.Close(websocket.StatusNormalClosure, "session ended") //nolint:errcheck
				return
			}
			if err := wsjson.Write(ctx, conn, evt); err != nil {
				return // Client disconnected.
			}
			if evt.Type == "done" {
				conn.Close(websocket.StatusNormalClosure, "done") //nolint:errcheck
				return
			}
		}
	}
}


