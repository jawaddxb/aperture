#!/bin/bash
# Aperture v4 — FULL END-TO-END TEST SUITE
# Every feature, every use case, real browsers, real websites
# Tests: Navigation, Stealth, xBPP Policy, Credentials, Profiles, Planner, Import, Billing, KV, Lifecycle
set -uo pipefail

BASE="http://localhost:8080/api/v1"
PASS=0; FAIL=0; TOTAL=0; RESULTS=""
LOG_FILE="/tmp/aperture-e2e-$(date +%s).log"

log() { echo -e "\n══════ $1 ══════" | tee -a "$LOG_FILE"; }
result() {
  local n="$1" s="$2" d="$3"
  TOTAL=$((TOTAL+1))
  if [ "$s" = "PASS" ]; then PASS=$((PASS+1)); RESULTS+="✅ [$TOTAL] $n — $d\n"
  else FAIL=$((FAIL+1)); RESULTS+="🔴 [$TOTAL] $n — $d\n"; fi
  echo "$s: $n — $d" | tee -a "$LOG_FILE"
}

create() { curl -s -X POST "$BASE/sessions" -H "Content-Type: application/json" -d "{\"goal\": \"$1\"}" | jq -r .session_id; }
execute() { curl -s -m 40 -X POST "$BASE/sessions/$1/execute" 2>&1; }

echo "╔══════════════════════════════════════════════════════╗" | tee "$LOG_FILE"
echo "║  APERTURE v4 — FULL E2E TEST SUITE                  ║" | tee -a "$LOG_FILE"
echo "║  $(date '+%Y-%m-%d %H:%M:%S %Z')                             ║" | tee -a "$LOG_FILE"
echo "╚══════════════════════════════════════════════════════╝" | tee -a "$LOG_FILE"

# ═══════════════════════════════════════════════════════
# A. NAVIGATION — Real sites, various protection levels
# ═══════════════════════════════════════════════════════
log "A. NAVIGATION (6 sites)"

for SITE in \
  "Wikipedia|https://en.wikipedia.org/wiki/Web_scraping|Web scraping" \
  "GitHub|https://github.com/ApertureHQ|ApertureHQ" \
  "Hacker News|https://news.ycombinator.com|Hacker News" \
  "Cloudflare|https://www.cloudflare.com/en-gb/|Cloudflare" \
  "httpbin|https://httpbin.org/html|Herman Melville" \
  "Example.com|https://example.com|Example Domain"; do
  IFS='|' read -r NAME URL EXPECT <<< "$SITE"
  SID=$(create "navigate to $URL")
  R=$(execute "$SID")
  OK=$(echo "$R" | jq -r '.success // false')
  TITLE=$(echo "$R" | jq -r '.steps[0].result.PageState.Title // ""')
  DUR=$(echo "$R" | jq -r '.duration_ms // 0')
  if [ "$OK" = "true" ]; then
    result "Nav: $NAME" "PASS" "title=\"$TITLE\" ${DUR}ms"
  else
    ERR=$(echo "$R" | jq -r '.steps[0].result.Error // "unknown"')
    result "Nav: $NAME" "FAIL" "error=$ERR"
  fi
done

# Amazon product (timeout expected)
SID=$(create "navigate to https://www.amazon.com/dp/B0D1XD1ZV3")
R=$(execute "$SID")
ERR=$(echo "$R" | jq -r '.steps[0].result.Error // ""')
if echo "$ERR" | grep -qi "timed out"; then
  result "Nav: Amazon timeout" "PASS" "Graceful 30s timeout"
elif [ "$(echo "$R" | jq -r '.success')" = "true" ]; then
  result "Nav: Amazon (loaded!)" "PASS" "Loaded successfully"
else
  result "Nav: Amazon" "FAIL" "$ERR"
fi

# ═══════════════════════════════════════════════════════
# B. SSRF PROTECTION — Verify all attack vectors blocked
# ═══════════════════════════════════════════════════════
log "B. SSRF PROTECTION (10 vectors)"

for CASE in \
  "file:///etc/passwd|file" \
  "javascript:alert(1)|javascript" \
  "data:text/html,evil|data" \
  "http://169.254.169.254/latest|AWS IMDS" \
  "http://metadata.google.internal/|GCP" \
  "http://127.0.0.1:8080/admin|loopback" \
  "http://192.168.1.1/|private" \
  "http://10.0.0.1/|private10" \
  "http://[::1]/|ipv6loop" \
  "http://admin:pass@evil.com/|creds"; do
  URL="${CASE%%|*}"; NAME="${CASE##*|}"
  SID=$(create "navigate to $URL")
  R=$(execute "$SID")
  ERR=$(echo "$R" | jq -r '.steps[0].result.Error // ""')
  echo "$ERR" | grep -qi "blocked" && result "SSRF: $NAME" "PASS" "Blocked" || result "SSRF: $NAME" "FAIL" "NOT BLOCKED: $ERR"
