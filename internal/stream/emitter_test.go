package stream_test

import (
	"sync"
	"testing"
	"time"

	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/ApertureHQ/aperture/internal/stream"
)

func makeEvent(sessionID, typ string, idx int) domain.ProgressEvent {
	return domain.ProgressEvent{
		SessionID: sessionID,
		Type:      typ,
		StepIndex: idx,
		Timestamp: time.Now(),
	}
}

func TestEmitter_FanOutToMultipleSubscribers(t *testing.T) {
	e := stream.NewChannelEmitter()
	ch1 := e.Subscribe("sess-1")
	ch2 := e.Subscribe("sess-1")
	defer e.Unsubscribe("sess-1", ch1)
	defer e.Unsubscribe("sess-1", ch2)

	evt := makeEvent("sess-1", "step_start", 0)
	e.Emit(evt)

	assertReceives(t, ch1, evt, "subscriber 1")
	assertReceives(t, ch2, evt, "subscriber 2")
}

func TestEmitter_UnsubscribeStopsReceiving(t *testing.T) {
	e := stream.NewChannelEmitter()
	ch := e.Subscribe("sess-2")

	e.Unsubscribe("sess-2", ch)

	// After unsubscribe, emitting must not deliver events to the old channel.
	e.Emit(makeEvent("sess-2", "done", 0))

	// Give a short window to ensure no event arrives.
	select {
	case evt := <-ch:
		t.Errorf("expected no events after unsubscribe, got: %+v", evt)
	case <-time.After(100 * time.Millisecond):
		// Correct: no event delivered after unsubscribe.
	}
}

func TestEmitter_SlowSubscriberDoesNotBlock(t *testing.T) {
	e := stream.NewChannelEmitter()

	// Create subscriber but never read from it.
	ch := e.Subscribe("sess-3")
	defer e.Unsubscribe("sess-3", ch)

	// Emit many events; buffer size is 64. Exceed it to ensure non-blocking.
	done := make(chan struct{})
	go func() {
		for i := 0; i < 200; i++ {
			e.Emit(makeEvent("sess-3", "step_start", i))
		}
		close(done)
	}()

	select {
	case <-done:
		// All emits completed without blocking — pass.
	case <-time.After(2 * time.Second):
		t.Fatal("Emit blocked on slow subscriber")
	}
}

func TestEmitter_IsolationBetweenSessions(t *testing.T) {
	e := stream.NewChannelEmitter()
	chA := e.Subscribe("sess-a")
	chB := e.Subscribe("sess-b")
	defer e.Unsubscribe("sess-a", chA)
	defer e.Unsubscribe("sess-b", chB)

	evtA := makeEvent("sess-a", "done", 0)
	e.Emit(evtA)

	assertReceives(t, chA, evtA, "session A subscriber")

	// Session B should not have received session A's event.
	select {
	case got := <-chB:
		t.Errorf("session B received unexpected event: %+v", got)
	case <-time.After(50 * time.Millisecond):
		// Correct: no event delivered.
	}
}

func TestEmitter_ConcurrentEmitSubscribeUnsubscribe(t *testing.T) {
	e := stream.NewChannelEmitter()
	var wg sync.WaitGroup
	const goroutines = 20

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ch := e.Subscribe("concurrent")
			e.Emit(makeEvent("concurrent", "step_start", i))
			e.Unsubscribe("concurrent", ch)
		}(i)
	}

	wg.Wait() // Must not deadlock or race.
}

// assertReceives asserts that ch yields an event matching expected within 500ms.
func assertReceives(t *testing.T, ch <-chan domain.ProgressEvent, expected domain.ProgressEvent, label string) {
	t.Helper()
	select {
	case got := <-ch:
		if got.Type != expected.Type || got.SessionID != expected.SessionID {
			t.Errorf("%s: expected event %+v, got %+v", label, expected, got)
		}
	case <-time.After(500 * time.Millisecond):
		t.Errorf("%s: timed out waiting for event", label)
	}
}
