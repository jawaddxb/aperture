package resolver

import (
	"strings"

	"github.com/ApertureHQ/aperture/internal/domain"
)

// searchByRole walks the tree and collects MatchResults for nodes matching role/name.
func searchByRole(node *domain.AXNode, role, name string) []domain.MatchResult {
	if node == nil {
		return nil
	}
	var results []domain.MatchResult
	collectByRole(node, role, name, &results)
	return results
}

// collectByRole recursively collects matches for role/name from the tree.
func collectByRole(node *domain.AXNode, role, name string, out *[]domain.MatchResult) {
	if node == nil {
		return
	}

	confidence := computeConfidence(node, role, name)
	if confidence > 0 {
		*out = append(*out, domain.MatchResult{Node: node, Confidence: confidence})
	}

	for _, child := range node.Children {
		collectByRole(child, role, name, out)
	}
}

// computeConfidence returns the match confidence for a node given target role/name.
// Returns 0 when the role does not match at all.
func computeConfidence(node *domain.AXNode, role, name string) float64 {
	nodeRole := strings.ToLower(strings.TrimSpace(node.Role))
	targetRole := strings.ToLower(strings.TrimSpace(role))

	if nodeRole != targetRole {
		return 0
	}

	// Role matched; evaluate name.
	if name == "" {
		return 0.3 // role-only query
	}

	nodeName := strings.TrimSpace(node.Name)
	targetName := strings.TrimSpace(name)

	if nodeName == targetName {
		return 1.0 // exact match
	}

	lowerNode := strings.ToLower(nodeName)
	lowerTarget := strings.ToLower(targetName)

	if lowerNode == lowerTarget {
		return 0.95 // case-insensitive exact
	}

	if strings.Contains(lowerNode, lowerTarget) || strings.Contains(lowerTarget, lowerNode) {
		// Score by overlap ratio; partial matches fall in [0.5, 0.9).
		ratio := float64(len(lowerTarget)) / float64(max(len(lowerNode), 1))
		if ratio > 1 {
			ratio = float64(len(lowerNode)) / float64(len(lowerTarget))
		}
		return 0.5 + ratio*0.4
	}

	return 0.3 // role-only — name didn't match
}

// collectInteractable recursively collects nodes with interactable roles.
func collectInteractable(node *domain.AXNode, out *[]*domain.AXNode) {
	if node == nil {
		return
	}
	role := strings.ToLower(strings.TrimSpace(node.Role))
	if interactableRoles[role] {
		*out = append(*out, node)
	}
	for _, child := range node.Children {
		collectInteractable(child, out)
	}
}
