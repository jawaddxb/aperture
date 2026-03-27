package resolver

import (
	"context"
	"fmt"

	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/chromedp/cdproto/accessibility"
	"github.com/chromedp/chromedp"
)

// interactableRoles is the set of WAI-ARIA roles whose nodes are considered
// clickable or typeable for the purposes of FindInteractable.
var interactableRoles = map[string]bool{
	"button":           true,
	"link":             true,
	"textbox":          true,
	"checkbox":         true,
	"radio":            true,
	"combobox":         true,
	"listbox":          true,
	"menuitem":         true,
	"menuitemcheckbox": true,
	"menuitemradio":    true,
	"option":           true,
	"searchbox":        true,
	"slider":           true,
	"spinbutton":       true,
	"switch":           true,
	"tab":              true,
}

// AXTreeResolver implements domain.ElementResolver using Chrome DevTools Protocol.
// It holds a single browser context and caches the last captured snapshot so
// that FindByRole and FindInteractable work without additional CDP round-trips.
type AXTreeResolver struct {
	browserCtx context.Context
	lastTree   *domain.AXTree
}

// NewAXTreeResolver creates an AXTreeResolver for the given browser context.
// browserCtx must be a chromedp tab context obtained from a domain.BrowserInstance.
func NewAXTreeResolver(browserCtx context.Context) *AXTreeResolver {
	return &AXTreeResolver{browserCtx: browserCtx}
}

// Snapshot fetches the full accessibility tree from the browser via CDP,
// indexes every node with a stable semantic ID, and caches the result.
// Implements domain.ElementResolver.
func (r *AXTreeResolver) Snapshot(ctx context.Context) (*domain.AXTree, error) {
	nodes, err := fetchAXNodes(ctx, r.browserCtx)
	if err != nil {
		return nil, fmt.Errorf("axtree snapshot: %w", err)
	}

	tree := buildTree(nodes)
	r.lastTree = tree
	return tree, nil
}

// FindByRole searches the cached snapshot for nodes matching role and name.
// Returns an empty slice when no snapshot has been taken.
// Implements domain.ElementResolver.
func (r *AXTreeResolver) FindByRole(role, name string) ([]domain.MatchResult, error) {
	if r.lastTree == nil {
		return nil, nil
	}
	return searchByRole(r.lastTree.Root, role, name), nil
}

// FindInteractable returns all clickable or typeable nodes from the cached snapshot.
// Returns an empty slice when no snapshot has been taken.
// Implements domain.ElementResolver.
func (r *AXTreeResolver) FindInteractable() ([]*domain.AXNode, error) {
	if r.lastTree == nil {
		return nil, nil
	}
	var results []*domain.AXNode
	collectInteractable(r.lastTree.Root, &results)
	return results, nil
}

// fetchAXNodes calls CDP to retrieve all accessibility nodes for the current page.
// It enables the Accessibility domain first, which is required for GetFullAXTree
// to return the complete tree.
//
// Chrome restricts accessibility tree access for plain-text data: URLs (e.g.
// data:text/html,...), returning only a partial tree. When the current page is
// a plain-text data: URL, this function re-injects the HTML via
// Page.setDocumentContent so Chrome builds a full AX tree.
func fetchAXNodes(ctx, browserCtx context.Context) ([]*accessibility.Node, error) {
	// Step 1: enable accessibility domain.
	if err := chromedp.Run(browserCtx, chromedp.ActionFunc(func(innerCtx context.Context) error {
		return accessibility.Enable().Do(innerCtx)
	})); err != nil {
		return nil, fmt.Errorf("accessibility.Enable: %w", err)
	}

	// Step 2: if the page is a plain-text data: URL, re-inject the HTML.
	// Chrome 140+ returns a truncated AX tree for plain data: URLs but works
	// correctly when content is set via Page.setDocumentContent.
	if err := maybeReinjectDataURL(browserCtx); err != nil {
		return nil, fmt.Errorf("reinjectDataURL: %w", err)
	}

	// Step 3: capture the full AX tree.
	var nodes []*accessibility.Node
	if err := chromedp.Run(browserCtx, chromedp.ActionFunc(func(innerCtx context.Context) error {
		var err error
		nodes, err = accessibility.GetFullAXTree().Do(innerCtx)
		return err
	})); err != nil {
		return nil, fmt.Errorf("GetFullAXTree: %w", err)
	}
	return nodes, nil
}

// buildTree converts a flat CDP node list into a rooted AXTree with semantic IDs.
// The CDP list uses parent/child ID references; this builds the pointer tree.
func buildTree(raw []*accessibility.Node) *domain.AXTree {
	cdpMap := buildCDPIndex(raw)
	ancestryPaths := resolveAncestryPaths(raw, cdpMap)
	domainMap := buildDomainMap(raw, cdpMap, ancestryPaths)
	wireChildren(raw, cdpMap, domainMap)
	root, index := buildTreeIndex(raw, domainMap)
	return &domain.AXTree{Root: root, Index: index}
}

// buildCDPIndex builds a map of nodeID → *accessibility.Node from the flat list.
func buildCDPIndex(raw []*accessibility.Node) map[string]*accessibility.Node {
	cdpMap := make(map[string]*accessibility.Node, len(raw))
	for _, n := range raw {
		cdpMap[n.NodeID.String()] = n
	}
	return cdpMap
}

// buildDomainMap converts each non-ignored CDP node into a *domain.AXNode.
// ancestryPaths provides the pre-computed ancestry path for each node ID.
func buildDomainMap(
	raw []*accessibility.Node,
	cdpMap map[string]*accessibility.Node,
	ancestryPaths map[string]string,
) map[string]*domain.AXNode {
	domainMap := make(map[string]*domain.AXNode, len(raw))
	for _, n := range raw {
		if n.Ignored {
			continue
		}
		role := axNodeRole(n)
		name := axNodeName(n)
		path := ancestryPaths[n.NodeID.String()]
		dn := &domain.AXNode{
			SemanticID:  SemanticID(role, name, path),
			Role:        role,
			Name:        name,
			Description: axNodeDescription(n),
			Value:       axNodeValue(n),
			NodeID:      n.NodeID.String(),
		}
		domainMap[dn.NodeID] = dn
	}
	return domainMap
}

// wireChildren wires parent → children pointers in the domain map.
// Ignored nodes may appear between a parent and its logical children in the
// CDP tree; collectNonIgnoredDescendants skips them so there are no gaps.
func wireChildren(
	raw []*accessibility.Node,
	cdpMap map[string]*accessibility.Node,
	domainMap map[string]*domain.AXNode,
) {
	for _, n := range raw {
		if n.Ignored {
			continue
		}
		parent, ok := domainMap[n.NodeID.String()]
		if !ok {
			continue
		}
		for _, childID := range n.ChildIDs {
			collectNonIgnoredDescendants(childID.String(), cdpMap, domainMap, &parent.Children)
		}
	}
}

// buildTreeIndex builds the semantic-ID index and finds the root node.
// The root is the domain node whose CDP ID is not a child of any other node.
func buildTreeIndex(
	raw []*accessibility.Node,
	domainMap map[string]*domain.AXNode,
) (*domain.AXNode, map[string]*domain.AXNode) {
	index := make(map[string]*domain.AXNode, len(domainMap))
	var root *domain.AXNode
	parentSet := buildParentSet(raw)
	for id, dn := range domainMap {
		index[dn.SemanticID] = dn
		if !parentSet[id] {
			root = dn
		}
	}
	return root, index
}
