// Package domain defines core interfaces for Aperture.
// This file defines the CredentialVault interface and credential types.
package domain

import "context"

// Credential stores login details for one domain.
type Credential struct {
	AgentID   string            `json:"agent_id"`
	Domain    string            `json:"domain"`
	Username  string            `json:"username"`
	Password  string            `json:"password,omitempty"`
	TOTPSeed  string            `json:"totp_seed,omitempty"`
	AutoLogin bool              `json:"auto_login"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// CredentialVault manages encrypted per-agent, per-domain credentials.
type CredentialVault interface {
	// Store saves or updates a credential. Password is never stored in plaintext.
	Store(ctx context.Context, cred Credential) error

	// Get retrieves a credential for an agent+domain (decrypted for internal use only).
	// Returns nil if not found.
	Get(ctx context.Context, agentID, domain string) (*Credential, error)

	// List returns all credential domains for an agent (never returns passwords).
	List(ctx context.Context, agentID string) ([]CredentialSummary, error)

	// Delete removes a credential.
	Delete(ctx context.Context, agentID, domain string) error
}

// CredentialSummary is the safe-to-return version (no password/TOTP).
type CredentialSummary struct {
	Domain      string            `json:"domain"`
	Username    string            `json:"username"`
	HasPassword bool              `json:"has_password"`
	HasTOTP     bool              `json:"has_totp"`
	AutoLogin   bool              `json:"auto_login"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}
