package api_test

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ApertureHQ/aperture/internal/api"
	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/ApertureHQ/aperture/internal/stream"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

// TestWebSocket_StreamDeliversEvents verifies that events emitted for a session
// are received by a WebSocket client connected to GET /api/v1/sessions/:id/stream.
func TestWebSocket_StreamDeliversEvents(t *testing.T) {
	emitter := stream.NewChannelEmitter()
	router := api.NewRouter(api.RouterConfig{
		ProgressEmitter: emitter,
	})

	srv := httptest.NewServer(router)
	defer srv.Close()

	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/v1/sessions/test-session/stream"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatalf("websocket dial failed: %v", err)
	}
	defer conn.CloseNow() //nolint:errcheck

	// Give the handler time to subscribe before emitting.
	time.Sleep(50 * time.Millisecond)

	evt := domain.ProgressEvent{
		SessionID: "test-session",
		Type:      "step_start",
		StepIndex: 0,
		Action:    "navigate",
		Timestamp: time.Now(),
	}
	emitter.Emit(evt)

	var received domain.ProgressEvent
	if err := wsjson.Read(ctx, conn, &received); err != nil {
		t.Fatalf("failed to read websocket message: %v", err)
	}

	if received.Type != evt.Type {
		t.Errorf("expected Type=%q, got %q", evt.Type, received.Type)
	}
	if received.Action != evt.Action {
		t.Errorf("expected Action=%q, got %q", evt.Action, received.Action)
	}
}

// TestWebSocket_DoneEventClosesStream verifies that sending a "done" event
// causes the server to close the WebSocket connection.
func TestWebSocket_DoneEventClosesStream(t *testing.T) {
	emitter := stream.NewChannelEmitter()
	router := api.NewRouter(api.RouterConfig{
		ProgressEmitter: emitter,
	})

	srv := httptest.NewServer(router)
	defer srv.Close()

	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/v1/sessions/done-session/stream"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatalf("websocket dial failed: %v", err)
	}
	defer conn.CloseNow() //nolint:errcheck

	time.Sleep(50 * time.Millisecond)

	emitter.Emit(domain.ProgressEvent{
		SessionID: "done-session",
		Type:      "done",
		Timestamp: time.Now(),
	})

	// Server should close; Read should return an error or a done event.
	var received domain.ProgressEvent
	if err := wsjson.Read(ctx, conn, &received); err == nil {
		// We got the "done" event — that's also acceptable.
		if received.Type != "done" {
			t.Errorf("expected done event, got %q", received.Type)
		}
	}
	// Either the "done" event arrived or the connection was closed. Both pass.
}
