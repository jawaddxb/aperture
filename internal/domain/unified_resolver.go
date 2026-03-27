// Package domain defines core interfaces for Aperture.
// This file extends the resolver domain with unified resolution types.
package domain

import "context"

// ResolutionTier identifies which strategy produced a resolution candidate.
type ResolutionTier string

const (
	// TierAXTree indicates the candidate was found via the accessibility tree.
	TierAXTree ResolutionTier = "ax_tree"

	// TierDOMHeuristic indicates the candidate was found via DOM-based heuristics.
	TierDOMHeuristic ResolutionTier = "dom_heuristic"
)

// ResolutionTarget describes the element to resolve during a Resolve call.
// At least one field should be non-empty for meaningful resolution.
type ResolutionTarget struct {
	// Text is the visible text content to search for (e.g. button label).
	Text string

	// Role is an optional WAI-ARIA role to narrow the AX-tree search.
	Role string

	// Selector is an optional CSS selector for direct DOM targeting.
	Selector string
}

// Candidate is a single resolution result with its confidence and provenance.
type Candidate struct {
	// SemanticID is the stable 16-character hex element identifier.
	SemanticID string

	// Role is the WAI-ARIA role of the matched element.
	Role string

	// Name is the accessible name of the matched element.
	Name string

	// Confidence is a value in [0.0, 1.0] indicating match quality.
	Confidence float64

	// ResolutionTier identifies which strategy produced this candidate.
	ResolutionTier ResolutionTier
}

// Resolution is the response from a UnifiedResolver.Resolve call.
// Candidates are sorted by confidence descending.
// When Confidence > 0.9 exactly one candidate is returned (auto-selected).
type Resolution struct {
	// Tier is the primary strategy that produced the best-ranking candidate.
	Tier ResolutionTier

	// Confidence is the confidence score of the top candidate (0.0–1.0).
	Confidence float64

	// Candidates is the ranked list of matched elements.
	Candidates []Candidate
}

// DOMElementResolver extends ElementResolver with DOM-specific query methods.
// Implementations use JavaScript evaluation via CDP to query DOM state directly,
// bypassing the accessibility tree for situations where AX data is sparse.
type DOMElementResolver interface {
	ElementResolver

	// FindByText queries elements whose visible text, aria-label, or placeholder
	// matches text exactly. Confidence: 0.75 for exact text match.
	FindByText(ctx context.Context, text string) ([]MatchResult, error)

	// FindBySelector queries elements matching a CSS selector.
	// Confidence: 0.70 for structural CSS matches.
	FindBySelector(ctx context.Context, css string) ([]MatchResult, error)

	// FindByPattern queries elements using common structural patterns such as
	// button[type=submit], input[type=text], and a[href].
	// Confidence: 0.65 for heuristic pattern matches.
	FindByPattern(ctx context.Context, patterns []string) ([]MatchResult, error)
}

// UnifiedResolver resolves page elements using a tiered AX-tree + DOM strategy.
//
// Confidence thresholds:
//   - > 0.9  : auto-select, return single candidate
//   - 0.5–0.9: return ranked candidates (AX + DOM merged)
//   - < 0.5  : DOM-only fallback (AX match discarded)
type UnifiedResolver interface {
	// Resolve finds elements matching target in the given browser instance.
	// Returns *ErrNoMatch when no candidates are found.
	Resolve(ctx context.Context, target ResolutionTarget, inst BrowserInstance) (*Resolution, error)
}

// ErrNoMatch is returned by UnifiedResolver.Resolve when no elements are found.
type ErrNoMatch struct {
	Target ResolutionTarget
}

// Error implements the error interface.
func (e *ErrNoMatch) Error() string {
	return "resolver: no match found for target text=" + e.Target.Text +
		" role=" + e.Target.Role + " selector=" + e.Target.Selector
}
