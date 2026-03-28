// Package store provides a persistent storage layer backed by SQLite.
// It stores session metadata, agent KV state, and xBPP policies.
// Browser instances are inherently ephemeral and remain in-memory.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// Store is the persistence interface for Aperture metadata.
type Store interface {
	// Sessions
	SaveSession(ctx context.Context, s *SessionRecord) error
	GetSession(ctx context.Context, id string) (*SessionRecord, error)
	ListSessions(ctx context.Context, accountID string) ([]*SessionRecord, error)
	UpdateSessionStatus(ctx context.Context, id, status string) error
	DeleteSession(ctx context.Context, id string) error

	// Agent KV
	SetKV(ctx context.Context, agentID, key string, value json.RawMessage) error
	GetKV(ctx context.Context, agentID, key string) (json.RawMessage, bool, error)
	ListKV(ctx context.Context, agentID string) (map[string]json.RawMessage, error)
	DeleteKV(ctx context.Context, agentID, key string) error

	// Policies
	SetPolicy(ctx context.Context, agentID string, policy json.RawMessage) error
	GetPolicy(ctx context.Context, agentID string) (json.RawMessage, bool, error)
	DeletePolicy(ctx context.Context, agentID string) error

	// Close releases resources.
	Close() error
}

// SessionRecord is the persistable subset of a session (no browser reference).
type SessionRecord struct {
	ID           string            `json:"id"`
	AccountID    string            `json:"account_id"`
	Goal         string            `json:"goal"`
	Status       string            `json:"status"`
	BrowserID    string            `json:"browser_id"`
	Metadata     map[string]string `json:"metadata"`
	LLMCallCount int               `json:"llm_call_count"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
}

// SQLiteStore implements Store using SQLite.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore opens (or creates) the SQLite database and runs migrations.
func NewSQLiteStore(path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("store: open %s: %w", path, err)
	}
	// WAL mode for concurrent readers.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: WAL mode: %w", err)
	}
	s := &SQLiteStore{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: migrate: %w", err)
	}
	return s, nil
}

func (s *SQLiteStore) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			account_id TEXT NOT NULL DEFAULT '',
			goal TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'active',
			browser_id TEXT NOT NULL DEFAULT '',
			metadata TEXT DEFAULT '{}',
			llm_call_count INTEGER DEFAULT 0,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_account ON sessions(account_id)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_status ON sessions(status)`,
		`CREATE TABLE IF NOT EXISTS agent_kv (
			agent_id TEXT NOT NULL,
			key TEXT NOT NULL,
			value TEXT NOT NULL,
			updated_at DATETIME NOT NULL,
			PRIMARY KEY (agent_id, key)
		)`,
		`CREATE TABLE IF NOT EXISTS policies (
			agent_id TEXT PRIMARY KEY,
			policy TEXT NOT NULL,
			updated_at DATETIME NOT NULL
		)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("exec %q: %w", stmt[:40], err)
		}
	}
	return nil
}

// ── Sessions ────────────────────────────────────────────────────────────────

func (s *SQLiteStore) SaveSession(_ context.Context, rec *SessionRecord) error {
	meta, _ := json.Marshal(rec.Metadata)
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO sessions (id, account_id, goal, status, browser_id, metadata, llm_call_count, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.ID, rec.AccountID, rec.Goal, rec.Status, rec.BrowserID,
		string(meta), rec.LLMCallCount, rec.CreatedAt, rec.UpdatedAt,
	)
	return err
}

func (s *SQLiteStore) GetSession(_ context.Context, id string) (*SessionRecord, error) {
	row := s.db.QueryRow(`SELECT id, account_id, goal, status, browser_id, metadata, llm_call_count, created_at, updated_at FROM sessions WHERE id = ?`, id)
	rec := &SessionRecord{}
	var metaStr string
	err := row.Scan(&rec.ID, &rec.AccountID, &rec.Goal, &rec.Status, &rec.BrowserID, &metaStr, &rec.LLMCallCount, &rec.CreatedAt, &rec.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(metaStr), &rec.Metadata)
	return rec, nil
}

func (s *SQLiteStore) ListSessions(_ context.Context, accountID string) ([]*SessionRecord, error) {
	var rows *sql.Rows
	var err error
	if accountID == "" {
		rows, err = s.db.Query(`SELECT id, account_id, goal, status, browser_id, metadata, llm_call_count, created_at, updated_at FROM sessions ORDER BY created_at DESC`)
	} else {
		rows, err = s.db.Query(`SELECT id, account_id, goal, status, browser_id, metadata, llm_call_count, created_at, updated_at FROM sessions WHERE account_id = ? ORDER BY created_at DESC`, accountID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*SessionRecord
	for rows.Next() {
		rec := &SessionRecord{}
		var metaStr string
		if err := rows.Scan(&rec.ID, &rec.AccountID, &rec.Goal, &rec.Status, &rec.BrowserID, &metaStr, &rec.LLMCallCount, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(metaStr), &rec.Metadata)
		results = append(results, rec)
	}
	return results, rows.Err()
}

func (s *SQLiteStore) UpdateSessionStatus(_ context.Context, id, status string) error {
	_, err := s.db.Exec(`UPDATE sessions SET status = ?, updated_at = ? WHERE id = ?`, status, time.Now(), id)
	return err
}

func (s *SQLiteStore) DeleteSession(_ context.Context, id string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE id = ?`, id)
	return err
}

// ── Agent KV ────────────────────────────────────────────────────────────────

func (s *SQLiteStore) SetKV(_ context.Context, agentID, key string, value json.RawMessage) error {
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO agent_kv (agent_id, key, value, updated_at) VALUES (?, ?, ?, ?)`,
		agentID, key, string(value), time.Now(),
	)
	return err
}

func (s *SQLiteStore) GetKV(_ context.Context, agentID, key string) (json.RawMessage, bool, error) {
	var val string
	err := s.db.QueryRow(`SELECT value FROM agent_kv WHERE agent_id = ? AND key = ?`, agentID, key).Scan(&val)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return json.RawMessage(val), true, nil
}

func (s *SQLiteStore) ListKV(_ context.Context, agentID string) (map[string]json.RawMessage, error) {
	rows, err := s.db.Query(`SELECT key, value FROM agent_kv WHERE agent_id = ?`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]json.RawMessage)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		result[k] = json.RawMessage(v)
	}
	return result, rows.Err()
}

func (s *SQLiteStore) DeleteKV(_ context.Context, agentID, key string) error {
	_, err := s.db.Exec(`DELETE FROM agent_kv WHERE agent_id = ? AND key = ?`, agentID, key)
	return err
}

// ── Policies ────────────────────────────────────────────────────────────────

func (s *SQLiteStore) SetPolicy(_ context.Context, agentID string, policy json.RawMessage) error {
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO policies (agent_id, policy, updated_at) VALUES (?, ?, ?)`,
		agentID, string(policy), time.Now(),
	)
	return err
}

func (s *SQLiteStore) GetPolicy(_ context.Context, agentID string) (json.RawMessage, bool, error) {
	var val string
	err := s.db.QueryRow(`SELECT policy FROM policies WHERE agent_id = ?`, agentID).Scan(&val)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return json.RawMessage(val), true, nil
}

func (s *SQLiteStore) DeletePolicy(_ context.Context, agentID string) error {
	_, err := s.db.Exec(`DELETE FROM policies WHERE agent_id = ?`, agentID)
	return err
}

// ── Close ───────────────────────────────────────────────────────────────────

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}
