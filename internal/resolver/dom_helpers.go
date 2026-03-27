// Package resolver contains element resolution strategies for Aperture.
// This file contains pure helper functions for DOMResolver: JS script builders
// and AXNode conversion utilities extracted to keep dom.go within LOC limits.
package resolver

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ApertureHQ/aperture/internal/domain"
)

// domElementToAXNode converts a raw DOM result into a lightweight AXNode.
// SemanticID is derived from tag+text+selector for DOM-sourced nodes.
func domElementToAXNode(el domElement) *domain.AXNode {
	role := el.Role
	if role == "" {
		role = el.Tag
	}
	name := el.Text
	if name == "" {
		name = el.AriaLabel
	}
	if name == "" {
		name = el.Placeholder
	}
	path := el.Selector
	return &domain.AXNode{
		SemanticID: SemanticID(role, name, path),
		Role:       role,
		Name:       name,
		NodeID:     el.Selector,
	}
}

// textMatchConfidence returns confidence for a text-query hit.
// Exact aria-label or visible text match → 0.75; partial → 0.60.
func textMatchConfidence(el domElement, query string) float64 {
	lower := strings.ToLower(query)
	if strings.ToLower(el.Text) == lower ||
		strings.ToLower(el.AriaLabel) == lower ||
		strings.ToLower(el.Placeholder) == lower {
		return 0.75
	}
	return 0.60
}

// buildFindByTextScript returns JS that serialises elements matching text.
func buildFindByTextScript(text string) string {
	escaped := strings.ReplaceAll(text, `"`, `\"`)
	return fmt.Sprintf(`(function(){
  var q = %q.toLowerCase();
  var all = Array.from(document.querySelectorAll('*'));
  var hits = all.filter(function(el){
    var t = (el.innerText||el.textContent||'').trim().toLowerCase();
    var a = (el.getAttribute('aria-label')||'').toLowerCase();
    var p = (el.getAttribute('placeholder')||'').toLowerCase();
    return t===q || a===q || p===q || t.includes(q) || a.includes(q);
  });
  return JSON.stringify(hits.slice(0,20).map(function(el){
    return {
      tag: el.tagName.toLowerCase(),
      type: el.type||'',
      text: (el.innerText||el.textContent||'').trim().substring(0,120),
      ariaLabel: el.getAttribute('aria-label')||'',
      placeholder: el.getAttribute('placeholder')||'',
      role: el.getAttribute('role')||'',
      id: el.id||'',
      name: el.name||'',
      href: el.href||'',
      selector: el.tagName.toLowerCase()+(el.id?'#'+el.id:'')+(el.className?' .'+el.className.split(' ').join('.'):'')
    };
  }));
})()`, escaped)
}

// buildFindBySelectorScript returns JS that serialises elements matching css.
func buildFindBySelectorScript(css string) string {
	escaped := strings.ReplaceAll(css, `"`, `\"`)
	return fmt.Sprintf(`(function(){
  var hits;
  try { hits = Array.from(document.querySelectorAll(%q)); } catch(e){ return JSON.stringify([]); }
  return JSON.stringify(hits.slice(0,20).map(function(el){
    return {
      tag: el.tagName.toLowerCase(),
      type: el.type||'',
      text: (el.innerText||el.textContent||'').trim().substring(0,120),
      ariaLabel: el.getAttribute('aria-label')||'',
      placeholder: el.getAttribute('placeholder')||'',
      role: el.getAttribute('role')||'',
      id: el.id||'',
      name: el.name||'',
      href: el.href||'',
      selector: %q
    };
  }));
})()`, escaped, css)
}

// buildFindByPatternScript returns JS that queries all patterns in one pass.
func buildFindByPatternScript(patterns []string) string {
	patJSON, _ := json.Marshal(patterns)
	return fmt.Sprintf(`(function(){
  var patterns = %s;
  var results = [];
  var seen = {};
  patterns.forEach(function(sel){
    var els;
    try { els = Array.from(document.querySelectorAll(sel)); } catch(e){ return; }
    els.forEach(function(el){
      var key = sel+'|'+el.tagName+(el.id?'#'+el.id:'');
      if(seen[key]) return;
      seen[key] = true;
      results.push({
        tag: el.tagName.toLowerCase(),
        type: el.type||'',
        text: (el.innerText||el.textContent||'').trim().substring(0,120),
        ariaLabel: el.getAttribute('aria-label')||'',
        placeholder: el.getAttribute('placeholder')||'',
        role: el.getAttribute('role')||'',
        id: el.id||'',
        name: el.name||'',
        href: el.href||'',
        selector: sel
      });
    });
  });
  return JSON.stringify(results.slice(0,40));
})()`, string(patJSON))
}
