// Package observe provides structured logging and metrics for Aperture.
// This file implements JSONLogger, a thread-safe structured JSON logger.
package observe

import (
	"encoding/json"
	"io"
	"os"
	"sync"
	"time"

	"github.com/ApertureHQ/aperture/internal/domain"
)

// JSONLogger writes LogEntry values as JSON lines to an io.Writer.
// It satisfies domain.ActionLogger.
// The zero value is not usable; construct with NewJSONLogger.
type JSONLogger struct {
	mu        sync.Mutex
	w         io.Writer
	sessionID string
	action    string
}

// NewJSONLogger creates a JSONLogger that writes to w.
// Pass nil to default to os.Stderr.
func NewJSONLogger(w io.Writer) *JSONLogger {
	if w == nil {
		w = os.Stderr
	}
	return &JSONLogger{w: w}
}

// Log encodes entry as a JSON line and writes it to the underlying writer.
// Pre-set sessionID and action fields are merged into the entry.
// Thread-safe.
func (l *JSONLogger) Log(entry domain.LogEntry) {
	if entry.SessionID == "" {
		entry.SessionID = l.sessionID
	}
	if entry.Action == "" {
		entry.Action = l.action
	}
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if err := json.NewEncoder(l.w).Encode(entry); err != nil {
		// Best-effort: cannot log the logging error without recursion.
		_ = err
	}
}

// WithSession returns a new JSONLogger that pre-sets SessionID to sessionID.
// The returned logger shares the same writer and mutex-free fields copy.
func (l *JSONLogger) WithSession(sessionID string) domain.ActionLogger {
	return &JSONLogger{
		w:         l.w,
		sessionID: sessionID,
		action:    l.action,
	}
}

// WithAction returns a new JSONLogger that pre-sets Action to action.
func (l *JSONLogger) WithAction(action string) domain.ActionLogger {
	return &JSONLogger{
		w:         l.w,
		sessionID: l.sessionID,
		action:    action,
	}
}

// compile-time interface check.
var _ domain.ActionLogger = (*JSONLogger)(nil)
