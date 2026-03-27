// Package domain defines core interfaces for Aperture.
// All services depend on these interfaces, never on concrete implementations.
package domain

import "context"

// BoundingBox represents the screen-space rectangle of a rendered element.
// Coordinates are in CSS pixels relative to the page origin.
type BoundingBox struct {
	X      float64
	Y      float64
	Width  float64
	Height float64
}

// AXNode is a resolved accessibility tree node enriched with a stable semantic ID.
// It is constructed by an ElementResolver from the raw CDP accessibility tree.
type AXNode struct {
	// SemanticID is a stable 16-character hex identifier derived from the node's
	// role, name, and ancestry path. The same logical element produces the same
	// SemanticID across page re-renders.
	SemanticID string

	// Role is the WAI-ARIA role of the element (e.g. "button", "link", "textbox").
	Role string

	// Name is the accessible name of the element (e.g. label text, aria-label).
	Name string

	// Description is the accessible description (e.g. aria-describedby text).
	Description string

	// Value is the current value of the element (e.g. text field content).
	Value string

	// Children contains the direct child nodes of this node in tree order.
	Children []*AXNode

	// BoundingBox is the screen-space rectangle of this element, or nil when
	// the resolver did not fetch layout information for this node.
	BoundingBox *BoundingBox

	// NodeID is the raw CDP accessibility node identifier.
	// This is an internal implementation detail; callers should use SemanticID.
	NodeID string
}

// AXTree is an indexed snapshot of the page accessibility tree.
// The Index provides O(1) lookup by SemanticID.
type AXTree struct {
	// Root is the top-level node of the accessibility tree.
	Root *AXNode

	// Index maps SemanticID → *AXNode for fast lookup.
	Index map[string]*AXNode
}

// MatchResult pairs an AXNode with a confidence score from a resolver search.
type MatchResult struct {
	// Node is the matched accessibility node.
	Node *AXNode

	// Confidence is a value in [0.0, 1.0] indicating match quality:
	//   1.0 — exact role + name match
	//   0.5–0.9 — partial name match
	//   0.3 — role-only match
	Confidence float64
}

// ElementResolver resolves elements in a live browser session.
// Implementations must be safe for single-goroutine use per instance.
//
// Usage pattern:
//
//	tree, err := resolver.Snapshot(ctx)
//	matches, err := resolver.FindByRole("button", "Buy Now")
type ElementResolver interface {
	// Snapshot fetches the current accessibility tree from the browser.
	// The returned AXTree is a point-in-time snapshot; call again for updates.
	// The tree is also stored internally so FindByRole and FindInteractable
	// operate on it without additional browser round-trips.
	Snapshot(ctx context.Context) (*AXTree, error)

	// FindByRole searches the last snapshot for nodes matching role and name.
	// Returns an empty slice when no snapshot has been taken.
	// Confidence scoring: exact match = 1.0, partial name = 0.5–0.9, role-only = 0.3.
	FindByRole(role, name string) ([]MatchResult, error)

	// FindInteractable returns all clickable or typeable nodes from the last snapshot.
	// Returns an empty slice when no snapshot has been taken.
	FindInteractable() ([]*AXNode, error)
}
