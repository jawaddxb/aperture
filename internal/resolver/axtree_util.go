package resolver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/chromedp/cdproto/accessibility"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// maybeReinjectDataURL checks the current page URL and, if it is a plain-text
// (non-base64) data: URL, extracts the HTML body and re-injects it via
// Page.setDocumentContent. This forces Chrome to build a complete AX tree.
func maybeReinjectDataURL(browserCtx context.Context) error {
	var currentURL string
	if err := chromedp.Run(browserCtx, chromedp.Location(&currentURL)); err != nil {
		// Non-fatal: best-effort, just proceed without re-injection.
		return nil
	}

	if !isPlainDataURL(currentURL) {
		return nil
	}

	// Extract the HTML from the data URL.
	htmlContent := extractDataURLContent(currentURL)
	if htmlContent == "" {
		return nil
	}

	return chromedp.Run(browserCtx, chromedp.ActionFunc(func(innerCtx context.Context) error {
		frameTree, err := page.GetFrameTree().Do(innerCtx)
		if err != nil {
			return fmt.Errorf("GetFrameTree: %w", err)
		}
		return page.SetDocumentContent(frameTree.Frame.ID, htmlContent).Do(innerCtx)
	}))
}

// isPlainDataURL reports whether rawURL is a data: URL with a plain (non-base64)
// text/html media type — the kind Chrome mishandles for accessibility.
func isPlainDataURL(rawURL string) bool {
	if !strings.HasPrefix(rawURL, "data:") {
		return false
	}
	// base64 data URLs already work fine; skip them.
	if strings.Contains(rawURL, ";base64,") {
		return false
	}
	return strings.HasPrefix(rawURL, "data:text/html,") || strings.HasPrefix(rawURL, "data:text/html;")
}

// extractDataURLContent extracts the raw content from a plain-text data: URL.
// Returns empty string on any parse error.
func extractDataURLContent(rawURL string) string {
	// Format: data:[<mediatype>][;base64],<data>
	commaIdx := strings.Index(rawURL, ",")
	if commaIdx < 0 {
		return ""
	}
	encoded := rawURL[commaIdx+1:]
	// URL-decode the content.
	decoded, err := url.QueryUnescape(strings.ReplaceAll(encoded, "+", "%2B"))
	if err != nil {
		// Fall back to raw content if unescape fails.
		return encoded
	}
	return decoded
}

// collectNonIgnoredDescendants traverses the CDP tree starting at nodeID and
// appends all nearest non-ignored descendants to out. This ensures that ignored
// intermediate nodes (e.g. role="none") do not break the parent→child wiring.
func collectNonIgnoredDescendants(
	nodeID string,
	cdpMap map[string]*accessibility.Node,
	domainMap map[string]*domain.AXNode,
	out *[]*domain.AXNode,
) {
	n, ok := cdpMap[nodeID]
	if !ok {
		return
	}
	if !n.Ignored {
		if dn, ok := domainMap[nodeID]; ok {
			*out = append(*out, dn)
			return
		}
	}
	// Node is ignored or not in domain map: recurse into its children.
	for _, childID := range n.ChildIDs {
		collectNonIgnoredDescendants(childID.String(), cdpMap, domainMap, out)
	}
}

// buildParentSet returns the set of CDP node IDs that appear as a child of another.
func buildParentSet(raw []*accessibility.Node) map[string]bool {
	set := make(map[string]bool, len(raw))
	for _, n := range raw {
		for _, childID := range n.ChildIDs {
			set[childID.String()] = true
		}
	}
	return set
}

// resolveAncestryPaths computes the ancestry path string for every node.
// It does a BFS from the root(s), accumulating ancestor info.
func resolveAncestryPaths(raw []*accessibility.Node, cdpMap map[string]*accessibility.Node) map[string]string {
	parentSet := buildParentSet(raw)
	paths := make(map[string]string, len(raw))

	type entry struct {
		id        string
		ancestors []ancestorInfo
	}
	queue := make([]entry, 0)

	for _, n := range raw {
		if !parentSet[n.NodeID.String()] {
			queue = append(queue, entry{id: n.NodeID.String(), ancestors: nil})
		}
	}

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]

		paths[cur.id] = buildAncestryPath(cur.ancestors)

		n, ok := cdpMap[cur.id]
		if !ok {
			continue
		}

		role := axNodeRole(n)
		name := axNodeName(n)
		childAncestors := append(append([]ancestorInfo(nil), cur.ancestors...), ancestorInfo{Role: role, Name: name})

		for _, childID := range n.ChildIDs {
			queue = append(queue, entry{id: childID.String(), ancestors: childAncestors})
		}
	}
	return paths
}

// max returns the larger of two ints.
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// axNodeRole extracts the human-readable role string from a CDP node.
func axNodeRole(n *accessibility.Node) string {
	if n.Role == nil {
		return ""
	}
	return rawValueString(n.Role.Value)
}

// axNodeName extracts the accessible name from a CDP node.
func axNodeName(n *accessibility.Node) string {
	if n.Name == nil {
		return ""
	}
	return rawValueString(n.Name.Value)
}

// axNodeDescription extracts the accessible description from a CDP node.
func axNodeDescription(n *accessibility.Node) string {
	if n.Description == nil {
		return ""
	}
	return rawValueString(n.Description.Value)
}

// axNodeValue extracts the current value from a CDP node.
func axNodeValue(n *accessibility.Node) string {
	if n.Value == nil {
		return ""
	}
	return rawValueString(n.Value.Value)
}

// rawValueString JSON-decodes an easyjson.RawMessage ([]byte) that should be a string.
// Returns an empty string on any error or non-string value.
func rawValueString(raw []byte) string {
	if raw == nil {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return s
}
