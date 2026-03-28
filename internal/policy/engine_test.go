package policy

import (
	"context"
	"testing"
	"time"

	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNoPolicy_Allow(t *testing.T) {
	e := NewInMemoryPolicyEngine()
	d := e.Evaluate(context.Background(), "agent-1", "click", "example.com")
	assert.Equal(t, domain.PolicyAllow, d.Result)
}

func TestDomainBlocklist_Block(t *testing.T) {
	e := NewInMemoryPolicyEngine()
	require.NoError(t, e.SetPolicy("agent-1", domain.AgentPolicy{
		DomainBlocklist: []string{"*.evil.com", "bad.org"},
	}))

	d := e.Evaluate(context.Background(), "agent-1", "navigate", "www.evil.com")
	assert.Equal(t, domain.PolicyBlock, d.Result)
	assert.Equal(t, "domain_blocklist", d.CheckID)

	d = e.Evaluate(context.Background(), "agent-1", "navigate", "bad.org")
	assert.Equal(t, domain.PolicyBlock, d.Result)
}

func TestDomainAllowlist_Miss_Block(t *testing.T) {
	e := NewInMemoryPolicyEngine()
	require.NoError(t, e.SetPolicy("agent-1", domain.AgentPolicy{
		DomainAllowlist: []string{"*.google.com"},
	}))

	d := e.Evaluate(context.Background(), "agent-1", "navigate", "evil.com")
	assert.Equal(t, domain.PolicyBlock, d.Result)
	assert.Equal(t, "domain_allowlist", d.CheckID)
}

func TestDomainAllowlist_Hit_Allow(t *testing.T) {
	e := NewInMemoryPolicyEngine()
	require.NoError(t, e.SetPolicy("agent-1", domain.AgentPolicy{
		DomainAllowlist: []string{"*.google.com"},
	}))

	d := e.Evaluate(context.Background(), "agent-1", "navigate", "www.google.com")
	assert.Equal(t, domain.PolicyAllow, d.Result)
}

func TestActionAllowlist_Block(t *testing.T) {
	e := NewInMemoryPolicyEngine()
	require.NoError(t, e.SetPolicy("agent-1", domain.AgentPolicy{
		ActionAllowlist: []string{"navigate", "extract"},
	}))

	d := e.Evaluate(context.Background(), "agent-1", "click", "example.com")
	assert.Equal(t, domain.PolicyBlock, d.Result)
	assert.Equal(t, "action_allowlist", d.CheckID)
}

func TestRateLimit_Block(t *testing.T) {
	e := NewInMemoryPolicyEngine()
	require.NoError(t, e.SetPolicy("agent-1", domain.AgentPolicy{
		RateLimitPerMin: 3,
	}))

	ctx := context.Background()
	// First 3 should pass.
	for i := 0; i < 3; i++ {
		d := e.Evaluate(ctx, "agent-1", "click", "example.com")
		assert.Equal(t, domain.PolicyAllow, d.Result, "request %d should be allowed", i)
	}
	// 4th should be blocked.
	d := e.Evaluate(ctx, "agent-1", "click", "example.com")
	assert.Equal(t, domain.PolicyBlock, d.Result)
	assert.Equal(t, "rate_limit", d.CheckID)
}

func TestMaxActions_Block(t *testing.T) {
	e := NewInMemoryPolicyEngine()
	require.NoError(t, e.SetPolicy("agent-1", domain.AgentPolicy{
		MaxActionsPerSession: 2,
	}))

	ctx := context.Background()
	d := e.Evaluate(ctx, "agent-1", "click", "example.com")
	assert.Equal(t, domain.PolicyAllow, d.Result)
	d = e.Evaluate(ctx, "agent-1", "click", "example.com")
	assert.Equal(t, domain.PolicyAllow, d.Result)
	d = e.Evaluate(ctx, "agent-1", "click", "example.com")
	assert.Equal(t, domain.PolicyBlock, d.Result)
	assert.Equal(t, "max_actions", d.CheckID)
}

func TestCustomRule_Match(t *testing.T) {
	e := NewInMemoryPolicyEngine()
	require.NoError(t, e.SetPolicy("agent-1", domain.AgentPolicy{
		CustomRules: []domain.PolicyRule{
			{
				Condition: "domain == secret.com AND action == click",
				Result:    domain.PolicyBlock,
				Message:   "no clicking on secret.com",
			},
		},
	}))

	d := e.Evaluate(context.Background(), "agent-1", "click", "secret.com")
	assert.Equal(t, domain.PolicyBlock, d.Result)
	assert.Equal(t, "custom_rule", d.CheckID)
	assert.Equal(t, "no clicking on secret.com", d.Reason)

	// Different action should be allowed.
	d = e.Evaluate(context.Background(), "agent-1", "navigate", "secret.com")
	assert.Equal(t, domain.PolicyAllow, d.Result)
}

func TestTransactionThreshold_Escalate(t *testing.T) {
	e := NewInMemoryPolicyEngine()
	require.NoError(t, e.SetPolicy("agent-1", domain.AgentPolicy{
		TransactionThresholdUSD: 100.0,
	}))

	d := e.Evaluate(context.Background(), "agent-1", "click", "my-bank.com")
	assert.Equal(t, domain.PolicyEscalate, d.Result)
	assert.Equal(t, "transaction_threshold", d.CheckID)

	// Non-click should not escalate.
	d = e.Evaluate(context.Background(), "agent-1", "navigate", "my-bank.com")
	assert.Equal(t, domain.PolicyAllow, d.Result)
}

func TestLatency_Under10ms(t *testing.T) {
	e := NewInMemoryPolicyEngine()
	require.NoError(t, e.SetPolicy("agent-1", domain.AgentPolicy{
		DomainBlocklist: []string{"*.blocked.com"},
		DomainAllowlist: []string{"*.allowed.com"},
		ActionAllowlist: []string{"navigate", "click"},
		CustomRules: []domain.PolicyRule{
			{Condition: "domain == test.com", Result: domain.PolicyBlock, Message: "test"},
		},
	}))

	ctx := context.Background()
	start := time.Now()
	for i := 0; i < 100; i++ {
		e.Evaluate(ctx, "agent-1", "click", "www.allowed.com")
	}
	elapsed := time.Since(start)
	avgMs := float64(elapsed.Milliseconds()) / 100.0
	assert.Less(t, avgMs, 10.0, "average evaluation latency should be under 10ms, got %.2fms", avgMs)
}

func TestDeletePolicy_RevertsToOpen(t *testing.T) {
	e := NewInMemoryPolicyEngine()
	require.NoError(t, e.SetPolicy("agent-1", domain.AgentPolicy{
		DomainBlocklist: []string{"evil.com"},
	}))

	d := e.Evaluate(context.Background(), "agent-1", "navigate", "evil.com")
	assert.Equal(t, domain.PolicyBlock, d.Result)

	require.NoError(t, e.DeletePolicy("agent-1"))

	d = e.Evaluate(context.Background(), "agent-1", "navigate", "evil.com")
	assert.Equal(t, domain.PolicyAllow, d.Result)
}

func TestGetPolicy_NilWhenNotSet(t *testing.T) {
	e := NewInMemoryPolicyEngine()
	assert.Nil(t, e.GetPolicy("nonexistent"))
}

func TestSetGetPolicy_Roundtrip(t *testing.T) {
	e := NewInMemoryPolicyEngine()
	pol := domain.AgentPolicy{
		DomainAllowlist: []string{"*.example.com"},
		RateLimitPerMin: 60,
	}
	require.NoError(t, e.SetPolicy("agent-1", pol))

	got := e.GetPolicy("agent-1")
	require.NotNil(t, got)
	assert.Equal(t, "agent-1", got.AgentID)
	assert.Equal(t, []string{"*.example.com"}, got.DomainAllowlist)
	assert.Equal(t, 60, got.RateLimitPerMin)
}

// ─── xBPP Checks 8–12 ──────────────────────────────────────────────────────────

func TestCheck8_PIIDomainEscalation(t *testing.T) {
	e := NewInMemoryPolicyEngine()
	require.NoError(t, e.SetPolicy("agent-1", domain.AgentPolicy{
		AllowPII: false, // default: block PII domains
	}))

	// Non-navigate action on a medical domain → ESCALATE
	d := e.Evaluate(context.Background(), "agent-1", "click", "portal.health.gov")
	assert.Equal(t, domain.PolicyEscalate, d.Result)
	assert.Equal(t, "check_8_content", d.CheckID)

	// Navigate is exempted from check 8
	d = e.Evaluate(context.Background(), "agent-1", "navigate", "portal.health.gov")
	assert.Equal(t, domain.PolicyAllow, d.Result)

	// AllowPII = true → no escalation
	require.NoError(t, e.SetPolicy("agent-2", domain.AgentPolicy{AllowPII: true}))
	d = e.Evaluate(context.Background(), "agent-2", "click", "portal.health.gov")
	assert.Equal(t, domain.PolicyAllow, d.Result)
}

func TestCheck9_DataExfiltrationBlock(t *testing.T) {
	e := NewInMemoryPolicyEngine()
	require.NoError(t, e.SetPolicy("agent-1", domain.AgentPolicy{
		AllowPII:          true, // bypass check 8 so check 9 fires
		DataExfilPatterns: []string{"secret", "ssn"},
	}))

	// Extract on a domain matching an exfil pattern → BLOCK
	d := e.Evaluate(context.Background(), "agent-1", "extract", "secret-vault.com")
	assert.Equal(t, domain.PolicyBlock, d.Result)
	assert.Equal(t, "check_9_exfil", d.CheckID)

	// Non-extract action → ALLOW
	d = e.Evaluate(context.Background(), "agent-1", "click", "secret-vault.com")
	assert.Equal(t, domain.PolicyAllow, d.Result)

	// Extract on non-matching domain → ALLOW
	d = e.Evaluate(context.Background(), "agent-1", "extract", "example.com")
	assert.Equal(t, domain.PolicyAllow, d.Result)
}

func TestCheck11_ScopeCheck(t *testing.T) {
	e := NewInMemoryPolicyEngine()
	require.NoError(t, e.SetPolicy("agent-1", domain.AgentPolicy{
		ScopeKeywords: []string{"shopping", "amazon"},
	}))

	// Domain contains "amazon" → ALLOW
	d := e.Evaluate(context.Background(), "agent-1", "click", "www.amazon.com")
	assert.Equal(t, domain.PolicyAllow, d.Result)

	// Domain not in scope → BLOCK
	d = e.Evaluate(context.Background(), "agent-1", "click", "evil.com")
	assert.Equal(t, domain.PolicyBlock, d.Result)
	assert.Equal(t, "check_11_scope", d.CheckID)
}

func TestCheck12_ReputationEscalation(t *testing.T) {
	e := NewInMemoryPolicyEngine()
	require.NoError(t, e.SetPolicy("agent-1", domain.AgentPolicy{
		DomainBlocklist:    []string{"blocked.com"},
		MaxReputationScore: 3,
	}))

	ctx := context.Background()
	// Accumulate 3 blocks via domain blocklist
	for i := 0; i < 3; i++ {
		d := e.Evaluate(ctx, "agent-1", "navigate", "blocked.com")
		assert.Equal(t, domain.PolicyBlock, d.Result)
	}

	// Now any action should trigger reputation escalation (check 12 runs even on allowed domains)
	d := e.Evaluate(ctx, "agent-1", "click", "safe.com")
	assert.Equal(t, domain.PolicyEscalate, d.Result)
	assert.Equal(t, "check_12_reputation", d.CheckID)
}

func TestCheck8_PIIBankDomain(t *testing.T) {
	e := NewInMemoryPolicyEngine()
	require.NoError(t, e.SetPolicy("agent-1", domain.AgentPolicy{
		AllowPII: false,
	}))

	// "bank" in domain should trigger PII escalation for non-navigate
	d := e.Evaluate(context.Background(), "agent-1", "type", "www.mybank.com")
	assert.Equal(t, domain.PolicyEscalate, d.Result)
	assert.Equal(t, "check_8_content", d.CheckID)
	assert.Contains(t, d.Reason, "pii_domain_escalation")
}
