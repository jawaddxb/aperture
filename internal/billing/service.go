package billing

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// ErrInsufficientCredits is returned when a deduction exceeds the account balance.
var ErrInsufficientCredits = errors.New("insufficient credits")

// Account represents a billing account.
type Account struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Email         string    `json:"email"`
	Plan          string    `json:"plan"`
	CreditBalance int       `json:"credit_balance"`
	MonthlyLimit  int       `json:"monthly_limit"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// APIKey represents a stored API key.
type APIKey struct {
	Key       string     `json:"key"`
	AccountID string     `json:"account_id"`
	Label     string     `json:"label"`
	IsAdmin   bool       `json:"is_admin"`
	CreatedAt time.Time  `json:"created_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

// LedgerEntry is a single credit transaction.
type LedgerEntry struct {
	ID           int       `json:"id"`
	AccountID    string    `json:"account_id"`
	Amount       int       `json:"amount"`
	BalanceAfter int       `json:"balance_after"`
	Reason       string    `json:"reason"`
	SessionID    string    `json:"session_id,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// UsageEntry groups credit usage by action type.
type UsageEntry struct {
	Action    string `json:"action"`
	Count     int    `json:"count"`
	TotalCost int    `json:"total_cost"`
}

// Stats holds aggregate billing stats.
type Stats struct {
	TotalAccounts    int `json:"total_accounts"`
	ActiveAccounts   int `json:"active_accounts"`
	TotalCreditsUsed int `json:"total_credits_used"`
	TotalSessions    int `json:"total_sessions"`
}

// AccountService provides account and credit operations backed by SQLite.
type AccountService struct {
	db *sql.DB
}

// NewAccountService creates a new AccountService.
func NewAccountService(db *sql.DB) *AccountService {
	return &AccountService{db: db}
}

// HasAccounts returns true if at least one account exists in the database.
// Used to determine whether billing mode is active.
func (s *AccountService) HasAccounts() bool {
	var count int
	_ = s.db.QueryRow("SELECT COUNT(*) FROM accounts").Scan(&count)
	return count > 0
}

// CreateAccount creates a new account with an initial API key and credit balance.
// The first key is admin ONLY if no other accounts exist (bootstrap). Otherwise it's a regular key.
func (s *AccountService) CreateAccount(name, email, plan string, initialCredits int) (*Account, string, error) {
	// Clamp credits to non-negative.
	if initialCredits < 0 {
		initialCredits = 0
	}

	id := "acct_" + randomHex(12)
	key := "apt_" + randomHex(32)
	now := time.Now()

	tx, err := s.db.Begin()
	if err != nil {
		return nil, "", fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.Exec(
		`INSERT INTO accounts (id, name, email, plan, credit_balance, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, name, email, plan, initialCredits, now, now,
	)
	if err != nil {
		return nil, "", fmt.Errorf("insert account: %w", err)
	}

	// First account ever = bootstrap admin. Subsequent accounts get regular keys.
	isFirstAccount := !s.HasAccounts()
	_, err = tx.Exec(
		`INSERT INTO api_keys (key, account_id, label, is_admin, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		key, id, "default", isFirstAccount, now,
	)
	if err != nil {
		return nil, "", fmt.Errorf("insert api key: %w", err)
	}

	if initialCredits > 0 {
		_, err = tx.Exec(
			`INSERT INTO ledger (account_id, amount, balance_after, reason, created_at)
			 VALUES (?, ?, ?, ?, ?)`,
			id, initialCredits, initialCredits, "initial_credit", now,
		)
		if err != nil {
			return nil, "", fmt.Errorf("insert ledger: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, "", fmt.Errorf("commit: %w", err)
	}

	acct := &Account{
		ID:            id,
		Name:          name,
		Email:         email,
		Plan:          plan,
		CreditBalance: initialCredits,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	return acct, key, nil
}

// GetAccount retrieves an account by ID.
func (s *AccountService) GetAccount(id string) (*Account, error) {
	a := &Account{}
	err := s.db.QueryRow(
		`SELECT id, name, email, plan, credit_balance, monthly_limit, created_at, updated_at
		 FROM accounts WHERE id = ?`, id,
	).Scan(&a.ID, &a.Name, &a.Email, &a.Plan, &a.CreditBalance, &a.MonthlyLimit, &a.CreatedAt, &a.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("account not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get account: %w", err)
	}
	return a, nil
}

// ListAccounts returns a paginated list of accounts.
func (s *AccountService) ListAccounts(offset, limit int) ([]Account, int, error) {
	var total int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM accounts").Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count accounts: %w", err)
	}

	rows, err := s.db.Query(
		`SELECT id, name, email, plan, credit_balance, monthly_limit, created_at, updated_at
		 FROM accounts ORDER BY created_at DESC LIMIT ? OFFSET ?`, limit, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("list accounts: %w", err)
	}
	defer rows.Close()

	var accounts []Account
	for rows.Next() {
		var a Account
		if err := rows.Scan(&a.ID, &a.Name, &a.Email, &a.Plan, &a.CreditBalance, &a.MonthlyLimit, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan account: %w", err)
		}
		accounts = append(accounts, a)
	}
	return accounts, total, nil
}

// AddCredits adds credits to an account and records a ledger entry.
func (s *AccountService) AddCredits(accountID string, amount int, reason string) (int, error) {
	if amount <= 0 {
		return 0, fmt.Errorf("amount must be positive (got %d)", amount)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var balance int
	err = tx.QueryRow("SELECT credit_balance FROM accounts WHERE id = ?", accountID).Scan(&balance)
	if err != nil {
		return 0, fmt.Errorf("get balance: %w", err)
	}

	newBalance := balance + amount
	_, err = tx.Exec("UPDATE accounts SET credit_balance = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", newBalance, accountID)
	if err != nil {
		return 0, fmt.Errorf("update balance: %w", err)
	}

	_, err = tx.Exec(
		`INSERT INTO ledger (account_id, amount, balance_after, reason, created_at)
		 VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		accountID, amount, newBalance, reason,
	)
	if err != nil {
		return 0, fmt.Errorf("insert ledger: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return newBalance, nil
}

// DeductCredits atomically deducts credits. Returns ErrInsufficientCredits if balance is too low.
func (s *AccountService) DeductCredits(accountID string, amount int, reason, sessionID string) (int, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var balance int
	err = tx.QueryRow("SELECT credit_balance FROM accounts WHERE id = ?", accountID).Scan(&balance)
	if err != nil {
		return 0, fmt.Errorf("get balance: %w", err)
	}

	if balance < amount {
		return balance, ErrInsufficientCredits
	}

	newBalance := balance - amount
	_, err = tx.Exec("UPDATE accounts SET credit_balance = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", newBalance, accountID)
	if err != nil {
		return 0, fmt.Errorf("update balance: %w", err)
	}

	_, err = tx.Exec(
		`INSERT INTO ledger (account_id, amount, balance_after, reason, session_id, created_at)
		 VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		accountID, -amount, newBalance, reason, sessionID,
	)
	if err != nil {
		return 0, fmt.Errorf("insert ledger: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return newBalance, nil
}

// GetUsage returns aggregated credit usage for an account in a time range.
func (s *AccountService) GetUsage(accountID, from, to string) ([]UsageEntry, error) {
	rows, err := s.db.Query(
		`SELECT reason, COUNT(*) as cnt, SUM(ABS(amount)) as total
		 FROM ledger
		 WHERE account_id = ? AND amount < 0 AND created_at >= ? AND created_at <= ?
		 GROUP BY reason ORDER BY total DESC`,
		accountID, from, to,
	)
	if err != nil {
		return nil, fmt.Errorf("query usage: %w", err)
	}
	defer rows.Close()

	var entries []UsageEntry
	for rows.Next() {
		var e UsageEntry
		if err := rows.Scan(&e.Action, &e.Count, &e.TotalCost); err != nil {
			return nil, fmt.Errorf("scan usage: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// GetLedger returns paginated ledger entries for an account.
func (s *AccountService) GetLedger(accountID string, offset, limit int) ([]LedgerEntry, error) {
	rows, err := s.db.Query(
		`SELECT id, account_id, amount, balance_after, reason, session_id, created_at
		 FROM ledger WHERE account_id = ? ORDER BY id DESC LIMIT ? OFFSET ?`,
		accountID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("query ledger: %w", err)
	}
	defer rows.Close()

	var entries []LedgerEntry
	for rows.Next() {
		var e LedgerEntry
		if err := rows.Scan(&e.ID, &e.AccountID, &e.Amount, &e.BalanceAfter, &e.Reason, &e.SessionID, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan ledger: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// CreateAPIKey generates a new API key for the given account.
func (s *AccountService) CreateAPIKey(accountID, label string, isAdmin bool) (string, error) {
	key := "apt_" + randomHex(32)
	_, err := s.db.Exec(
		`INSERT INTO api_keys (key, account_id, label, is_admin, created_at)
		 VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		key, accountID, label, isAdmin,
	)
	if err != nil {
		return "", fmt.Errorf("insert api key: %w", err)
	}
	return key, nil
}

// RevokeAPIKey marks an API key as revoked.
func (s *AccountService) RevokeAPIKey(key string) error {
	res, err := s.db.Exec(
		"UPDATE api_keys SET revoked_at = CURRENT_TIMESTAMP WHERE key = ? AND revoked_at IS NULL",
		key,
	)
	if err != nil {
		return fmt.Errorf("revoke key: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("key not found or already revoked")
	}
	return nil
}

// ResolveAPIKey looks up an API key and returns the associated account and admin status.
func (s *AccountService) ResolveAPIKey(key string) (*Account, bool, error) {
	var accountID string
	var isAdmin bool
	err := s.db.QueryRow(
		`SELECT account_id, is_admin FROM api_keys WHERE key = ? AND revoked_at IS NULL`,
		key,
	).Scan(&accountID, &isAdmin)
	if err == sql.ErrNoRows {
		return nil, false, fmt.Errorf("invalid api key")
	}
	if err != nil {
		return nil, false, fmt.Errorf("resolve key: %w", err)
	}

	acct, err := s.GetAccount(accountID)
	if err != nil {
		return nil, false, err
	}
	return acct, isAdmin, nil
}

// GetAPIKeys returns all API keys for an account (active and revoked).
func (s *AccountService) GetAPIKeys(accountID string) ([]APIKey, error) {
	rows, err := s.db.Query(
		`SELECT key, account_id, label, is_admin, created_at, revoked_at
		 FROM api_keys WHERE account_id = ? ORDER BY created_at DESC`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("query keys: %w", err)
	}
	defer rows.Close()

	var keys []APIKey
	for rows.Next() {
		var k APIKey
		if err := rows.Scan(&k.Key, &k.AccountID, &k.Label, &k.IsAdmin, &k.CreatedAt, &k.RevokedAt); err != nil {
			return nil, fmt.Errorf("scan key: %w", err)
		}
		keys = append(keys, k)
	}
	return keys, nil
}

// GetStats returns aggregate billing statistics.
func (s *AccountService) GetStats() (*Stats, error) {
	st := &Stats{}
	_ = s.db.QueryRow("SELECT COUNT(*) FROM accounts").Scan(&st.TotalAccounts)
	_ = s.db.QueryRow("SELECT COUNT(DISTINCT account_id) FROM ledger WHERE amount < 0").Scan(&st.ActiveAccounts)
	_ = s.db.QueryRow("SELECT COALESCE(SUM(ABS(amount)), 0) FROM ledger WHERE amount < 0").Scan(&st.TotalCreditsUsed)
	_ = s.db.QueryRow("SELECT COUNT(DISTINCT session_id) FROM ledger WHERE session_id != ''").Scan(&st.TotalSessions)
	return st, nil
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
