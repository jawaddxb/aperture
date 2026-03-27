package resolver_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/ApertureHQ/aperture/internal/resolver"
)

// ─── stub implementations ──────────────────────────────────────────────────────

// stubBrowserInstance satisfies domain.BrowserInstance for unit tests.
// It holds a pre-cancelled context so no actual CDP calls succeed.
type stubBrowserInstance struct {
	ctx context.Context
}

func (s *stubBrowserInstance) Context() context.Context { return s.ctx }
func (s *stubBrowserInstance) ID() string               { return "stub-0" }
func (s *stubBrowserInstance) CreatedAt() time.Time     { return time.Time{} }
func (s *stubBrowserInstance) IsAlive() bool            { return false }
func (s *stubBrowserInstance) Close() error             { return nil }
func (s *stubBrowserInstance) Downloads() domain.DownloadManager { return nil }
func (s *stubBrowserInstance) Network() domain.NetworkManager   { return nil }

// stubAXTreeResolver is an AXTreeResolver whose Snapshot behaviour is injected.
// It wraps an ordinary AXTreeResolver but overrides Snapshot + FindByRole via
// a test-only interface (injected via factory function).
//
// We instead use the factory injection pattern: the unified resolver accepts
// factory functions, so we provide factories that return pre-seeded resolvers.
// The trick: we subvert the AXTreeResolver by providing an outer object that
// satisfies the same interface.
//
// Since AXTreeResolver is a concrete type (not an interface), we supply a
// *canned* resolver that stores results ahead of time and returns them.

// cannedAXResolver records canned results and returns them from FindByRole.
// It fully satisfies domain.ElementResolver.
type cannedAXResolver struct {
	snapshotErr error
	results     []domain.MatchResult
}

func (c *cannedAXResolver) Snapshot(_ context.Context) (*domain.AXTree, error) {
	if c.snapshotErr != nil {
		return nil, c.snapshotErr
	}
	return &domain.AXTree{Index: make(map[string]*domain.AXNode)}, nil
}

func (c *cannedAXResolver) FindByRole(_, _ string) ([]domain.MatchResult, error) {
	return c.results, nil
}

func (c *cannedAXResolver) FindInteractable() ([]*domain.AXNode, error) {
	return nil, nil
}

// cannedDOMResolver records canned results returned from FindByText.
type cannedDOMResolver struct {
	textResults     []domain.MatchResult
	selectorResults []domain.MatchResult
	patternResults  []domain.MatchResult
}

func (c *cannedDOMResolver) Snapshot(_ context.Context) (*domain.AXTree, error) {
	return &domain.AXTree{Index: make(map[string]*domain.AXNode)}, nil
}

func (c *cannedDOMResolver) FindByRole(_, _ string) ([]domain.MatchResult, error) {
	return c.textResults, nil
}

func (c *cannedDOMResolver) FindInteractable() ([]*domain.AXNode, error) {
	return nil, nil
}

func (c *cannedDOMResolver) FindByText(_ context.Context, _ string) ([]domain.MatchResult, error) {
	return c.textResults, nil
}

func (c *cannedDOMResolver) FindBySelector(_ context.Context, _ string) ([]domain.MatchResult, error) {
	return c.selectorResults, nil
}

func (c *cannedDOMResolver) FindByPattern(_ context.Context, _ []string) ([]domain.MatchResult, error) {
	return c.patternResults, nil
}

// ─── factory helpers ───────────────────────────────────────────────────────────

// makeNode constructs a minimal AXNode for use in canned results.
func makeNode(role, name string) *domain.AXNode {
	return &domain.AXNode{
		SemanticID: resolver.SemanticID(role, name, "test/path"),
		Role:       role,
		Name:       name,
	}
}

// ─── unit tests ────────────────────────────────────────────────────────────────

// TestResolver_AXTreeHit verifies that a high-confidence AX match (>0.9) auto-selects.
func TestResolver_AXTreeHit(t *testing.T) {
	axCanned := &cannedAXResolver{
		results: []domain.MatchResult{
			{Node: makeNode("button", "Buy Now"), Confidence: 1.0},
		},
	}
	domCanned := &cannedDOMResolver{} // empty — should not be consulted

	r := newTestUnifiedResolver(axCanned, domCanned)
	inst := &stubBrowserInstance{ctx: inst_unused()}

	res, err := r.Resolve(context.Background(), domain.ResolutionTarget{
		Text: "Buy Now",
		Role: "button",
	}, inst)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(res.Candidates) != 1 {
		t.Fatalf("expected 1 candidate (auto-select), got %d", len(res.Candidates))
	}
	if res.Confidence != 1.0 {
		t.Errorf("Confidence = %.2f, want 1.0", res.Confidence)
	}
	if res.Tier != domain.TierAXTree {
		t.Errorf("Tier = %q, want %q", res.Tier, domain.TierAXTree)
	}
	if res.Candidates[0].ResolutionTier != domain.TierAXTree {
		t.Errorf("candidate Tier = %q, want ax_tree", res.Candidates[0].ResolutionTier)
	}
	t.Logf("AXTreeHit: candidate=%+v", res.Candidates[0])
}

