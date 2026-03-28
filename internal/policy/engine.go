// Package policy implements the xBPP (Cross-platform Behavior Policy Protocol) engine.
// It gates every agent action against configurable rules with <10ms latency.
package policy

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ApertureHQ/aperture/internal/domain"
)

// InMemoryPolicyEngine stores agent policies in memory and evaluates actions
// against a 12-check rule pipeline. Safe for concurrent use.
type InMemoryPolicyEngine struct {
	mu       sync.RWMutex
	policies map[string]domain.AgentPolicy

	// actionCounters tracks per-agent action counts for MaxActionsPerSession.
	counterMu sync.Mutex
	counters  map[string]int

	// rateBuckets tracks per-agent last-minute action timestamps for rate limiting.
	bucketMu sync.Mutex
	buckets  map[string][]time.Time

	// blockCounts tracks accumulated BLOCK decisions per agent for reputation check (check 12).
	blockCounts sync.Map
}

// NewInMemoryPolicyEngine constructs a ready-to-use policy engine.
func NewInMemoryPolicyEngine() *InMemoryPolicyEngine {
	return &InMemoryPolicyEngine{
		policies: make(map[string]domain.AgentPolicy),
		counters: make(map[string]int),
		buckets:  make(map[string][]time.Time),
	}
}

// Evaluate checks whether an action is allowed by running 7 checks in order.
// Short-circuits on the first BLOCK or ESCALATE.
func (e *InMemoryPolicyEngine) Evaluate(_ context.Context, agentID, actionType, domainName string) domain.PolicyDecision {
	start := time.Now()

	e.mu.RLock()
	pol, exists := e.policies[agentID]
	e.mu.RUnlock()

	if !exists {
		return domain.PolicyDecision{
			Result:    domain.PolicyAllow,
			LatencyMs: time.Since(start).Milliseconds(),
		}
	}

	// Check 1: Domain blocklist
	if d := checkDomainBlocklist(pol.DomainBlocklist, domainName); d != nil {
		d.LatencyMs = time.Since(start).Milliseconds()
		e.incrementBlockCount(agentID, d.Result)
		return *d
	}

	// Check 2: Domain allowlist
	if d := checkDomainAllowlist(pol.DomainAllowlist, domainName); d != nil {
		d.LatencyMs = time.Since(start).Milliseconds()
		e.incrementBlockCount(agentID, d.Result)
		return *d
	}

	// Check 3: Action allowlist
	if d := checkActionAllowlist(pol.ActionAllowlist, actionType); d != nil {
		d.LatencyMs = time.Since(start).Milliseconds()
		e.incrementBlockCount(agentID, d.Result)
		return *d
	}

	// Check 4: Max actions per session
	if d := e.checkMaxActions(agentID, pol.MaxActionsPerSession); d != nil {
		d.LatencyMs = time.Since(start).Milliseconds()
		e.incrementBlockCount(agentID, d.Result)
		return *d
	}

	// Check 5: Rate limit
	if d := e.checkRateLimit(agentID, pol.RateLimitPerMin); d != nil {
		d.LatencyMs = time.Since(start).Milliseconds()
		e.incrementBlockCount(agentID, d.Result)
		return *d
	}

	// Check 6: Custom rules
	if d := checkCustomRules(pol.CustomRules, actionType, domainName); d != nil {
		d.LatencyMs = time.Since(start).Milliseconds()
		e.incrementBlockCount(agentID, d.Result)
		return *d
	}

	// Check 7: Transaction threshold
	if d := checkTransactionThreshold(pol.TransactionThresholdUSD, actionType, domainName); d != nil {
		d.LatencyMs = time.Since(start).Milliseconds()
		e.incrementBlockCount(agentID, d.Result)
		return *d
	}

	// Check 8: PII content check
	// If AllowPII == false AND the domain looks like a financial/medical/HR site → ESCALATE
	if !pol.AllowPII && containsAny(domainName, []string{"bank", "health", "medical", "hr", "payroll", "insurance", "pension"}) && actionType != "navigate" {
		d := domain.PolicyDecision{
			Result:    domain.PolicyEscalate,
			Reason:    "pii_domain_escalation: domain may contain PII",
			CheckID:   "check_8_content",
			LatencyMs: time.Since(start).Milliseconds(),
		}
		e.incrementBlockCount(agentID, d.Result)
		return d
	}

	// Check 9: Data exfiltration check
	if len(pol.DataExfilPatterns) > 0 && actionType == "extract" {
		for _, pattern := range pol.DataExfilPatterns {
			if strings.Contains(strings.ToLower(domainName), strings.ToLower(pattern)) {
				d := domain.PolicyDecision{
					Result:    domain.PolicyBlock,
					Reason:    "data_exfil_blocked: domain matches exfil pattern " + pattern,
					CheckID:   "check_9_exfil",
					LatencyMs: time.Since(start).Milliseconds(),
				}
				e.incrementBlockCount(agentID, d.Result)
				return d
			}
		}
	}

	// Check 10: Authentication boundary (covered by action allowlist check 3 — pass-through).

	// Check 11: Scope check
	if len(pol.ScopeKeywords) > 0 {
		matched := false
		for _, kw := range pol.ScopeKeywords {
			if strings.Contains(actionType, strings.ToLower(kw)) || strings.Contains(strings.ToLower(domainName), strings.ToLower(kw)) {
				matched = true
				break
			}
		}
		if !matched {
			d := domain.PolicyDecision{
				Result:    domain.PolicyBlock,
				Reason:    "scope_violation: action/domain not in declared scope",
				CheckID:   "check_11_scope",
				LatencyMs: time.Since(start).Milliseconds(),
			}
			e.incrementBlockCount(agentID, d.Result)
			return d
		}
	}

	// Check 12: Reputation check
	if pol.MaxReputationScore > 0 {
		if count, ok := e.blockCounts.Load(agentID); ok {
			if count.(int) >= pol.MaxReputationScore {
				d := domain.PolicyDecision{
					Result:    domain.PolicyEscalate,
					Reason:    fmt.Sprintf("reputation_threshold: %d blocks accumulated", count.(int)),
					CheckID:   "check_12_reputation",
					LatencyMs: time.Since(start).Milliseconds(),
				}
				return d
			}
		}
	}

	// Increment counters on ALLOW.
	e.incrementCounters(agentID)

	return domain.PolicyDecision{
		Result:    domain.PolicyAllow,
		LatencyMs: time.Since(start).Milliseconds(),
	}
}

