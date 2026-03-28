// Package domain defines core interfaces for Aperture.
// This file defines the PolicyEngine interface and xBPP policy types.
package domain

import "context"

// PolicyResult is the outcome of an xBPP policy check.
type PolicyResult string

const (
	PolicyAllow    PolicyResult = "ALLOW"
	PolicyBlock    PolicyResult = "BLOCK"
	PolicyEscalate PolicyResult = "ESCALATE"
)

// PolicyDecision is returned from every policy evaluation.
type PolicyDecision struct {
	Result    PolicyResult `json:"result"`
	Reason    string       `json:"reason,omitempty"`
	CheckID   string       `json:"check_id,omitempty"`
	LatencyMs int64        `json:"latency_ms"`
}

// AgentPolicy defines the rules for a specific agent.
type AgentPolicy struct {
	AgentID                 string       `json:"agent_id"`
	DomainAllowlist         []string     `json:"domain_allowlist,omitempty"`
	DomainBlocklist         []string     `json:"domain_blocklist,omitempty"`
	ActionAllowlist         []string     `json:"action_allowlist,omitempty"`
	MaxActionsPerSession    int          `json:"max_actions_per_session,omitempty"`
	BudgetCredits           int          `json:"budget_credits,omitempty"`
	RateLimitPerMin         int          `json:"rate_limit_per_min,omitempty"`
	TransactionThresholdUSD float64      `json:"transaction_threshold_usd,omitempty"`
	EscalationWebhook       string       `json:"escalation_webhook,omitempty"`
	CustomRules             []PolicyRule `json:"custom_rules,omitempty"`

	// Check 8: if false, escalate on domains containing PII indicators.
	AllowPII bool `json:"allow_pii,omitempty"`

	// Check 9: patterns in extracted data that trigger BLOCK (e.g. "ssn", "credit_card").
	DataExfilPatterns []string `json:"data_exfil_patterns,omitempty"`

	// Check 11: if non-empty, action/domain must match a keyword to proceed.
	ScopeKeywords []string `json:"scope_keywords,omitempty"`

	// Check 12: 0 = no check; agent blocks exceeding this threshold → ESCALATE.
	MaxReputationScore int `json:"max_reputation_score,omitempty"`

	// Check 8 companion: if false, block financial transaction pages.
	AllowFinancial bool `json:"allow_financial,omitempty"`
}

// PolicyRule is an operator-defined custom rule.
type PolicyRule struct {
	Condition string       `json:"condition"`
	Result    PolicyResult `json:"result"`
	Message   string       `json:"message,omitempty"`
}

// PolicyEngine evaluates agent actions against policy.
type PolicyEngine interface {
	// Evaluate checks whether an action is allowed.
	// actionType: "click", "navigate", "type", etc.
	// domain: current page domain
	// Returns decision in <10ms (rule-based, no LLM).
	Evaluate(ctx context.Context, agentID, actionType, domain string) PolicyDecision

	// GetPolicy returns the policy for an agent (nil = default/open policy).
	GetPolicy(agentID string) *AgentPolicy

	// SetPolicy stores or updates a policy for an agent.
	SetPolicy(agentID string, policy AgentPolicy) error

	// DeletePolicy removes a policy (agent reverts to default/open).
	DeletePolicy(agentID string) error
}