// TestResolver_DOMFallback verifies that when AX finds nothing, DOM results are used.
func TestResolver_DOMFallback(t *testing.T) {
	axCanned := &cannedAXResolver{
		snapshotErr: errors.New("ax tree unavailable"),
	}
	domCanned := &cannedDOMResolver{
		textResults: []domain.MatchResult{
			{Node: makeNode("button", "Submit"), Confidence: 0.75},
		},
	}

	r := newTestUnifiedResolver(axCanned, domCanned)
	inst := &stubBrowserInstance{ctx: inst_unused()}

	res, err := r.Resolve(context.Background(), domain.ResolutionTarget{
		Text: "Submit",
	}, inst)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(res.Candidates) == 0 {
		t.Fatal("expected at least one candidate from DOM fallback, got 0")
	}
	if res.Tier != domain.TierDOMHeuristic {
		t.Errorf("Tier = %q, want dom_heuristic", res.Tier)
	}
	for _, c := range res.Candidates {
		if c.ResolutionTier != domain.TierDOMHeuristic {
			t.Errorf("candidate Tier = %q, want dom_heuristic (name=%q)", c.ResolutionTier, c.Name)
		}
	}
	t.Logf("DOMFallback: %d candidates, top confidence=%.2f", len(res.Candidates), res.Confidence)
}

// TestResolver_Disambiguation verifies that multiple matches are returned ranked by confidence.
func TestResolver_Disambiguation(t *testing.T) {
	axCanned := &cannedAXResolver{
		results: []domain.MatchResult{
			{Node: makeNode("button", "Submit Form"), Confidence: 0.80},
			{Node: makeNode("button", "Submit Order"), Confidence: 0.70},
		},
	}
	domCanned := &cannedDOMResolver{
		textResults: []domain.MatchResult{
			{Node: makeNode("button", "Submit"), Confidence: 0.75},
		},
	}

	r := newTestUnifiedResolver(axCanned, domCanned)
	inst := &stubBrowserInstance{ctx: inst_unused()}

	res, err := r.Resolve(context.Background(), domain.ResolutionTarget{
		Text: "Submit",
	}, inst)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(res.Candidates) < 2 {
		t.Fatalf("expected >= 2 candidates for disambiguation, got %d", len(res.Candidates))
	}

	// Candidates must be sorted by confidence descending.
	for i := 1; i < len(res.Candidates); i++ {
		if res.Candidates[i].Confidence > res.Candidates[i-1].Confidence {
			t.Errorf("candidates not sorted: index %d (%.2f) > index %d (%.2f)",
				i, res.Candidates[i].Confidence, i-1, res.Candidates[i-1].Confidence)
		}
	}

	// Top candidate confidence must equal res.Confidence.
	if res.Confidence != res.Candidates[0].Confidence {
		t.Errorf("res.Confidence=%.2f but top candidate=%.2f", res.Confidence, res.Candidates[0].Confidence)
	}

	t.Logf("Disambiguation: %d candidates", len(res.Candidates))
	for _, c := range res.Candidates {
		t.Logf("  [%.2f] %s %q tier=%s", c.Confidence, c.Role, c.Name, c.ResolutionTier)
	}
}

// TestResolver_NoMatch verifies that a clear ErrNoMatch is returned when nothing is found.
func TestResolver_NoMatch(t *testing.T) {
	axCanned := &cannedAXResolver{} // no results
	domCanned := &cannedDOMResolver{} // no results

	r := newTestUnifiedResolver(axCanned, domCanned)
	inst := &stubBrowserInstance{ctx: inst_unused()}

	_, err := r.Resolve(context.Background(), domain.ResolutionTarget{
		Text: "Nonexistent Button",
	}, inst)
	if err == nil {
		t.Fatal("expected ErrNoMatch, got nil error")
	}

	var noMatch *domain.ErrNoMatch
	if !errors.As(err, &noMatch) {
		t.Errorf("error type = %T, want *domain.ErrNoMatch", err)
	}
	t.Logf("NoMatch error: %v", err)
}

// TestResolver_CandidateFields verifies all required fields are present on each candidate.
func TestResolver_CandidateFields(t *testing.T) {
	axCanned := &cannedAXResolver{
		results: []domain.MatchResult{
			{Node: makeNode("link", "Home"), Confidence: 0.85},
		},
	}
	domCanned := &cannedDOMResolver{
		textResults: []domain.MatchResult{
			{Node: makeNode("link", "Home page"), Confidence: 0.70},
		},
	}

	r := newTestUnifiedResolver(axCanned, domCanned)
	inst := &stubBrowserInstance{ctx: inst_unused()}

	res, err := r.Resolve(context.Background(), domain.ResolutionTarget{Text: "Home"}, inst)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	for _, c := range res.Candidates {
		if c.SemanticID == "" {
			t.Errorf("candidate missing SemanticID (name=%q)", c.Name)
		}
		if len(c.SemanticID) != 16 {
			t.Errorf("SemanticID len=%d, want 16 (name=%q)", len(c.SemanticID), c.Name)
		}
		if c.Role == "" {
			t.Errorf("candidate missing Role (name=%q)", c.Name)
		}
		if c.ResolutionTier == "" {
			t.Errorf("candidate missing ResolutionTier (name=%q)", c.Name)
		}
		if c.Confidence <= 0 || c.Confidence > 1.0 {
			t.Errorf("candidate Confidence=%.2f out of [0,1] range (name=%q)", c.Confidence, c.Name)
		}
	}
}

// ─── helpers ──────────────────────────────────────────────────────────────────

// newTestUnifiedResolver builds a UnifiedResolver backed by canned resolvers.
// It uses the factory injection constructor so no real browser is needed.
func newTestUnifiedResolver(ax *cannedAXResolver, dom *cannedDOMResolver) domain.UnifiedResolver {
	// We need to adapt canned resolvers into factory functions.
	// AXTreeResolver and DOMResolver are concrete types; we can't easily swap them
	// via the current public API of NewUnifiedResolverWithFactories which expects
	// concrete types. Instead, we expose a test-helper constructor that accepts
	// domain.ElementResolver interfaces.
	return resolver.NewUnifiedResolverWithCanned(ax, dom)
}
