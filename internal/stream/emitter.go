// Package stream provides ChannelEmitter, an in-memory pub/sub ProgressEmitter.
package stream

import (
	"sync"

	"github.com/ApertureHQ/aperture/internal/domain"
)

const defaultBufferSize = 64

// subscription pairs a delivery channel with a cancellation signal.
// The done channel is closed when the subscriber calls Unsubscribe.
// The ch channel is NEVER closed by the emitter; it is left for GC.
// This avoids the close-while-sending data race.
type subscription struct {
	ch   chan domain.ProgressEvent
	done chan struct{}
}

// isDone reports whether the subscription has been cancelled.
// Safe to call without any lock.
func (s *subscription) isDone() bool {
	select {
	case <-s.done:
		return true
	default:
		return false
	}
}

// ChannelEmitter implements domain.ProgressEmitter using Go channels.
// Events are fanned out per session ID. Slow subscribers drop events.
// Safe for concurrent use.
type ChannelEmitter struct {
	mu          sync.RWMutex
	subscribers map[string][]*subscription
}

// NewChannelEmitter constructs a ready-to-use ChannelEmitter.
func NewChannelEmitter() *ChannelEmitter {
	return &ChannelEmitter{
		subscribers: make(map[string][]*subscription),
	}
}

// Emit publishes event to every active subscriber of event.SessionID.
// Non-blocking: events are dropped for slow or cancelled subscribers.
func (e *ChannelEmitter) Emit(event domain.ProgressEvent) {
	e.mu.RLock()
	subs := e.copySubs(event.SessionID)
	e.mu.RUnlock()

	for _, sub := range subs {
		if sub.isDone() {
			continue
		}
		select {
		case sub.ch <- event:
		case <-sub.done:
			// Subscriber cancelled while sending; skip.
		default:
			// Buffer full; drop event for this subscriber.
		}
	}
}

// Subscribe returns a buffered channel that receives events for sessionID.
// The caller must call Unsubscribe when done to release resources.
func (e *ChannelEmitter) Subscribe(sessionID string) <-chan domain.ProgressEvent {
	sub := &subscription{
		ch:   make(chan domain.ProgressEvent, defaultBufferSize),
		done: make(chan struct{}),
	}
	e.mu.Lock()
	e.subscribers[sessionID] = append(e.subscribers[sessionID], sub)
	e.mu.Unlock()
	return sub.ch
}

// Unsubscribe signals that ch should no longer receive events and removes it
// from the fan-out list. The channel is NOT closed; callers should drain it
// as needed. Passing an unrecognised channel is a no-op.
func (e *ChannelEmitter) Unsubscribe(sessionID string, ch <-chan domain.ProgressEvent) {
	e.mu.Lock()
	defer e.mu.Unlock()

	subs := e.subscribers[sessionID]
	remaining := subs[:0]
	for _, sub := range subs {
		if sub.ch == ch {
			// Signal done so Emit stops delivering to this subscriber.
			select {
			case <-sub.done:
				// Already cancelled.
			default:
				close(sub.done)
			}
			continue
		}
		remaining = append(remaining, sub)
	}
	if len(remaining) == 0 {
		delete(e.subscribers, sessionID)
	} else {
		e.subscribers[sessionID] = remaining
	}
}

// copySubs returns a snapshot of subscriptions for sessionID.
// Caller must hold at least a read lock.
func (e *ChannelEmitter) copySubs(sessionID string) []*subscription {
	src := e.subscribers[sessionID]
	if len(src) == 0 {
		return nil
	}
	out := make([]*subscription, len(src))
	copy(out, src)
	return out
}

// compile-time interface check.
var _ domain.ProgressEmitter = (*ChannelEmitter)(nil)
