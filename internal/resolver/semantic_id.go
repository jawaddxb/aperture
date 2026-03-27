// Package resolver provides concrete implementations of domain.ElementResolver
// that extract and index the Chrome accessibility tree via CDP.
package resolver

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// SemanticID generates a stable 16-character hex identifier for an AX node.
// The identifier is derived from the node's role, accessible name, and
// ancestry path so that the same logical element produces the same ID
// even after a page re-render.
//
// Format: sha256(role + "|" + name + "|" + ancestryPath)[:16] (hex string).
func SemanticID(role, name, ancestryPath string) string {
	raw := buildSemanticInput(role, name, ancestryPath)
	return hashToHex16(raw)
}

// buildSemanticInput constructs the canonical string fed to the hash function.
// All components are normalised to lower-case and trimmed so that minor
// whitespace or casing differences in the browser do not break stability.
func buildSemanticInput(role, name, ancestryPath string) string {
	parts := [3]string{
		strings.ToLower(strings.TrimSpace(role)),
		strings.TrimSpace(name),
		strings.ToLower(strings.TrimSpace(ancestryPath)),
	}
	return parts[0] + "|" + parts[1] + "|" + parts[2]
}

// hashToHex16 returns the first 16 characters of the hex-encoded SHA-256 digest.
func hashToHex16(input string) string {
	sum := sha256.Sum256([]byte(input))
	full := hex.EncodeToString(sum[:])
	return full[:16]
}

// buildAncestryPath constructs an ancestry path string from a slice of ancestor
// roles and names in root-to-parent order.
// Example: ["WebArea", "main", "list", "listitem"] → "webarea/main/list/listitem"
func buildAncestryPath(ancestors []ancestorInfo) string {
	if len(ancestors) == 0 {
		return ""
	}
	parts := make([]string, 0, len(ancestors))
	for _, a := range ancestors {
		role := strings.ToLower(strings.TrimSpace(a.Role))
		name := strings.TrimSpace(a.Name)
		if name != "" {
			parts = append(parts, role+":"+name)
		} else {
			parts = append(parts, role)
		}
	}
	return strings.Join(parts, "/")
}

// ancestorInfo holds the role and name of a single ancestor node for path building.
type ancestorInfo struct {
	Role string
	Name string
}
