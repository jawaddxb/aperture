package executor

import (
	"fmt"
	"strings"

	"github.com/ApertureHQ/aperture/internal/domain"
)

// rawSelectorPrefix is a sentinel stored in Candidate.SemanticID to signal that
// the value after the prefix should be used verbatim as a CSS selector.
// This path is used in tests and when the resolver provides an exact DOM selector.
const rawSelectorPrefix = "raw:"

// selectorForCandidate builds a CSS selector from a Candidate.
//
// Priority:
//  1. If SemanticID starts with "raw:" → use the rest as a literal CSS selector.
//  2. If SemanticID is a non-empty 16-char hex string → data-aperture-id attribute.
//  3. If Role and Name are both non-empty → [role="…"][aria-label="…"].
//  4. Fallback: "*" (unsafe; caller should validate).
func selectorForCandidate(c domain.Candidate) string {
	if strings.HasPrefix(c.SemanticID, rawSelectorPrefix) {
		return c.SemanticID[len(rawSelectorPrefix):]
	}
	if c.SemanticID != "" {
		return fmt.Sprintf("[data-aperture-id=%q]", c.SemanticID)
	}
	if c.Role != "" && c.Name != "" {
		return fmt.Sprintf("[role=%q][aria-label=%q]", c.Role, c.Name)
	}
	return "*"
}
