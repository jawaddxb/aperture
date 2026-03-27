// Package resolver contains element resolution strategies for Aperture.
// This file implements DOMResolver, the Tier 2 DOM-heuristic fallback.
// Helper functions (JS builders, AXNode conversion) live in dom_helpers.go.
package resolver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/chromedp/chromedp"
)

// CommonPatterns contains structural CSS selectors applied by FindByPattern.
// They cover the most common interactive element shapes on the web.
var CommonPatterns = []string{
	"button[type=submit]",
	"button[type=button]",
	"input[type=text]",
	"input[type=email]",
	"input[type=password]",
	"input[type=search]",
	"input[type=submit]",
	"a[href]",
	"select",
	"textarea",
}

// domElement is the raw shape returned from the JavaScript evaluations.
type domElement struct {
	Tag         string `json:"tag"`
	Type        string `json:"type"`
	Text        string `json:"text"`
	AriaLabel   string `json:"ariaLabel"`
	Placeholder string `json:"placeholder"`
	Role        string `json:"role"`
	ID          string `json:"id"`
	Name        string `json:"name"`
	Href        string `json:"href"`
	Selector    string `json:"selector"`
}

// DOMResolver implements domain.DOMElementResolver using JavaScript evaluation
// via CDP. It holds a reference to the browser tab context.
// A DOMResolver is NOT safe for concurrent use.
type DOMResolver struct {
	browserCtx context.Context
	lastTree   *domain.AXTree
}

// NewDOMResolver creates a DOMResolver for the given browser tab context.
// browserCtx must be a chromedp tab context.
func NewDOMResolver(browserCtx context.Context) *DOMResolver {
	return &DOMResolver{browserCtx: browserCtx}
}

// Snapshot is a no-op tree snapshot for interface compliance.
// DOMResolver does not maintain an AX tree; it queries the DOM directly.
// Implements domain.ElementResolver.
func (d *DOMResolver) Snapshot(_ context.Context) (*domain.AXTree, error) {
	d.lastTree = &domain.AXTree{Index: make(map[string]*domain.AXNode)}
	return d.lastTree, nil
}

// FindByRole satisfies domain.ElementResolver by delegating to FindByText.
// When name is non-empty the name is used as the text query; otherwise it
// queries elements whose ARIA role matches the given role.
// Implements domain.ElementResolver.
func (d *DOMResolver) FindByRole(role, name string) ([]domain.MatchResult, error) {
	if name != "" {
		return d.FindByText(context.Background(), name)
	}
	css := fmt.Sprintf("[role=%q]", role)
	return d.FindBySelector(context.Background(), css)
}

// FindInteractable returns elements matching CommonPatterns.
// Implements domain.ElementResolver.
func (d *DOMResolver) FindInteractable() ([]*domain.AXNode, error) {
	results, err := d.FindByPattern(context.Background(), CommonPatterns)
	if err != nil {
		return nil, err
	}
	nodes := make([]*domain.AXNode, 0, len(results))
	for i := range results {
		nodes = append(nodes, results[i].Node)
	}
	return nodes, nil
}

// FindByText queries elements whose visible text, aria-label, or placeholder
// contains text (case-insensitive). Confidence: 0.75 for each result.
// Implements domain.DOMElementResolver.
func (d *DOMResolver) FindByText(ctx context.Context, text string) ([]domain.MatchResult, error) {
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}

	script := buildFindByTextScript(text)
	elems, err := d.evalScript(ctx, script)
	if err != nil {
		return nil, fmt.Errorf("dom FindByText: %w", err)
	}

	results := make([]domain.MatchResult, 0, len(elems))
	for _, el := range elems {
		node := domElementToAXNode(el)
		confidence := textMatchConfidence(el, text)
		results = append(results, domain.MatchResult{Node: node, Confidence: confidence})
	}
	return results, nil
}

// FindBySelector queries elements matching a CSS selector.
// Confidence: 0.70 per result.
// Implements domain.DOMElementResolver.
func (d *DOMResolver) FindBySelector(ctx context.Context, css string) ([]domain.MatchResult, error) {
	if strings.TrimSpace(css) == "" {
		return nil, nil
	}

	script := buildFindBySelectorScript(css)
	elems, err := d.evalScript(ctx, script)
	if err != nil {
		return nil, fmt.Errorf("dom FindBySelector: %w", err)
	}

	results := make([]domain.MatchResult, 0, len(elems))
	for _, el := range elems {
		node := domElementToAXNode(el)
		results = append(results, domain.MatchResult{Node: node, Confidence: 0.70})
	}
	return results, nil
}

// FindByPattern queries elements using the provided CSS pattern selectors.
// Confidence: 0.65 per result. Duplicates (same selector path) are deduplicated.
// Implements domain.DOMElementResolver.
func (d *DOMResolver) FindByPattern(ctx context.Context, patterns []string) ([]domain.MatchResult, error) {
	if len(patterns) == 0 {
		return nil, nil
	}

	script := buildFindByPatternScript(patterns)
	elems, err := d.evalScript(ctx, script)
	if err != nil {
		return nil, fmt.Errorf("dom FindByPattern: %w", err)
	}

	seen := make(map[string]bool, len(elems))
	results := make([]domain.MatchResult, 0, len(elems))
	for _, el := range elems {
		if seen[el.Selector] {
			continue
		}
		seen[el.Selector] = true
		node := domElementToAXNode(el)
		results = append(results, domain.MatchResult{Node: node, Confidence: 0.65})
	}
	return results, nil
}

// ─── internal helpers ──────────────────────────────────────────────────────────

// evalScript runs a JavaScript expression in the browser and decodes the result
// as a []domElement. Returns nil slice when the page returns null/undefined.
func (d *DOMResolver) evalScript(ctx context.Context, script string) ([]domElement, error) {
	var raw string
	action := chromedp.Evaluate(script, &raw)
	if err := chromedp.Run(d.browserCtx, action); err != nil {
		return nil, err
	}
	if raw == "" || raw == "null" || raw == "undefined" {
		return nil, nil
	}
	var elems []domElement
	if err := json.Unmarshal([]byte(raw), &elems); err != nil {
		return nil, fmt.Errorf("decode DOM result: %w", err)
	}
	return elems, nil
}
