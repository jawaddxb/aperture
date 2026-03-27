package observe_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/ApertureHQ/aperture/internal/api"
	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/ApertureHQ/aperture/internal/observe"
)

// ─── Logger tests ─────────────────────────────────────────────────────────────

func TestJSONLogger_OutputsValidJSON(t *testing.T) {
	var buf bytes.Buffer
	l := observe.NewJSONLogger(&buf)

	l.Log(domain.LogEntry{
		Level:   "info",
		Message: "test message",
	})

	var entry domain.LogEntry
	if err := json.NewDecoder(&buf).Decode(&entry); err != nil {
		t.Fatalf("output is not valid JSON: %v\nraw: %s", err, buf.String())
	}
	if entry.Level != "info" {
		t.Errorf("expected level=info, got %q", entry.Level)
	}
	if entry.Message != "test message" {
		t.Errorf("expected message=%q, got %q", "test message", entry.Message)
	}
}

func TestJSONLogger_WithSessionChaining(t *testing.T) {
	var buf bytes.Buffer
	l := observe.NewJSONLogger(&buf)

	sessionLogger := l.WithSession("session-42")
	sessionLogger.Log(domain.LogEntry{
		Level:   "debug",
		Message: "scoped entry",
	})

	var entry domain.LogEntry
	if err := json.NewDecoder(&buf).Decode(&entry); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if entry.SessionID != "session-42" {
		t.Errorf("expected SessionID=session-42, got %q", entry.SessionID)
	}
}

func TestJSONLogger_WithActionChaining(t *testing.T) {
	var buf bytes.Buffer
	l := observe.NewJSONLogger(&buf)

	actionLogger := l.WithAction("click")
	actionLogger.Log(domain.LogEntry{
		Level:   "info",
		Message: "action entry",
	})

	var entry domain.LogEntry
	if err := json.NewDecoder(&buf).Decode(&entry); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if entry.Action != "click" {
		t.Errorf("expected Action=click, got %q", entry.Action)
	}
}

func TestJSONLogger_WithSessionAndActionChaining(t *testing.T) {
	var buf bytes.Buffer
	l := observe.NewJSONLogger(&buf)

	chained := l.WithSession("sess-1").WithAction("navigate")
	chained.Log(domain.LogEntry{Level: "warn", Message: "chained"})

	var entry domain.LogEntry
	if err := json.NewDecoder(&buf).Decode(&entry); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if entry.SessionID != "sess-1" {
		t.Errorf("expected SessionID=sess-1, got %q", entry.SessionID)
	}
	if entry.Action != "navigate" {
		t.Errorf("expected Action=navigate, got %q", entry.Action)
	}
}

func TestJSONLogger_TimestampSetAutomatically(t *testing.T) {
	var buf bytes.Buffer
	l := observe.NewJSONLogger(&buf)
	before := time.Now()
	l.Log(domain.LogEntry{Level: "info", Message: "ts test"})
	after := time.Now()

	var entry domain.LogEntry
	if err := json.NewDecoder(&buf).Decode(&entry); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if entry.Timestamp.Before(before) || entry.Timestamp.After(after) {
		t.Errorf("timestamp %v not in range [%v, %v]", entry.Timestamp, before, after)
	}
}

// ─── Metrics tests ────────────────────────────────────────────────────────────

func TestMetrics_RecordAndSnapshot(t *testing.T) {
	m := observe.NewInMemoryMetrics()
	m.RecordAction("click", 100*time.Millisecond, true)
	m.RecordAction("click", 200*time.Millisecond, true)
	m.RecordAction("navigate", 50*time.Millisecond, false)
	m.RecordSession(time.Second, 3, 1)

	snap := m.Snapshot()

	if snap.TotalActions != 3 {
		t.Errorf("expected TotalActions=3, got %d", snap.TotalActions)
	}
	if snap.TotalSessions != 1 {
		t.Errorf("expected TotalSessions=1, got %d", snap.TotalSessions)
	}
	if snap.ActionCounts["click"] != 2 {
		t.Errorf("expected click count=2, got %d", snap.ActionCounts["click"])
	}
	if snap.ActionCounts["navigate"] != 1 {
		t.Errorf("expected navigate count=1, got %d", snap.ActionCounts["navigate"])
	}
	if snap.AvgDurationMs["click"] <= 0 {
		t.Errorf("expected positive avg duration for click")
	}
	if snap.ErrorRate == 0 {
		t.Error("expected non-zero error rate after failed action")
	}
}

func TestMetrics_ConcurrentSafe(t *testing.T) {
	m := observe.NewInMemoryMetrics()
	const goroutines = 100

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			action := "action"
			if i%2 == 0 {
				action = "other"
			}
			m.RecordAction(action, time.Millisecond, i%3 != 0)
		}(i)
	}
	wg.Wait()

	snap := m.Snapshot()
	if snap.TotalActions != goroutines {
		t.Errorf("expected %d total actions, got %d", goroutines, snap.TotalActions)
	}
}

// ─── Metrics endpoint test ────────────────────────────────────────────────────

func TestMetricsEndpoint_ReturnsJSON(t *testing.T) {
	m := observe.NewInMemoryMetrics()
	m.RecordAction("navigate", 10*time.Millisecond, true)

	router := api.NewRouter(api.RouterConfig{
		MetricsCollector: m,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	var snap domain.MetricsSnapshot
	if err := json.NewDecoder(rec.Body).Decode(&snap); err != nil {
		t.Fatalf("response is not valid JSON: %v\nraw: %s", err, rec.Body.String())
	}
	if snap.TotalActions != 1 {
		t.Errorf("expected TotalActions=1, got %d", snap.TotalActions)
	}
}