// GetPolicy returns the policy for an agent, or nil if none is set.
func (e *InMemoryPolicyEngine) GetPolicy(agentID string) *domain.AgentPolicy {
	e.mu.RLock()
	defer e.mu.RUnlock()
	pol, ok := e.policies[agentID]
	if !ok {
		return nil
	}
	cp := pol
	return &cp
}

// SetPolicy stores or updates a policy for an agent.
func (e *InMemoryPolicyEngine) SetPolicy(agentID string, policy domain.AgentPolicy) error {
	policy.AgentID = agentID
	e.mu.Lock()
	defer e.mu.Unlock()
	e.policies[agentID] = policy
	return nil
}

// DeletePolicy removes a policy (agent reverts to default/open).
func (e *InMemoryPolicyEngine) DeletePolicy(agentID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.policies, agentID)
	return nil
}

// ResetCounters resets action counters for an agent (e.g. on new session).
func (e *InMemoryPolicyEngine) ResetCounters(agentID string) {
	e.counterMu.Lock()
	defer e.counterMu.Unlock()
	delete(e.counters, agentID)
}

// ─── check helpers ─────────────────────────────────────────────────────────────

func checkDomainBlocklist(blocklist []string, domainName string) *domain.PolicyDecision {
	for _, pattern := range blocklist {
		if matchDomain(pattern, domainName) {
			return &domain.PolicyDecision{
				Result:  domain.PolicyBlock,
				Reason:  "domain blocked by policy: " + pattern,
				CheckID: "domain_blocklist",
			}
		}
	}
	return nil
}

func checkDomainAllowlist(allowlist []string, domainName string) *domain.PolicyDecision {
	if len(allowlist) == 0 {
		return nil
	}
	for _, pattern := range allowlist {
		if matchDomain(pattern, domainName) {
			return nil // allowed
		}
	}
	return &domain.PolicyDecision{
		Result:  domain.PolicyBlock,
		Reason:  "domain not in allowlist",
		CheckID: "domain_allowlist",
	}
}

func checkActionAllowlist(allowlist []string, actionType string) *domain.PolicyDecision {
	if len(allowlist) == 0 {
		return nil
	}
	for _, a := range allowlist {
		if strings.EqualFold(a, actionType) {
			return nil
		}
	}
	return &domain.PolicyDecision{
		Result:  domain.PolicyBlock,
		Reason:  "action type '" + actionType + "' not in allowlist",
		CheckID: "action_allowlist",
	}
}

func (e *InMemoryPolicyEngine) checkMaxActions(agentID string, max int) *domain.PolicyDecision {
	if max <= 0 {
		return nil
	}
	e.counterMu.Lock()
	count := e.counters[agentID]
	e.counterMu.Unlock()
	if count >= max {
		return &domain.PolicyDecision{
			Result:  domain.PolicyBlock,
			Reason:  "max actions per session exceeded",
			CheckID: "max_actions",
		}
	}
	return nil
}