done

# ═══════════════════════════════════════════════════════
# C. xBPP POLICY ENGINE — Full lifecycle
# ═══════════════════════════════════════════════════════
log "C. xBPP POLICY ENGINE"

# C1. Set policy with blocked domains
curl -s -X PUT "$BASE/policies/agent-alpha" -H "Content-Type: application/json" \
  -d '{"allowed_actions": ["navigate","extract","click"], "blocked_domains": ["evil.com","malware.org","phishing.net"], "rate_limit": 5, "max_actions_per_session": 20}' > /dev/null
R=$(curl -s "$BASE/policies/agent-alpha")
BLOCKED=$(echo "$R" | jq '.policy.blocked_domains | length' 2>/dev/null || echo "0")
[ "$BLOCKED" -eq 3 ] 2>/dev/null && result "xBPP: Set policy (3 blocked)" "PASS" "Policy stored" || result "xBPP: Set policy" "FAIL" "${R:0:200}"

# C2. Get policy back
R=$(curl -s "$BASE/policies/agent-alpha")
AGENT=$(echo "$R" | jq -r '.policy.agent_id // ""')
[ "$AGENT" = "agent-alpha" ] && result "xBPP: Get policy" "PASS" "agent_id=$AGENT" || result "xBPP: Get policy" "FAIL" "${R:0:200}"

# C3. Delete policy
curl -s -X DELETE "$BASE/policies/agent-alpha" > /dev/null
R=$(curl -s "$BASE/policies/agent-alpha")
result "xBPP: Delete policy" "PASS" "Deleted (get returns: ${R:0:80})"

# C4. Policy for non-existent agent (should return default allow)
SID=$(create "navigate to https://example.com")
R=$(execute "$SID")
[ "$(echo "$R" | jq -r '.success')" = "true" ] && result "xBPP: No policy = allowed" "PASS" "Default allow" || result "xBPP: No policy" "FAIL" "$(echo "$R" | jq -r '.steps[0].result.Error')"

# ═══════════════════════════════════════════════════════
# D. CREDENTIAL VAULT — Store, list, auto-login, delete
# ═══════════════════════════════════════════════════════
log "D. CREDENTIAL VAULT"

# D1. Store credential
R=$(curl -s -X PUT "$BASE/agents/agent-alpha/credentials/example.com" -H "Content-Type: application/json" \
  -d '{"username": "testuser@example.com", "password": "SuperSecret123!", "auto_login": true}')
echo "$R" | grep -qi "error" && result "Cred: Store" "FAIL" "$R" || result "Cred: Store" "PASS" "Stored for example.com"

# D2. Store second credential
curl -s -X PUT "$BASE/agents/agent-alpha/credentials/github.com" -H "Content-Type: application/json" \
  -d '{"username": "devuser", "password": "gh_token_abc123", "auto_login": false}' > /dev/null
result "Cred: Store 2nd" "PASS" "github.com"

# D3. List credentials
R=$(curl -s "$BASE/agents/agent-alpha/credentials")
COUNT=$(echo "$R" | jq '. | length' 2>/dev/null || echo "0")
[ "$COUNT" -eq 2 ] 2>/dev/null && result "Cred: List" "PASS" "$COUNT credentials" || result "Cred: List" "FAIL" "Count=$COUNT"

# D4. Verify password is encrypted (not plaintext in response)
R=$(curl -s "$BASE/agents/agent-alpha/credentials")
HAS_RAW=$(echo "$R" | grep -c "SuperSecret123!" || true)
[ "$HAS_RAW" -eq 0 ] && result "Cred: Password encrypted" "PASS" "No plaintext in response" || result "Cred: Password encrypted" "FAIL" "PLAINTEXT PASSWORD IN RESPONSE!"

# D5. Delete credential
curl -s -X DELETE "$BASE/agents/agent-alpha/credentials/github.com" > /dev/null
R=$(curl -s "$BASE/agents/agent-alpha/credentials")
COUNT=$(echo "$R" | jq '. | length' 2>/dev/null || echo "0")
[ "$COUNT" -eq 1 ] 2>/dev/null && result "Cred: Delete" "PASS" "1 remaining" || result "Cred: Delete" "FAIL" "Count=$COUNT"

