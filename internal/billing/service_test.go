package billing

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestDB(t *testing.T) *AccountService {
	t.Helper()
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return NewAccountService(db)
}

func TestCreateAccount(t *testing.T) {
	svc := setupTestDB(t)

	acct, key, err := svc.CreateAccount("Acme Corp", "admin@acme.com", "starter", 5000)
	require.NoError(t, err)

	assert.NotEmpty(t, acct.ID)
	assert.True(t, len(acct.ID) > 5 && acct.ID[:5] == "acct_")
	assert.Equal(t, "Acme Corp", acct.Name)
	assert.Equal(t, "admin@acme.com", acct.Email)
	assert.Equal(t, "starter", acct.Plan)
	assert.Equal(t, 5000, acct.CreditBalance)

	assert.NotEmpty(t, key)
	assert.True(t, len(key) > 4 && key[:4] == "apt_")

	// Verify account is fetchable.
	fetched, err := svc.GetAccount(acct.ID)
	require.NoError(t, err)
	assert.Equal(t, acct.ID, fetched.ID)
	assert.Equal(t, 5000, fetched.CreditBalance)

	// Verify HasAccounts is true.
	assert.True(t, svc.HasAccounts())
}

func TestCreateAccountNoInitialCredits(t *testing.T) {
	svc := setupTestDB(t)

	acct, _, err := svc.CreateAccount("Zero Corp", "", "free", 0)
	require.NoError(t, err)
	assert.Equal(t, 0, acct.CreditBalance)
}

func TestHasAccountsEmpty(t *testing.T) {
	svc := setupTestDB(t)
	assert.False(t, svc.HasAccounts())
}

func TestAddCredits(t *testing.T) {
	svc := setupTestDB(t)

	acct, _, err := svc.CreateAccount("Test", "", "starter", 1000)
	require.NoError(t, err)

	newBalance, err := svc.AddCredits(acct.ID, 500, "manual top-up")
	require.NoError(t, err)
	assert.Equal(t, 1500, newBalance)

	// Verify balance persisted.
	fetched, err := svc.GetAccount(acct.ID)
	require.NoError(t, err)
	assert.Equal(t, 1500, fetched.CreditBalance)
}

func TestDeductCredits(t *testing.T) {
	svc := setupTestDB(t)

	acct, _, err := svc.CreateAccount("Test", "", "starter", 1000)
	require.NoError(t, err)

	newBalance, err := svc.DeductCredits(acct.ID, 300, "session_execute", "sess-123")
	require.NoError(t, err)
	assert.Equal(t, 700, newBalance)

	// Verify balance persisted.
	fetched, err := svc.GetAccount(acct.ID)
	require.NoError(t, err)
	assert.Equal(t, 700, fetched.CreditBalance)
}

func TestDeductCreditsInsufficientBalance(t *testing.T) {
	svc := setupTestDB(t)

	acct, _, err := svc.CreateAccount("Test", "", "starter", 100)
	require.NoError(t, err)

	balance, err := svc.DeductCredits(acct.ID, 500, "session_execute", "sess-456")
	assert.ErrorIs(t, err, ErrInsufficientCredits)
	assert.Equal(t, 100, balance) // Balance unchanged.

	// Verify balance still 100.
	fetched, err := svc.GetAccount(acct.ID)
	require.NoError(t, err)
	assert.Equal(t, 100, fetched.CreditBalance)
}

func TestLedgerEntries(t *testing.T) {
	svc := setupTestDB(t)

	acct, _, err := svc.CreateAccount("Test", "", "starter", 1000)
	require.NoError(t, err)

	_, err = svc.AddCredits(acct.ID, 200, "bonus")
	require.NoError(t, err)
	_, err = svc.DeductCredits(acct.ID, 50, "session_execute", "sess-1")
	require.NoError(t, err)

	entries, err := svc.GetLedger(acct.ID, 0, 10)
	require.NoError(t, err)
	// Should have 3 entries: initial_credit, bonus, session_execute
	assert.Len(t, entries, 3)
	// Most recent first.
	assert.Equal(t, -50, entries[0].Amount)
	assert.Equal(t, 1150, entries[0].BalanceAfter)
	assert.Equal(t, 200, entries[1].Amount)
	assert.Equal(t, 1200, entries[1].BalanceAfter)
}

func TestResolveAPIKey(t *testing.T) {
	svc := setupTestDB(t)

	acct, key, err := svc.CreateAccount("Test", "", "starter", 1000)
	require.NoError(t, err)

	resolved, isAdmin, err := svc.ResolveAPIKey(key)
	require.NoError(t, err)
	assert.True(t, isAdmin) // First key is always admin.
	assert.Equal(t, acct.ID, resolved.ID)

	// Invalid key.
	_, _, err = svc.ResolveAPIKey("apt_nonexistent")
	assert.Error(t, err)
}

func TestRevokeAPIKey(t *testing.T) {
	svc := setupTestDB(t)

	acct, key, err := svc.CreateAccount("Test", "", "starter", 1000)
	require.NoError(t, err)

	err = svc.RevokeAPIKey(key)
	require.NoError(t, err)

	// Revoked key should not resolve.
	_, _, err = svc.ResolveAPIKey(key)
	assert.Error(t, err)

	// Create a new key to verify the account still works.
	newKey, err := svc.CreateAPIKey(acct.ID, "backup", false)
	require.NoError(t, err)

	resolved, isAdmin, err := svc.ResolveAPIKey(newKey)
	require.NoError(t, err)
	assert.False(t, isAdmin)
	assert.Equal(t, acct.ID, resolved.ID)
}

func TestListAccounts(t *testing.T) {
	svc := setupTestDB(t)

	for i := 0; i < 5; i++ {
		_, _, err := svc.CreateAccount("Acct"+string(rune('A'+i)), "", "free", 0)
		require.NoError(t, err)
	}

	accounts, total, err := svc.ListAccounts(0, 3)
	require.NoError(t, err)
	assert.Equal(t, 5, total)
	assert.Len(t, accounts, 3)

	// Page 2.
	accounts2, total2, err := svc.ListAccounts(3, 3)
	require.NoError(t, err)
	assert.Equal(t, 5, total2)
	assert.Len(t, accounts2, 2)
}

func TestGetStats(t *testing.T) {
	svc := setupTestDB(t)

	acct, _, err := svc.CreateAccount("Test", "", "starter", 1000)
	require.NoError(t, err)

	_, err = svc.DeductCredits(acct.ID, 50, "test", "sess-1")
	require.NoError(t, err)

	stats, err := svc.GetStats()
	require.NoError(t, err)
	assert.Equal(t, 1, stats.TotalAccounts)
	assert.Equal(t, 1, stats.ActiveAccounts)
	assert.Equal(t, 50, stats.TotalCreditsUsed)
	assert.Equal(t, 1, stats.TotalSessions)
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
