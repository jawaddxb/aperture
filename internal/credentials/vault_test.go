package credentials

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testVault creates a vault in a temp directory for testing.
func testVault(t *testing.T) *EncryptedFileVault {
	t.Helper()
	tmpDir := t.TempDir()

	key := deriveFromPassphrase("test-vault-key-for-testing-only")
	block, err := aes.NewCipher(key)
	require.NoError(t, err)
	aead, err := cipher.NewGCM(block)
	require.NoError(t, err)

	return &EncryptedFileVault{baseDir: tmpDir, aead: aead}
}

func TestStoreAndGet_Roundtrip(t *testing.T) {
	vault := testVault(t)
	ctx := context.Background()

	cred := domain.Credential{
		AgentID:   "agent-1",
		Domain:    "example.com",
		Username:  "user@test.com",
		Password:  "s3cret!pass",
		TOTPSeed:  "JBSWY3DPEHPK3PXP",
		AutoLogin: true,
		Metadata:  map[string]string{"env": "test"},
	}

	require.NoError(t, vault.Store(ctx, cred))

	got, err := vault.Get(ctx, "agent-1", "example.com")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "user@test.com", got.Username)
	assert.Equal(t, "s3cret!pass", got.Password)
	assert.Equal(t, "JBSWY3DPEHPK3PXP", got.TOTPSeed)
	assert.True(t, got.AutoLogin)
	assert.Equal(t, "test", got.Metadata["env"])
}

func TestPasswordEncryptedAtRest(t *testing.T) {
	vault := testVault(t)
	ctx := context.Background()

	cred := domain.Credential{
		AgentID:  "agent-1",
		Domain:   "secret.com",
		Username: "admin",
		Password: "plaintext_password_123",
	}
	require.NoError(t, vault.Store(ctx, cred))

	// Read the raw file and ensure plaintext password is not present.
	rawPath := filepath.Join(vault.baseDir, sanitizeName("agent-1"), sanitizeName("secret.com")+".json")
	rawData, err := os.ReadFile(rawPath)
	require.NoError(t, err)

	assert.NotContains(t, string(rawData), "plaintext_password_123")

	// Verify it's valid JSON with encrypted_password field.
	var stored map[string]interface{}
	require.NoError(t, json.Unmarshal(rawData, &stored))
	assert.NotEmpty(t, stored["encrypted_password"])
}

func TestListReturnsSummaries(t *testing.T) {
	vault := testVault(t)
	ctx := context.Background()

	require.NoError(t, vault.Store(ctx, domain.Credential{
		AgentID: "agent-1", Domain: "site-a.com", Username: "user-a", Password: "pass-a",
	}))
	require.NoError(t, vault.Store(ctx, domain.Credential{
		AgentID: "agent-1", Domain: "site-b.com", Username: "user-b", Password: "pass-b", TOTPSeed: "seed",
	}))

	summaries, err := vault.List(ctx, "agent-1")
	require.NoError(t, err)
	assert.Len(t, summaries, 2)

	for _, s := range summaries {
		assert.True(t, s.HasPassword)
		// Ensure no password data leaked.
		data, _ := json.Marshal(s)
		assert.NotContains(t, string(data), "pass-")
	}
}

func TestDelete(t *testing.T) {
	vault := testVault(t)
	ctx := context.Background()

	require.NoError(t, vault.Store(ctx, domain.Credential{
		AgentID: "agent-1", Domain: "delete-me.com", Username: "u", Password: "p",
	}))

	got, err := vault.Get(ctx, "agent-1", "delete-me.com")
	require.NoError(t, err)
	require.NotNil(t, got)

	require.NoError(t, vault.Delete(ctx, "agent-1", "delete-me.com"))

	got, err = vault.Get(ctx, "agent-1", "delete-me.com")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestGetNonExistent(t *testing.T) {
	vault := testVault(t)
	ctx := context.Background()

	got, err := vault.Get(ctx, "no-agent", "no-domain.com")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestWildcardDomainMatching(t *testing.T) {
	vault := testVault(t)
	ctx := context.Background()

	require.NoError(t, vault.Store(ctx, domain.Credential{
		AgentID:   "agent-1",
		Domain:    "*.amazon.com",
		Username:  "shopper",
		Password:  "amazon-pass",
		AutoLogin: true,
	}))

	// Should match via wildcard.
	got, err := vault.Get(ctx, "agent-1", "www.amazon.com")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "shopper", got.Username)
	assert.Equal(t, "amazon-pass", got.Password)
}

func TestListEmptyAgent(t *testing.T) {
	vault := testVault(t)
	ctx := context.Background()

	summaries, err := vault.List(ctx, "nobody")
	require.NoError(t, err)
	assert.Empty(t, summaries)
}