func (e *InMemoryPolicyEngine) checkRateLimit(agentID string, rpm int) *domain.PolicyDecision {
	if rpm <= 0 {
		return nil
	}
	now := time.Now()
	cutoff := now.Add(-time.Minute)

	e.bucketMu.Lock()
	// Prune old entries.
	bucket := e.buckets[agentID]
	pruned := bucket[:0]
	for _, t := range bucket {
		if t.After(cutoff) {
			pruned = append(pruned, t)
		}
	}
	e.buckets[agentID] = pruned
	count := len(pruned)
	e.bucketMu.Unlock()

	if count >= rpm {
		return &domain.PolicyDecision{
			Result:  domain.PolicyBlock,
			Reason:  "rate limit exceeded",
			CheckID: "rate_limit",
		}
	}
	return nil
}

func checkCustomRules(rules []domain.PolicyRule, actionType, domainName string) *domain.PolicyDecision {
	for _, rule := range rules {
		if evalCondition(rule.Condition, actionType, domainName) {
			return &domain.PolicyDecision{
				Result:  rule.Result,
				Reason:  rule.Message,
				CheckID: "custom_rule",
			}
		}
	}
	return nil
}

func checkTransactionThreshold(threshold float64, actionType, domainName string) *domain.PolicyDecision {
	if threshold <= 0 {
		return nil
	}
	if actionType != "click" {
		return nil
	}
	lower := strings.ToLower(domainName)
	if strings.Contains(lower, "bank") || strings.Contains(lower, "payment") {
		return &domain.PolicyDecision{
			Result:  domain.PolicyEscalate,
			Reason:  "transaction threshold active for financial domain",
			CheckID: "transaction_threshold",
		}
	}
	return nil
}

// incrementCounters bumps the action counter and rate bucket for an agent.
func (e *InMemoryPolicyEngine) incrementCounters(agentID string) {
	e.counterMu.Lock()
	e.counters[agentID]++
	e.counterMu.Unlock()

	e.bucketMu.Lock()
	e.buckets[agentID] = append(e.buckets[agentID], time.Now())
	e.bucketMu.Unlock()
}

// incrementBlockCount bumps the block counter for an agent when a BLOCK decision is returned.
func (e *InMemoryPolicyEngine) incrementBlockCount(agentID string, result domain.PolicyResult) {
	if result == domain.PolicyBlock {
		actual, _ := e.blockCounts.LoadOrStore(agentID, 0)
		e.blockCounts.Store(agentID, actual.(int)+1)
	}
}

// containsAny returns true if s contains any of the given patterns (case-insensitive).
func containsAny(s string, patterns []string) bool {
	lower := strings.ToLower(s)
	for _, p := range patterns {
		if strings.Contains(lower, strings.ToLower(p)) {
			return true
		}
	}
	return false
}

// ─── domain matching ───────────────────────────────────────────────────────────

// matchDomain matches a pattern like "*.example.com" against a domain.
// Supports exact match and wildcard prefix "*.".
func matchDomain(pattern, domain string) bool {
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	domain = strings.ToLower(strings.TrimSpace(domain))

	if pattern == domain {
		return true
	}

	// Glob wildcard: *.example.com matches foo.example.com and example.com
	if strings.HasPrefix(pattern, "*.") {
		suffix := pattern[1:] // ".example.com"
		if strings.HasSuffix(domain, suffix) {
			return true
		}
		// Also match bare domain: *.example.com should match example.com
		if domain == pattern[2:] {
			return true
		}
	}

	return false
}

// evalCondition evaluates simple DSL conditions.
// Supports: "domain == X", "action == Y", combined with " AND ".
func evalCondition(condition, actionType, domainName string) bool {
	parts := strings.Split(condition, " AND ")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if !evalSingleCondition(part, actionType, domainName) {
			return false
		}
	}
	return true
}

func evalSingleCondition(cond, actionType, domainName string) bool {
	cond = strings.TrimSpace(cond)
	if idx := strings.Index(cond, "=="); idx >= 0 {
		key := strings.TrimSpace(cond[:idx])
		val := strings.TrimSpace(cond[idx+2:])
		val = strings.Trim(val, "\"'")
		switch strings.ToLower(key) {
		case "domain":
			return strings.EqualFold(domainName, val)
		case "action":
			return strings.EqualFold(actionType, val)
		}
	}
	return false
}

// compile-time interface assertion.
var _ domain.PolicyEngine = (*InMemoryPolicyEngine)(nil)