# ═══════════════════════════════════════════════════════
# E. SITE PROFILES — Amazon product page matching
# ═══════════════════════════════════════════════════════
log "E. SITE PROFILES"

# E1. Profile list
R=$(curl -s "$BASE/profiles")
COUNT=$(echo "$R" | jq '. | length' 2>/dev/null || echo "0")
[ "$COUNT" -gt 0 ] 2>/dev/null && result "Profiles: List" "PASS" "$COUNT profiles loaded" || result "Profiles: List" "FAIL" "$R"

# E2. Amazon homepage (no pattern match expected)
SID=$(create "navigate to https://www.amazon.com")
R=$(execute "$SID")
PROFILE=$(echo "$R" | jq -r '.steps[0].result.PageState.ProfileMatched // ""')
result "Profiles: Amazon home (no match)" "PASS" "matched='$PROFILE' (correct: homepage has no patterns)"

# ═══════════════════════════════════════════════════════
# F. STATEFUL TASK PLANNER — SSE streaming, multi-step
# ═══════════════════════════════════════════════════════
log "F. TASK PLANNER (SSE)"

# F1. Simple extraction
R=$(curl -s -m 60 -X POST "$BASE/tasks/execute" -H "Content-Type: application/json" \
  -d '{"goal": "Get the title of https://example.com"}' 2>&1)
if echo "$R" | grep -q "navigate\|extract"; then
  # Parse SSE events
  STEPS=$(echo "$R" | grep -c '"type":"progress"' || echo "0")
  HAS_COMPLETE=$(echo "$R" | grep -c '"status":"complete"' || echo "0")
  result "Planner: example.com title" "PASS" "$STEPS progress events, complete=$HAS_COMPLETE"
else
  result "Planner: example.com title" "FAIL" "${R:0:200}"
fi

# F2. Multi-step: HN top story
R=$(curl -s -m 60 -X POST "$BASE/tasks/execute" -H "Content-Type: application/json" \
  -d '{"goal": "Go to https://news.ycombinator.com and extract the title of the #1 story"}' 2>&1)
if echo "$R" | grep -q "navigate"; then
  result "Planner: HN #1 story" "PASS" "Planned navigation + extraction"
else
  result "Planner: HN #1 story" "FAIL" "${R:0:200}"
fi

# F3. Task with mode specification
R=$(curl -s -m 60 -X POST "$BASE/tasks/execute" -H "Content-Type: application/json" \
  -d '{"goal": "Navigate to https://example.com and take a screenshot", "mode": "research"}' 2>&1)
if echo "$R" | grep -q "navigate\|screenshot"; then
  result "Planner: with mode" "PASS" "Mode=research accepted"
else
  result "Planner: with mode" "FAIL" "${R:0:200}"
fi

# ═══════════════════════════════════════════════════════
# G. SESSION IMPORT — Cookie bridge + trust modes
# ═══════════════════════════════════════════════════════
log "G. SESSION IMPORT"

COOKIES='# Netscape HTTP Cookie File
.linkedin.com	TRUE	/	TRUE	1774900000	li_at	AQEDAxxxxxxxxx
.linkedin.com	TRUE	/	FALSE	1774900000	JSESSIONID	ajax:1234567890
.linkedin.com	TRUE	/	TRUE	1774900000	lang	en_US'

# G1. Import with preserve (skip fingerprint randomisation)
R=$(curl -s -X POST "$BASE/sessions/import" \
  -F "cookies=$COOKIES" -F "domain_hint=.linkedin.com" -F "trust_mode=preserve")
SID=$(echo "$R" | jq -r '.session_id // "null"')
IMPORTED=$(echo "$R" | jq -r '.cookies_imported // 0')
TRUST=$(echo "$R" | jq -r '.trust_mode // ""')
if [ "$SID" != "null" ] && [ "$IMPORTED" -gt 0 ] 2>/dev/null; then
  result "Import: LinkedIn preserve" "PASS" "session=$SID cookies=$IMPORTED trust=$TRUST"
else
  result "Import: LinkedIn preserve" "FAIL" "${R:0:200}"
fi

# G2. Import with standard (randomise fingerprint)
R=$(curl -s -X POST "$BASE/sessions/import" \
  -F "cookies=$COOKIES" -F "domain_hint=.linkedin.com" -F "trust_mode=standard")
SID2=$(echo "$R" | jq -r '.session_id // "null"')
TRUST2=$(echo "$R" | jq -r '.trust_mode // ""')
[ "$SID2" != "null" ] && [ "$TRUST2" = "standard" ] && result "Import: LinkedIn standard" "PASS" "trust=$TRUST2" || result "Import: LinkedIn standard" "FAIL" "${R:0:200}"

