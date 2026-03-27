# Aperture Phase 3: OpenClaw Integration Spec

**Status:** Spec READY for build  
**Date:** 2026-03-27  
**Build mode:** Manual (sub-agent failed — see AGENTS.md for recovery)

---

## Overview

Integrate Aperture (browser automation engine, Phases 1-2 complete) as OpenClaw's native browser automation backend. Replace browser tool's internal Chrome/Playwright with Aperture's CDP pool + executors.

## Deliverables

### 1. MCP Server (packages/mcp-server/)

**Language:** TypeScript/Node.js  
**Transport:** stdio (OpenClaw native)  
**Interface:** MCP 0.1 protocol

**Tools exposed:**
- `execute` — Run an executor (name, args, timeout)
- `screenshot` — Capture viewport
- `navigation` — navigate(url), goBack, goForward, reload
- `pool_status` — Get current browser pool health

**Config file:** `~/.openclaw/plugins/aperture-mcp.json`
```json
{
  "type": "mcp",
  "transport": "stdio",
  "command": "node",
  "args": ["~/.openclaw/workspace-builder/aperture/packages/mcp-server/dist/index.js"],
  "env": {
    "APERTURE_POOL_SIZE": "5",
    "APERTURE_TIMEOUT": "30000"
  }
}
```

### 2. Plugin Registration

**File:** `~/.openclaw/plugins/aperture-mcp.json` (created above)

**Integration points:**
- OpenClaw CLI tool `browser` routes to Aperture executors
- Session state tracks active browser pool
- Tool results include screenshots + execution metadata

### 3. E2E Test

**File:** `aperture/tests/e2e-openclaw.test.ts`

**Test flow:**
1. Start OpenClaw gateway with aperture-mcp plugin
2. Call `mcporter list` — verify 4 Aperture tools visible
3. Execute `navigate("https://example.com")` via MCP
4. Capture screenshot, verify DOM loaded
5. Execute `goBack()`, verify navigation history
6. Pool status → confirm 1 active session, 4 idle

**Success criteria:**
- ✅ All 5 E2E tests pass
- ✅ 0 TypeScript errors
- ✅ mcporter introspection shows Aperture commands

### 4. Documentation

**File:** `INTEGRATION.md`

**Sections:**
- Architecture (Aperture → MCP → OpenClaw CLI)
- Setup (plugin config, env vars)
- Tool reference (4 commands + examples)
- Troubleshooting (pool exhaustion, timeout tuning)

---

## Implementation Order

1. **MCP Server scaffold** (2h)
   - stdio transport
   - Tool definitions + handlers
   - Pool lifecycle management
   - Error handling + recovery

2. **OpenClaw plugin wiring** (1h)
   - Config file creation
   - CLI tool bridge
   - Session state integration

3. **E2E tests** (1.5h)
   - Gateway startup + plugin loading
   - 5 test cases (navigate, screenshot, history, pool status, cleanup)
   - CI/CD integration

4. **Docs + commit** (0.5h)
   - INTEGRATION.md
   - Update TASKS.md
   - Commit: `feat: Aperture Phase 3 OpenClaw integration`

**Total effort:** 5 hours  
**Team:** JarvisForge (sub-agent) or manual build

---

## Known Constraints

- **Pool size:** Default 5 concurrent browsers (config `APERTURE_POOL_SIZE`)
- **Timeout:** 30s default (tune via `APERTURE_TIMEOUT`)
- **Snapshot conflict:** Session screenshots may conflict with Aperture screenshots — use namespacing
- **Cleanup:** On OpenClaw shutdown, drain pool gracefully (don't force-kill)

---

## Success Checklist

- [ ] MCP server compiled, no TS errors
- [ ] aperture-mcp.json plugin registered
- [ ] `mcporter list` shows 4 Aperture tools
- [ ] E2E tests pass (5/5)
- [ ] Documentation complete
- [ ] TASKS.md updated to ✅ Completed
- [ ] Commit pushed to main

---

## Next After Phase 3

**Phase 4 (optional):** Agent pool integration  
- Aperture pool as shared resource across multiple OpenClaw agents
- Rate limiting per agent
- Memory-efficient browser reuse

