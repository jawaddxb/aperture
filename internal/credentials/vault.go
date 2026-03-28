// Package credentials implements the encrypted credential vault for Aperture.
// Credentials are stored as JSON files with AES-256-GCM encrypted passwords.
package credentials

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ApertureHQ/aperture/internal/domain"
	"golang.org/x/crypto/hkdf"
)

// storedCredential is the on-disk format with encrypted password.
type storedCredential struct {
	AgentID          string            `json:"agent_id"`
	Domain           string            `json:"domain"`
	Username         string            `json:"username"`
	EncryptedPassword string           `json:"encrypted_password"`
	EncryptedTOTP    string            `json:"encrypted_totp,omitempty"`
	AutoLogin        bool              `json:"auto_login"`
	Metadata         map[string]string `json:"metadata,omitempty"`
}

// EncryptedFileVault stores credentials as encrypted JSON files.
// Directory layout: {baseDir}/{agentID}/{domain}.json
type EncryptedFileVault struct {
	baseDir string
	aead    cipher.AEAD
}

// NewEncryptedFileVault creates a vault backed by the filesystem.
// Encryption key is derived from APERTURE_VAULT_KEY env var, or a generated
// key stored at ~/.aperture/vault.key.
func NewEncryptedFileVault() (*EncryptedFileVault, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("get home dir: %w", err)
	}

	baseDir := filepath.Join(home, ".aperture", "credentials")
	if err := os.MkdirAll(baseDir, 0700); err != nil {
		return nil, fmt.Errorf("create credentials dir: %w", err)
	}

	key, err := deriveKey(home)
	if err != nil {
		return nil, fmt.Errorf("derive encryption key: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create AES cipher: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}

	return &EncryptedFileVault{baseDir: baseDir, aead: aead}, nil
}

// Store saves or updates a credential. The password is encrypted before writing.
func (v *EncryptedFileVault) Store(_ context.Context, cred domain.Credential) error {
	agentDir := filepath.Join(v.baseDir, sanitizeName(cred.AgentID))
	if err := os.MkdirAll(agentDir, 0700); err != nil {
		return fmt.Errorf("create agent dir: %w", err)
	}

	encPassword, err := v.encrypt([]byte(cred.Password))
	if err != nil {
		return fmt.Errorf("encrypt password: %w", err)
	}

	var encTOTP string
	if cred.TOTPSeed != "" {
		enc, err := v.encrypt([]byte(cred.TOTPSeed))
		if err != nil {
			return fmt.Errorf("encrypt TOTP: %w", err)
		}
		encTOTP = enc
	}

	sc := storedCredential{
		AgentID:           cred.AgentID,
		Domain:            cred.Domain,
		Username:          cred.Username,
		EncryptedPassword: encPassword,
		EncryptedTOTP:     encTOTP,
		AutoLogin:         cred.AutoLogin,
		Metadata:          cred.Metadata,
	}

	data, err := json.MarshalIndent(sc, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal credential: %w", err)
	}

	path := filepath.Join(agentDir, sanitizeName(cred.Domain)+".json")
	return os.WriteFile(path, data, 0600)
}

// Get retrieves and decrypts a credential. Returns nil if not found.
func (v *EncryptedFileVault) Get(_ context.Context, agentID, domainName string) (*domain.Credential, error) {
	// Try exact match first, then wildcard.
	sc, err := v.loadStored(agentID, domainName)
	if err != nil {
		return nil, err
	}
	if sc == nil {
		// Try wildcard matching: look through all credentials for this agent.
		sc, err = v.findByWildcard(agentID, domainName)
		if err != nil || sc == nil {
			return nil, err
		}
	}

	password, err := v.decrypt(sc.EncryptedPassword)
	if err != nil {
		return nil, fmt.Errorf("decrypt password: %w", err)
	}

	var totp string
	if sc.EncryptedTOTP != "" {
		t, err := v.decrypt(sc.EncryptedTOTP)
		if err != nil {
			return nil, fmt.Errorf("decrypt TOTP: %w", err)
		}
		totp = string(t)
	}

	return &domain.Credential{
		AgentID:   sc.AgentID,
		Domain:    sc.Domain,
		Username:  sc.Username,
		Password:  string(password),
		TOTPSeed:  totp,
		AutoLogin: sc.AutoLogin,
		Metadata:  sc.Metadata,
	}, nil
}

// List returns credential summaries for an agent (never includes passwords).
func (v *EncryptedFileVault) List(_ context.Context, agentID string) ([]domain.CredentialSummary, error) {
	agentDir := filepath.Join(v.baseDir, sanitizeName(agentID))
	entries, err := os.ReadDir(agentDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []domain.CredentialSummary{}, nil
		}
		return nil, fmt.Errorf("read agent dir: %w", err)
	}

	var summaries []domain.CredentialSummary
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(agentDir, entry.Name()))
		if err != nil {
			continue
		}
		var sc storedCredential
		if err := json.Unmarshal(data, &sc); err != nil {
			continue
		}
		summaries = append(summaries, domain.CredentialSummary{
			Domain:      sc.Domain,
			Username:    sc.Username,
			HasPassword: sc.EncryptedPassword != "",
			HasTOTP:     sc.EncryptedTOTP != "",
			AutoLogin:   sc.AutoLogin,
			Metadata:    sc.Metadata,
		})
	}

	if summaries == nil {
		summaries = []domain.CredentialSummary{}
	}
	return summaries, nil
}