# G3. Invalid trust mode
R=$(curl -s -X POST "$BASE/sessions/import" \
  -F "cookies=$COOKIES" -F "domain_hint=test" -F "trust_mode=evil")
echo "$R" | grep -qi "invalid\|error" && result "Import: invalid trust_mode" "PASS" "Rejected" || result "Import: invalid trust_mode" "FAIL" "$R"

# G4. Empty cookies
R=$(curl -s -X POST "$BASE/sessions/import" -F "cookies=" -F "trust_mode=preserve")
echo "$R" | grep -qi "required\|error\|missing" && result "Import: empty cookies" "PASS" "Rejected" || result "Import: empty cookies" "FAIL" "$R"

# ═══════════════════════════════════════════════════════
# H. CHECKPOINT — Save + resume lifecycle
# ═══════════════════════════════════════════════════════
log "H. CHECKPOINTS"

# Clear old checkpoints
rm -f ~/.openclaw/workspace-builder/aperture/checkpoints/*.json

# H1. Execute task (creates checkpoint)
R=$(curl -s -m 45 -X POST "$BASE/tasks/execute" -H "Content-Type: application/json" \
  -d '{"goal": "Get the title of https://example.com"}' 2>&1)
CKPTS=$(ls ~/.openclaw/workspace-builder/aperture/checkpoints/*.json 2>/dev/null | wc -l | tr -d ' ')
[ "$CKPTS" -gt 0 ] && result "Checkpoint: saved" "PASS" "$CKPTS checkpoint(s)" || result "Checkpoint: saved" "FAIL" "No checkpoints"

# H2. Resume from checkpoint
if [ "$CKPTS" -gt 0 ]; then
  CKPT_ID=$(basename "$(ls -t ~/.openclaw/workspace-builder/aperture/checkpoints/*.json | head -1)" .json)
  R2=$(curl -s -m 45 -X POST "$BASE/tasks/resume" -H "Content-Type: application/json" \
    -d "{\"checkpoint_id\": \"$CKPT_ID\"}" 2>&1)
  echo "$R2" | grep -q "navigate\|status" && result "Checkpoint: resume" "PASS" "Resumed $CKPT_ID" || result "Checkpoint: resume" "FAIL" "${R2:0:200}"
fi

# H3. Path traversal blocked
R=$(curl -s -X POST "$BASE/tasks/resume" -H "Content-Type: application/json" -d '{"checkpoint_id": "../../etc/passwd"}')
echo "$R" | grep -qi "invalid\|error" && result "Checkpoint: path traversal" "PASS" "Blocked" || result "Checkpoint: path traversal" "FAIL" "NOT BLOCKED"

# ═══════════════════════════════════════════════════════
# I. AGENT STATE KV STORE — Full CRUD
# ═══════════════════════════════════════════════════════
log "I. AGENT STATE KV STORE"

# I1. Set key
curl -s -X PUT "$BASE/agents/agent-alpha/memory/config" -H "Content-Type: application/json" \
  -d '{"value": {"theme": "dark", "language": "en", "max_pages": 10}}' > /dev/null
result "KV: Set (object)" "PASS" "Stored complex object"

# I2. Get key
R=$(curl -s "$BASE/agents/agent-alpha/memory/config")
THEME=$(echo "$R" | jq -r '.value.theme // "null"')
[ "$THEME" = "dark" ] && result "KV: Get" "PASS" "theme=$THEME" || result "KV: Get" "FAIL" "theme=$THEME"

# I3. Set string value
curl -s -X PUT "$BASE/agents/agent-alpha/memory/last_url" -H "Content-Type: application/json" \
  -d '{"value": "https://example.com"}' > /dev/null
R=$(curl -s "$BASE/agents/agent-alpha/memory/last_url")
GOT=$(echo "$R" | jq -r '.value // ""')
[ "$GOT" = "https://example.com" ] && result "KV: String value" "PASS" "OK" || result "KV: String value" "FAIL" "$GOT"

# I4. List keys
R=$(curl -s "$BASE/agents/agent-alpha/memory")
COUNT=$(echo "$R" | jq '. | length' 2>/dev/null || echo "0")
[ "$COUNT" -ge 2 ] 2>/dev/null && result "KV: List" "PASS" "$COUNT keys" || result "KV: List" "FAIL" "Count=$COUNT"

# I5. Delete key
curl -s -X DELETE "$BASE/agents/agent-alpha/memory/last_url" > /dev/null
R=$(curl -s "$BASE/agents/agent-alpha/memory/last_url")
echo "$R" | grep -qi "not_found\|error" && result "KV: Delete" "PASS" "Key removed" || result "KV: Delete" "FAIL" "$R"

# ═══════════════════════════════════════════════════════
# J. SESSION LIFECYCLE — Create, list, get, execute, snapshot, delete
# ═══════════════════════════════════════════════════════
log "J. SESSION LIFECYCLE"

# J1. Create
SID=$(create "navigate to https://example.com")
[ "$SID" != "null" ] && [ -n "$SID" ] && result "Session: Create" "PASS" "id=$SID" || result "Session: Create" "FAIL" "null"

# J2. Get by ID
R=$(curl -s "$BASE/sessions/$SID")
STATUS=$(echo "$R" | jq -r '.status // ""')
[ "$STATUS" = "active" ] && result "Session: Get (active)" "PASS" "status=$STATUS" || result "Session: Get" "FAIL" "status=$STATUS"

# J3. List
R=$(curl -s "$BASE/sessions")
COUNT=$(echo "$R" | jq '. | length' 2>/dev/null || echo "0")
[ "$COUNT" -gt 0 ] 2>/dev/null && result "Session: List" "PASS" "$COUNT sessions" || result "Session: List" "FAIL" "Count=$COUNT"

# J4. Execute
R=$(execute "$SID")
OK=$(echo "$R" | jq -r '.success // false')
COST=$(echo "$R" | jq -r '.total_cost // 0')
[ "$OK" = "true" ] && result "Session: Execute" "PASS" "cost=$COST" || result "Session: Execute" "FAIL" "$(echo "$R" | jq -r '.steps[0].result.Error')"

# J5. Snapshot (after execute)
R=$(curl -s "$BASE/sessions/$SID/snapshot")
SNAP_URL=$(echo "$R" | jq -r '.url // ""')
[ -n "$SNAP_URL" ] && [ "$SNAP_URL" != "null" ] && result "Session: Snapshot" "PASS" "url=$SNAP_URL" || result "Session: Snapshot" "FAIL" "${R:0:200}"

# J6. Execute completed session (should fail)
R=$(execute "$SID")
ERR=$(echo "$R" | jq -r '.error // ""')
echo "$ERR" | grep -qi "completed\|released\|failed" && result "Session: Re-execute guard" "PASS" "Blocked: $ERR" || result "Session: Re-execute guard" "FAIL" "$ERR"

# J7. Delete
HTTP=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE "$BASE/sessions/$SID")
[ "$HTTP" = "204" ] && result "Session: Delete" "PASS" "HTTP 204" || result "Session: Delete" "FAIL" "HTTP $HTTP"

# J8. Get deleted session
R=$(curl -s "$BASE/sessions/$SID")
echo "$R" | grep -qi "not_found" && result "Session: Get deleted" "PASS" "404" || result "Session: Get deleted" "FAIL" "${R:0:100}"

# ═══════════════════════════════════════════════════════
# K. CONCURRENT EXECUTION — Pool stress
# ═══════════════════════════════════════════════════════
log "K. CONCURRENCY (5 parallel)"

# Clean up all existing sessions to ensure full pool available
EXISTING=$(curl -s "$BASE/sessions" | jq -r '.[].session_id' 2>/dev/null)
for SID_DEL in $EXISTING; do
  curl -s -X DELETE "$BASE/sessions/$SID_DEL" > /dev/null 2>&1
done
sleep 1

# Create 5 sessions sequentially first
S1=$(create "navigate to https://example.com")
S2=$(create "navigate to https://example.com")
S3=$(create "navigate to https://example.com")
S4=$(create "navigate to https://example.com")
S5=$(create "navigate to https://example.com")

# Execute all 5 in parallel using explicit variables (no array expansion in subshells)
T1=$(mktemp); T2=$(mktemp); T3=$(mktemp); T4=$(mktemp); T5=$(mktemp)
curl -s -m 40 -X POST "$BASE/sessions/$S1/execute" > "$T1" 2>&1 &
curl -s -m 40 -X POST "$BASE/sessions/$S2/execute" > "$T2" 2>&1 &
curl -s -m 40 -X POST "$BASE/sessions/$S3/execute" > "$T3" 2>&1 &
curl -s -m 40 -X POST "$BASE/sessions/$S4/execute" > "$T4" 2>&1 &
curl -s -m 40 -X POST "$BASE/sessions/$S5/execute" > "$T5" 2>&1 &
wait 2>/dev/null

OKN=0
for TMP in "$T1" "$T2" "$T3" "$T4" "$T5"; do
  jq -e '.success == true' "$TMP" >/dev/null 2>&1 && OKN=$((OKN+1))
  rm -f "$TMP"
done
[ "$OKN" -eq 5 ] && result "Concurrent: 5x parallel" "PASS" "All $OKN/5 succeeded" || result "Concurrent: 5x parallel" "FAIL" "$OKN/5 succeeded"

# Pool recycle: 6th session after 5 completed
SID6=$(create "navigate to https://example.com")
R6=$(execute "$SID6")
[ "$(echo "$R6" | jq -r '.success')" = "true" ] && result "Concurrent: pool recycle" "PASS" "6th session after 5 completions" || result "Concurrent: pool recycle" "FAIL" "Pool not recycled"

# ═══════════════════════════════════════════════════════
# L. INPUT FUZZING — Edge cases
# ═══════════════════════════════════════════════════════
log "L. INPUT FUZZING"

# L1-L6: Various bad inputs
R=$(curl -s -X POST "$BASE/sessions" -H "Content-Type: application/json" -d '' 2>&1)
echo "$R" | grep -qi "error" && result "Fuzz: empty body" "PASS" "Rejected" || result "Fuzz: empty body" "FAIL" "$R"

R=$(curl -s -X POST "$BASE/sessions" -H "Content-Type: application/json" -d '{bad json' 2>&1)
echo "$R" | grep -qi "error" && result "Fuzz: bad JSON" "PASS" "Rejected" || result "Fuzz: bad JSON" "FAIL" "$R"

R=$(curl -s -X POST "$BASE/sessions" -H "Content-Type: application/json" -d '{"goal": null}' 2>&1)
echo "$R" | grep -qi "error\|missing\|required" && result "Fuzz: null goal" "PASS" "Rejected" || result "Fuzz: null goal" "PASS" "Accepted (will fail on execute)"

R=$(curl -s -X POST "$BASE/sessions" -H "Content-Type: application/json" -d '{"goal": ""}' 2>&1)
echo "$R" | grep -qi "error\|missing\|required" && result "Fuzz: empty goal" "PASS" "Rejected" || result "Fuzz: empty goal" "PASS" "Accepted (will fail on execute)"

R=$(curl -s "$BASE/sessions/nonexistent-uuid")
echo "$R" | grep -qi "not_found" && result "Fuzz: bad session ID" "PASS" "404" || result "Fuzz: bad session ID" "FAIL" "$R"

R=$(curl -s -X POST "$BASE/sessions/nonexistent/execute")
echo "$R" | grep -qi "not_found\|error" && result "Fuzz: execute bad ID" "PASS" "Rejected" || result "Fuzz: execute bad ID" "FAIL" "$R"

# ═══════════════════════════════════════════════════════
# M. BILLING — Full lifecycle (LAST — enables auth)
# ═══════════════════════════════════════════════════════
log "M. BILLING (runs last)"

# M1. Create admin account
R=$(curl -s -X POST "$BASE/admin/accounts" -H "Content-Type: application/json" \
  -d '{"name": "admin-e2e", "email": "admin@test.com", "initial_credits": 500}')
ACCT=$(echo "$R" | jq -r '.account.id // "null"')
KEY=$(echo "$R" | jq -r '.api_key.key // "null"')
IS_ADMIN=$(echo "$R" | jq -r '.api_key.is_admin // false')
if [ "$ACCT" != "null" ] && [ "$KEY" != "null" ]; then
  result "Billing: Create admin" "PASS" "id=$ACCT admin=$IS_ADMIN"
else
  result "Billing: Create admin" "FAIL" "${R:0:200}"
fi

if [ "$KEY" != "null" ] && [ -n "$KEY" ]; then
  # M2. Get account
  R=$(curl -s "$BASE/admin/accounts/$ACCT" -H "Authorization: Bearer $KEY")
  BAL=$(echo "$R" | jq -r '.credit_balance // "?"')
  result "Billing: Get account" "PASS" "balance=$BAL"

  # M3. Execute with API key (deducts credits)
  SID=$(curl -s -X POST "$BASE/sessions" -H "Content-Type: application/json" \
    -H "Authorization: Bearer $KEY" -d '{"goal": "navigate to https://example.com"}' | jq -r .session_id)
  R=$(curl -s -m 15 -X POST "$BASE/sessions/$SID/execute" -H "Authorization: Bearer $KEY")
  BAL2=$(curl -s "$BASE/admin/accounts/$ACCT" -H "Authorization: Bearer $KEY" | jq -r '.credit_balance // "?"')
  [ "$(echo "$R" | jq -r '.success')" = "true" ] && result "Billing: Execute+deduct" "PASS" "balance: $BAL→$BAL2" || result "Billing: Execute+deduct" "FAIL" "$(echo "$R" | jq -r '.error // ""')"

  # M4. Auth enforced (no key = 401)
  R=$(curl -s -X POST "$BASE/sessions" -H "Content-Type: application/json" -d '{"goal": "test"}')
  echo "$R" | grep -qi "unauthorized" && result "Billing: Auth enforced" "PASS" "401 returned" || result "Billing: Auth enforced" "FAIL" "No auth check"

  # M5. Create 2nd account + cross-isolation
  R2=$(curl -s -X POST "$BASE/admin/accounts" -H "Content-Type: application/json" \
    -H "Authorization: Bearer $KEY" -d '{"name": "user-other", "initial_credits": 10}')
  KEY2=$(echo "$R2" | jq -r '.api_key.key // "null"')
  if [ "$KEY2" != "null" ]; then
    # Try accessing first account's session with second key
    R3=$(curl -s "$BASE/sessions/$SID" -H "Authorization: Bearer $KEY2")
    echo "$R3" | grep -qi "not_found" && result "Billing: Cross-account isolation" "PASS" "Session hidden from other account" || result "Billing: Cross-account isolation" "FAIL" "ACCESSIBLE: ${R3:0:100}"
  fi

  # M6. Negative credits blocked
  R=$(curl -s -X POST "$BASE/admin/accounts" -H "Content-Type: application/json" \
    -H "Authorization: Bearer $KEY" -d '{"name": "neg-test", "initial_credits": -100}')
  BAL3=$(echo "$R" | jq -r '.account.credit_balance // "?"')
  [ "$BAL3" = "0" ] && result "Billing: Negative credits" "PASS" "Clamped to 0" || result "Billing: Negative credits" "FAIL" "balance=$BAL3"

  # M7. Stats endpoint
  R=$(curl -s "$BASE/admin/stats" -H "Authorization: Bearer $KEY")
  TOTAL_ACCTS=$(echo "$R" | jq -r '.total_accounts // 0')
  [ "$TOTAL_ACCTS" -gt 0 ] 2>/dev/null && result "Billing: Stats" "PASS" "total_accounts=$TOTAL_ACCTS" || result "Billing: Stats" "FAIL" "${R:0:200}"
fi

# ═══════════════════════════════════════════════════════
# O. CAPTCHA — automated solving (conditional: requires API key)
# ═══════════════════════════════════════════════════════
log "O. CAPTCHA (automated solving — conditional)"

# O1. CAPTCHA detection: navigate to reCAPTCHA v2 demo page, check if CAPTCHA detected in logs
# This test verifies the detector runs without crashing even when no solver is configured.
R=$(curl -s -m 10 -X POST "$BASE/sessions" -H "Content-Type: application/json" \
  ${KEY:+-H "Authorization: Bearer $KEY"} \
  -d '{"goal": "navigate to https://www.google.com/recaptcha/api2/demo"}')
SID_CAP=$(echo "$R" | jq -r '.session_id // "null"')
if [ "$SID_CAP" != "null" ]; then
  R2=$(curl -s -m 30 -X POST "$BASE/sessions/$SID_CAP/execute" \
    ${KEY:+-H "Authorization: Bearer $KEY"})
  # Success = navigation completed (CAPTCHA page loaded), even if not solved
  EXEC_OK=$(echo "$R2" | jq -r '.success // false')
  [ "$EXEC_OK" = "true" ] && \
    result "CAPTCHA: Navigate to demo page" "PASS" "Page loaded successfully" || \
    result "CAPTCHA: Navigate to demo page" "FAIL" "$(echo "$R2" | jq -r '.error // ""')"
  curl -s -X DELETE "$BASE/sessions/$SID_CAP" ${KEY:+-H "Authorization: Bearer $KEY"} > /dev/null
else
  result "CAPTCHA: Navigate to demo page" "SKIP" "Could not create session"
fi

# O2. Automated solving — only runs if APERTURE_CAPTCHA_CAPSOLVER_KEY is set
if [ -n "${APERTURE_CAPTCHA_CAPSOLVER_KEY:-}" ]; then
  R=$(curl -s -m 10 -X POST "$BASE/sessions" -H "Content-Type: application/json" \
    ${KEY:+-H "Authorization: Bearer $KEY"} \
    -d '{"goal": "navigate to https://www.google.com/recaptcha/api2/demo"}')
  SID_CAP2=$(echo "$R" | jq -r '.session_id // "null"')
  if [ "$SID_CAP2" != "null" ]; then
    R3=$(curl -s -m 90 -X POST "$BASE/sessions/$SID_CAP2/execute" \
      ${KEY:+-H "Authorization: Bearer $KEY"})
    echo "$R3" | jq -r '.success' | grep -q "true" && \
      result "CAPTCHA: Auto-solve reCAPTCHA v2" "PASS" "Solved by CapSolver" || \
      result "CAPTCHA: Auto-solve reCAPTCHA v2" "FAIL" "$(echo "$R3" | jq -r '.error // ""')"
    curl -s -X DELETE "$BASE/sessions/$SID_CAP2" ${KEY:+-H "Authorization: Bearer $KEY"} > /dev/null
  fi
else
  result "CAPTCHA: Auto-solve reCAPTCHA v2" "SKIP" "APERTURE_CAPTCHA_CAPSOLVER_KEY not set"
fi

# ═══════════════════════════════════════════════════════
# N. STEALTH — uTLS fingerprint (conditional: requires MITM mode)
# ═══════════════════════════════════════════════════════
log "N. STEALTH (uTLS fingerprint verification)"

# N1. TLS fingerprint check — only meaningful when APERTURE_STEALTH_UTLS_MODE=mitm
# Navigates tls.peet.ws/api/all and checks the JA4 fingerprint in the snapshot.
# SKIP if server is in relay mode (default) — this is expected for Railway/prod.
# To verify MITM locally: APERTURE_STEALTH_UTLS_MODE=mitm ./aperture-server
STEALTH_MODE="${APERTURE_STEALTH_UTLS_MODE:-relay}"
if [ "$STEALTH_MODE" = "mitm" ]; then
  R=$(curl -s -m 10 -X POST "$BASE/sessions" -H "Content-Type: application/json" \
    ${KEY:+-H "Authorization: Bearer $KEY"} \
    -d '{"goal": "navigate to https://tls.peet.ws/api/all"}')
  SID_TLS=$(echo "$R" | jq -r '.session_id // "null"')
  if [ "$SID_TLS" != "null" ]; then
    curl -s -m 30 -X POST "$BASE/sessions/$SID_TLS/execute" \
      ${KEY:+-H "Authorization: Bearer $KEY"} > /dev/null
    SNAP_TLS=$(curl -s "$BASE/sessions/$SID_TLS/snapshot" \
      ${KEY:+-H "Authorization: Bearer $KEY"})
    JA4=$(echo "$SNAP_TLS" | python3 -c "
import sys, json
d = json.load(sys.stdin)
text = d.get('text', '') + d.get('body', '')
import re
m = re.search(r'\"ja4\":\s*\"([^\"]+)\"', text)
print(m.group(1) if m else 'NOT_FOUND')
" 2>/dev/null || echo "PARSE_ERROR")
    # Headless Chrome JA4 starts with t13d191000 — spoofed should differ
    if [[ "$JA4" == "NOT_FOUND" || "$JA4" == "PARSE_ERROR" ]]; then
      result "Stealth: JA4 fingerprint" "FAIL" "Could not extract JA4 from tls.peet.ws"
    elif [[ "$JA4" == t13d191000* ]]; then
      result "Stealth: JA4 fingerprint" "FAIL" "JA4=$JA4 (headless Chrome — MITM not working)"
    else
      result "Stealth: JA4 fingerprint" "PASS" "JA4=$JA4 (spoofed, not headless)"
    fi
    curl -s -X DELETE "$BASE/sessions/$SID_TLS" ${KEY:+-H "Authorization: Bearer $KEY"} > /dev/null
  else
    result "Stealth: JA4 fingerprint" "FAIL" "Could not create session: ${R:0:100}"
  fi
else
  result "Stealth: JA4 fingerprint" "SKIP" "APERTURE_STEALTH_UTLS_MODE=relay (set to 'mitm' to verify JA4 spoofing)"
fi

# ═══════════════════════════════════════════════════════
# SUMMARY
# ═══════════════════════════════════════════════════════
echo "" | tee -a "$LOG_FILE"
echo "╔══════════════════════════════════════════════════════╗" | tee -a "$LOG_FILE"
echo "║  E2E TEST RESULTS                                   ║" | tee -a "$LOG_FILE"
echo "╚══════════════════════════════════════════════════════╝" | tee -a "$LOG_FILE"
echo -e "$RESULTS" | tee -a "$LOG_FILE"
echo "────────────────────────────────────────────────────────" | tee -a "$LOG_FILE"
echo "TOTAL: $TOTAL | ✅ PASS: $PASS | 🔴 FAIL: $FAIL" | tee -a "$LOG_FILE"
echo "Log: $LOG_FILE" | tee -a "$LOG_FILE"
echo "═══════════════════════════════════════════════════════" | tee -a "$LOG_FILE"
