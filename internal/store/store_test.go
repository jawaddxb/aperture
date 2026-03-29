// Package store tests SQLiteStore and session recovery behaviour.
package store

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	_ "modernc.org/sqlite" // register "sqlite" driver
)

func newTestStore(t *testing.T) (*SQLiteStore, func()) {
	t.Helper()
	f, err := os.CreateTemp("", "aperture-test-*.db")
	if err != nil {
		t.Fatalf("create temp db: %v", err)
	}
	f.Close()
	s, err := NewSQLiteStore(f.Name())
	if err != nil {
		os.Remove(f.Name())
		t.Fatalf("new store: %v", err)
	}
	return s, func() {
		s.Close()
		os.Remove(f.Name())
	}
}

// TestSessionRecovery_ActiveMarkedInterrupted verifies that ListSessions returns
// "active" sessions and that UpdateSessionStatus can mark them "interrupted".
// This is the core of the startup session recovery logic in main.go.
func TestSessionRecovery_ActiveMarkedInterrupted(t *testing.T) {
	s, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	// Insert two active sessions and one completed session.
	active1 := &SessionRecord{
		ID: "sess-001", AccountID: "acct-1", Goal: "scrape prices",
		Status: "active", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	active2 := &SessionRecord{
		ID: "sess-002", AccountID: "acct-1", Goal: "login flow",
		Status: "active", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	done := &SessionRecord{
		ID: "sess-003", AccountID: "acct-1", Goal: "completed work",
		Status: "completed", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	for _, rec := range []*SessionRecord{active1, active2, done} {
		if err := s.SaveSession(ctx, rec); err != nil {
			t.Fatalf("SaveSession %s: %v", rec.ID, err)
		}
	}

	// Simulate recoverInterruptedSessions logic.
	sessions, err := s.ListSessions(ctx, "")
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	recovered := 0
	for _, rec := range sessions {
		if rec.Status == "active" {
			if err := s.UpdateSessionStatus(ctx, rec.ID, "interrupted"); err != nil {
				t.Errorf("UpdateSessionStatus %s: %v", rec.ID, err)
			}
			recovered++
		}
	}

	if recovered != 2 {
		t.Errorf("expected 2 sessions recovered, got %d", recovered)
	}

	// Verify DB state.
	for _, id := range []string{"sess-001", "sess-002"} {
		rec, err := s.GetSession(ctx, id)
		if err != nil {
			t.Fatalf("GetSession %s: %v", id, err)
		}
		if rec.Status != "interrupted" {
			t.Errorf("session %s: expected status 'interrupted', got %q", id, rec.Status)
		}
	}
	// Completed session must remain unchanged.
	rec, err := s.GetSession(ctx, "sess-003")
	if err != nil {
		t.Fatalf("GetSession sess-003: %v", err)
	}
	if rec.Status != "completed" {
		t.Errorf("completed session status changed: got %q", rec.Status)
	}
}

// TestKVPersistence verifies that KV values round-trip through JSON correctly.
func TestKVPersistence(t *testing.T) {
	s, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	value := json.RawMessage(`{"counter":42,"flag":true}`)
	if err := s.SetKV(ctx, "agent-1", "state", value); err != nil {
		t.Fatalf("SetKV: %v", err)
	}

	got, ok, err := s.GetKV(ctx, "agent-1", "state")
	if err != nil || !ok {
		t.Fatalf("GetKV: err=%v ok=%v", err, ok)
	}
	if string(got) != string(value) {
		t.Errorf("KV round-trip: got %s, want %s", got, value)
	}
}

// TestPolicyPersistence verifies that policies survive a store close/reopen.
func TestPolicyPersistence(t *testing.T) {
	f, err := os.CreateTemp("", "aperture-policy-test-*.db")
	if err != nil {
		t.Fatalf("create temp db: %v", err)
	}
	f.Close()
	dbPath := f.Name()
	defer os.Remove(dbPath)

	pol := json.RawMessage(`{"blocked_domains":["evil.com"],"max_steps":10}`)
	ctx := context.Background()

	// Write in first store instance.
	s1, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("open store 1: %v", err)
	}
	if err := s1.SetPolicy(ctx, "agent-99", pol); err != nil {
		t.Fatalf("SetPolicy: %v", err)
	}
	s1.Close()

	// Read in second store instance (simulates restart).
	s2, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("open store 2: %v", err)
	}
	defer s2.Close()

	got, ok, err := s2.GetPolicy(ctx, "agent-99")
	if err != nil || !ok {
		t.Fatalf("GetPolicy after restart: err=%v ok=%v", err, ok)
	}
	if string(got) != string(pol) {
		t.Errorf("policy round-trip: got %s, want %s", got, pol)
	}
}