// Delete removes a credential file.
func (v *EncryptedFileVault) Delete(_ context.Context, agentID, domainName string) error {
	path := filepath.Join(v.baseDir, sanitizeName(agentID), sanitizeName(domainName)+".json")
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// ─── encryption helpers ─────────────────────────────────────────────────────

func (v *EncryptedFileVault) encrypt(plaintext []byte) (string, error) {
	nonce := make([]byte, v.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := v.aead.Seal(nonce, nonce, plaintext, nil)
	return hex.EncodeToString(ciphertext), nil
}

func (v *EncryptedFileVault) decrypt(hexCiphertext string) ([]byte, error) {
	ciphertext, err := hex.DecodeString(hexCiphertext)
	if err != nil {
		return nil, fmt.Errorf("hex decode: %w", err)
	}
	if len(ciphertext) < v.aead.NonceSize() {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce, ct := ciphertext[:v.aead.NonceSize()], ciphertext[v.aead.NonceSize():]
	return v.aead.Open(nil, nonce, ct, nil)
}

// ─── internal helpers ───────────────────────────────────────────────────────

func (v *EncryptedFileVault) loadStored(agentID, domainName string) (*storedCredential, error) {
	path := filepath.Join(v.baseDir, sanitizeName(agentID), sanitizeName(domainName)+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var sc storedCredential
	if err := json.Unmarshal(data, &sc); err != nil {
		return nil, err
	}
	return &sc, nil
}

func (v *EncryptedFileVault) findByWildcard(agentID, domainName string) (*storedCredential, error) {
	agentDir := filepath.Join(v.baseDir, sanitizeName(agentID))
	entries, err := os.ReadDir(agentDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(agentDir, entry.Name()))
		if err != nil {
			continue
		}
		var sc storedCredential
		if err := json.Unmarshal(data, &sc); err != nil {
			continue
		}
		if matchWildcardDomain(sc.Domain, domainName) {
			return &sc, nil
		}
	}
	return nil, nil
}

// matchWildcardDomain checks if pattern (e.g. "*.amazon.com") matches domain (e.g. "www.amazon.com").
func matchWildcardDomain(pattern, domain string) bool {
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	domain = strings.ToLower(strings.TrimSpace(domain))

	if pattern == domain {
		return true
	}
	if strings.HasPrefix(pattern, "*.") {
		suffix := pattern[1:]
		if strings.HasSuffix(domain, suffix) {
			return true
		}
		if domain == pattern[2:] {
			return true
		}
	}
	return false
}

// deriveKey returns a 32-byte key from APERTURE_VAULT_KEY or a generated file key.
func deriveKey(homeDir string) ([]byte, error) {
	if envKey := os.Getenv("APERTURE_VAULT_KEY"); envKey != "" {
		return deriveFromPassphrase(envKey), nil
	}

	keyPath := filepath.Join(homeDir, ".aperture", "vault.key")
	if data, err := os.ReadFile(keyPath); err == nil && len(data) >= 32 {
		return data[:32], nil
	}

	// Generate new random key.
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(keyPath), 0700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(keyPath, key, 0600); err != nil {
		return nil, err
	}
	return key, nil
}

// deriveFromPassphrase uses HKDF-SHA256 to derive a 32-byte key from a passphrase.
func deriveFromPassphrase(passphrase string) []byte {
	hkdfReader := hkdf.New(sha256.New, []byte(passphrase), []byte("aperture-vault-salt"), []byte("aperture-vault-key"))
	key := make([]byte, 32)
	_, _ = io.ReadFull(hkdfReader, key)
	return key
}

// sanitizeName replaces filesystem-unsafe characters for use as directory/file names.
func sanitizeName(s string) string {
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		"*", "_star_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
	)
	return replacer.Replace(s)
}

// compile-time interface assertion.
var _ domain.CredentialVault = (*EncryptedFileVault)(nil)
