#!/bin/bash
# Aperture v4 — Full Adversarial Test Battery v3
# Order: navigation → planner → import → checkpoint → KV → credentials → profiles → concurrent → snapshot → policy → billing (last, creates accounts)
set -uo pipefail

BASE="http://localhost:8080/api/v1"
PASS=0; FAIL=0; RESULTS=""

log() { echo -e "\n=== $1 ===" ; }
result() {
  local n="$1" s="$2" d="$3" t="$4"
  if [ "$s" = "PASS" ]; then PASS=$((PASS+1)); RESULTS+="✅ $n ($t) — $d\n"
  else FAIL=$((FAIL+1)); RESULTS+="🔴 $n ($t) — $d\n"; fi
  echo "$s: $n ($t) — $d"
}

create_session() {
  curl -s -X POST "$BASE/sessions" -H "Content-Type: application/json" -d "{\"goal\": \"$1\"}" | jq -r .session_id
}

execute_session() {
  curl -s -m 40 -X POST "$BASE/sessions/$1/execute" 2>&1
}

# ──────────────────────────────────────────────────────────────
# 1. NAVIGATION: Multi-site (proxy + stealth)
# ──────────────────────────────────────────────────────────────
for SITE in \
  "Wikipedia|https://en.wikipedia.org/wiki/Web_scraping" \
  "GitHub|https://github.com/chromedp/chromedp" \
  "Hacker News|https://news.ycombinator.com" \
  "Cloudflare|https://www.cloudflare.com/en-gb/" \
  "httpbin|https://httpbin.org/html"; do
  NAME="${SITE%%|*}"; URL="${SITE##*|}"
  log "NAV: $NAME"
  T=$(date +%s)
  SID=$(create_session "navigate to $URL")
  R=$(execute_session "$SID")
  DUR="$(($(date +%s) - T))s"
  OK=$(echo "$R" | jq -r '.success // false')
  TITLE=$(echo "$R" | jq -r '.steps[0].result.PageState.Title // "null"')
  ERR=$(echo "$R" | jq -r '.steps[0].result.Error // "none"')
  [ "$OK" = "true" ] && result "$NAME" "PASS" "title=$TITLE" "$DUR" || result "$NAME" "FAIL" "error=$ERR" "$DUR"
done

# Amazon product (graceful timeout expected)
log "NAV: Amazon product (timeout test)"
T=$(date +%s)
SID=$(create_session "navigate to https://www.amazon.com/dp/B0D1XD1ZV3")
R=$(execute_session "$SID")
DUR="$(($(date +%s) - T))s"
ERR=$(echo "$R" | jq -r '.steps[0].result.Error // "no response"')
if echo "$ERR" | grep -qi "timed out"; then
  result "Amazon timeout" "PASS" "$ERR" "$DUR"
elif [ "$(echo "$R" | jq -r '.success')" = "true" ]; then
  result "Amazon (loaded!)" "PASS" "Loaded!" "$DUR"
else
  result "Amazon" "FAIL" "error=$ERR len=${#R}" "$DUR"
fi

# ──────────────────────────────────────────────────────────────
# 2. TASK PLANNER (SSE streaming)
# ──────────────────────────────────────────────────────────────
log "PLANNER: Simple extraction"
T=$(date +%s)
R=$(curl -s -m 60 -X POST "$BASE/tasks/execute" -H "Content-Type: application/json" \
  -d '{"goal": "Get the title of https://example.com"}' 2>&1)
DUR="$(($(date +%s) - T))s"
if echo "$R" | grep -q '"status":"complete"'; then
  result "Planner: example.com" "PASS" "Planned+extracted" "$DUR"
else
  result "Planner: example.com" "FAIL" "${R:0:200}" "$DUR"
fi

log "PLANNER: Multi-step HN extraction"
T=$(date +%s)
R=$(curl -s -m 60 -X POST "$BASE/tasks/execute" -H "Content-Type: application/json" \
  -d '{"goal": "Go to https://news.ycombinator.com and get the title of the top story"}' 2>&1)
DUR="$(($(date +%s) - T))s"
if echo "$R" | grep -q '"status":"complete"'; then
  result "Planner: HN top story" "PASS" "Extracted" "$DUR"
else
  result "Planner: HN top story" "FAIL" "${R:0:200}" "$DUR"
fi

# ──────────────────────────────────────────────────────────────
# 3. SESSION IMPORT (multipart form, cookie bridge)
# ──────────────────────────────────────────────────────────────
log "IMPORT: Netscape cookies (preserve)"
T=$(date +%s)
COOKIES='# Netscape HTTP Cookie File
.github.com	TRUE	/	TRUE	1743183600	logged_in	yes
.github.com	TRUE	/	FALSE	1743183600	_gh_sess	abc123def456'
R=$(curl -s -X POST "$BASE/sessions/import" -F "cookies=$COOKIES" -F "domain_hint=.github.com" -F "trust_mode=preserve" 2>&1)
DUR="$(($(date +%s) - T))s"
SID=$(echo "$R" | jq -r '.session_id // .profile_id // "null"')
[ "$SID" != "null" ] && result "Import preserve" "PASS" "id=$SID" "$DUR" || result "Import preserve" "FAIL" "${R:0:200}" "$DUR"

log "IMPORT: Netscape cookies (standard)"
T=$(date +%s)
R=$(curl -s -X POST "$BASE/sessions/import" -F "cookies=$COOKIES" -F "domain_hint=.github.com" -F "trust_mode=standard" 2>&1)
DUR="$(($(date +%s) - T))s"
SID=$(echo "$R" | jq -r '.session_id // .profile_id // "null"')
[ "$SID" != "null" ] && result "Import standard" "PASS" "id=$SID" "$DUR" || result "Import standard" "FAIL" "${R:0:200}" "$DUR"

# ──────────────────────────────────────────────────────────────
# 4. CHECKPOINT (save + resume)
# ──────────────────────────────────────────────────────────────
log "CHECKPOINT: Save + resume"
T=$(date +%s)
R=$(curl -s -m 45 -X POST "$BASE/tasks/execute" -H "Content-Type: application/json" \
  -d '{"goal": "Get the title of https://example.com"}' 2>&1)
CKPT=$(ls -t ~/.openclaw/workspace-builder/aperture/checkpoints/*.json 2>/dev/null | head -1)
if [ -n "$CKPT" ]; then
  CKPT_ID=$(basename "$CKPT" .json)
  R2=$(curl -s -m 45 -X POST "$BASE/tasks/resume" -H "Content-Type: application/json" \
    -d "{\"checkpoint_id\": \"$CKPT_ID\"}" 2>&1)
  DUR="$(($(date +%s) - T))s"
  if echo "$R2" | grep -q '"status"'; then
    result "Checkpoint resume" "PASS" "Resumed $CKPT_ID" "$DUR"
  else
    result "Checkpoint resume" "FAIL" "${R2:0:200}" "$DUR"
  fi
else
  DUR="$(($(date +%s) - T))s"
  result "Checkpoint resume" "FAIL" "No checkpoint files" "$DUR"
fi

# ──────────────────────────────────────────────────────────────
# 5. KV STORE
# ──────────────────────────────────────────────────────────────
log "KV: Set + Get"
T=$(date +%s)
curl -s -X PUT "$BASE/agents/test-agent-1/memory/test-key" -H "Content-Type: application/json" \
  -d '{"value": "hello-adversarial-2026"}' > /dev/null
R=$(curl -s "$BASE/agents/test-agent-1/memory/test-key")
DUR="$(($(date +%s) - T))s"
GOT=$(echo "$R" | jq -r '.value // "null"')
[ "$GOT" = "hello-adversarial-2026" ] && result "KV round-trip" "PASS" "Verified" "$DUR" || result "KV round-trip" "FAIL" "Got: $GOT" "$DUR"

# ──────────────────────────────────────────────────────────────
# 6. CREDENTIAL VAULT
# ──────────────────────────────────────────────────────────────
log "CREDENTIALS: Store + List"
T=$(date +%s)
curl -s -X PUT "$BASE/agents/test-agent-1/credentials/example.com" -H "Content-Type: application/json" \
  -d '{"username": "testuser", "password": "testpass123", "auto_login": true}' > /dev/null
R=$(curl -s "$BASE/agents/test-agent-1/credentials")
DUR="$(($(date +%s) - T))s"
COUNT=$(echo "$R" | jq '. | length' 2>/dev/null || echo "0")
[ "$COUNT" -gt 0 ] && result "Credentials" "PASS" "$COUNT credential(s)" "$DUR" || result "Credentials" "FAIL" "${R:0:200}" "$DUR"

# ──────────────────────────────────────────────────────────────
# 7. SITE PROFILES
# ──────────────────────────────────────────────────────────────
log "PROFILES: Amazon domain match"
T=$(date +%s)
SID=$(create_session "navigate to https://www.amazon.com")
R=$(execute_session "$SID")
DUR="$(($(date +%s) - T))s"
PROFILE=$(echo "$R" | jq -r '.steps[0].result.PageState.ProfileMatched // "none"')
if [ "$PROFILE" != "none" ] && [ "$PROFILE" != "null" ]; then
  result "Amazon profile" "PASS" "matched=$PROFILE" "$DUR"
elif [ "$(echo "$R" | jq -r '.success')" = "true" ]; then
  result "Amazon profile" "FAIL" "Nav OK but no profile" "$DUR"
else
  result "Amazon profile" "FAIL" "Nav failed" "$DUR"
fi

# ──────────────────────────────────────────────────────────────
# 8. CONCURRENT SESSIONS (pool stress)
# ──────────────────────────────────────────────────────────────
log "CONCURRENCY: 3 parallel navigations"
T=$(date +%s)
SA=$(create_session "navigate to https://example.com")
SB=$(create_session "navigate to https://httpbin.org/html")
SC=$(create_session "navigate to https://en.wikipedia.org")
TA=$(mktemp); TB=$(mktemp); TC=$(mktemp)
curl -s -m 35 -X POST "$BASE/sessions/$SA/execute" > "$TA" 2>&1 &
PA=$!
curl -s -m 35 -X POST "$BASE/sessions/$SB/execute" > "$TB" 2>&1 &
PB=$!
curl -s -m 35 -X POST "$BASE/sessions/$SC/execute" > "$TC" 2>&1 &
PC=$!
wait $PA $PB $PC 2>/dev/null
DUR="$(($(date +%s) - T))s"
OKN=0
for F in "$TA" "$TB" "$TC"; do
  jq -e '.success == true' "$F" >/dev/null 2>&1 && OKN=$((OKN+1))
done
rm -f "$TA" "$TB" "$TC"
[ "$OKN" -eq 3 ] && result "3x concurrent" "PASS" "All 3 succeeded" "$DUR" || result "3x concurrent" "FAIL" "$OKN/3 succeeded" "$DUR"

# ──────────────────────────────────────────────────────────────
# 9. SNAPSHOT
# ──────────────────────────────────────────────────────────────
log "SNAPSHOT: Capture page"
T=$(date +%s)
SID=$(create_session "navigate to https://example.com")
execute_session "$SID" > /dev/null
R=$(curl -s "$BASE/sessions/$SID/snapshot")
DUR="$(($(date +%s) - T))s"
LEN=$(echo "$R" | jq -r '.html // .snapshot // "" | length' 2>/dev/null || echo "0")
[ "$LEN" -gt 100 ] 2>/dev/null && result "Snapshot" "PASS" "${LEN} chars" "$DUR" || result "Snapshot" "FAIL" "${R:0:200}" "$DUR"

# ──────────────────────────────────────────────────────────────
# 10. xBPP POLICY CRUD
# ──────────────────────────────────────────────────────────────
log "xBPP: Policy set + get"
T=$(date +%s)
curl -s -X PUT "$BASE/policies/test-agent-1" -H "Content-Type: application/json" \
  -d '{"allowed_actions": ["navigate","extract"], "blocked_domains": ["evil.com"], "rate_limit": 10}' > /dev/null
R=$(curl -s "$BASE/policies/test-agent-1")
DUR="$(($(date +%s) - T))s"
BLOCKED=$(echo "$R" | jq -r '.blocked_domains // [] | length')
[ "$BLOCKED" -gt 0 ] 2>/dev/null && result "xBPP policy" "PASS" "blocked=$BLOCKED" "$DUR" || result "xBPP policy" "FAIL" "${R:0:200}" "$DUR"

# ──────────────────────────────────────────────────────────────
# 11. SESSION LIFECYCLE (list + delete)
# ──────────────────────────────────────────────────────────────
log "SESSION: List + Delete"
T=$(date +%s)
LIST=$(curl -s "$BASE/sessions" | jq '. | length' 2>/dev/null || echo "err")
SID=$(create_session "navigate to https://example.com")
DEL=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE "$BASE/sessions/$SID")
DUR="$(($(date +%s) - T))s"
[ "$DEL" = "200" ] || [ "$DEL" = "204" ] && result "Session list+delete" "PASS" "Listed $LIST, deleted HTTP $DEL" "$DUR" || result "Session list+delete" "FAIL" "HTTP $DEL" "$DUR"

# ──────────────────────────────────────────────────────────────
# 12. BILLING (LAST — creates accounts, enables auth)
# ──────────────────────────────────────────────────────────────
log "BILLING: Create account"
T=$(date +%s)
R=$(curl -s -X POST "$BASE/admin/accounts" -H "Content-Type: application/json" \
  -d '{"name": "test-adversarial", "initial_credits": 50}')
ACCT_ID=$(echo "$R" | jq -r '.account.id // "null"')
API_KEY=$(echo "$R" | jq -r '.api_key.key // "null"')
DUR="$(($(date +%s) - T))s"
if [ "$ACCT_ID" != "null" ]; then
  BALANCE=$(curl -s "$BASE/admin/accounts/$ACCT_ID" -H "Authorization: Bearer $API_KEY" | jq -r '.credit_balance // "?"')
  result "Billing: create" "PASS" "id=$ACCT_ID balance=$BALANCE" "$DUR"
else
  result "Billing: create" "FAIL" "${R:0:200}" "$DUR"
fi

if [ "${API_KEY:-null}" != "null" ]; then
  log "BILLING: Execute with key (credit deduction)"
  T=$(date +%s)
  SID=$(curl -s -X POST "$BASE/sessions" -H "Content-Type: application/json" \
    -H "Authorization: Bearer $API_KEY" -d '{"goal": "navigate to https://example.com"}' | jq -r .session_id)
  R2=$(curl -s -m 15 -X POST "$BASE/sessions/$SID/execute" -H "Authorization: Bearer $API_KEY" 2>&1)
  BALANCE2=$(curl -s "$BASE/admin/accounts/$ACCT_ID" -H "Authorization: Bearer $API_KEY" | jq -r '.credit_balance // "?"')
  DUR="$(($(date +%s) - T))s"
  [ "$(echo "$R2" | jq -r '.success')" = "true" ] && result "Billing: execute+deduct" "PASS" "balance=$BALANCE2" "$DUR" || result "Billing: execute+deduct" "FAIL" "$(echo "$R2" | jq -r '.steps[0].result.Error // "unknown"')" "$DUR"

  log "BILLING: Zero credits"
  T=$(date +%s)
  R3=$(curl -s -X POST "$BASE/admin/accounts" -H "Content-Type: application/json" \
    -H "Authorization: Bearer $API_KEY" -d '{"name": "test-broke", "initial_credits": 0}')
  KEY2=$(echo "$R3" | jq -r '.api_key.key // "null"')
  if [ "$KEY2" != "null" ]; then
    SID2=$(curl -s -X POST "$BASE/sessions" -H "Content-Type: application/json" \
      -H "Authorization: Bearer $KEY2" -d '{"goal": "navigate to https://example.com"}' | jq -r '.session_id // "null"')
    if [ "$SID2" != "null" ]; then
      R4=$(curl -s -m 10 -X POST "$BASE/sessions/$SID2/execute" -H "Authorization: Bearer $KEY2" 2>&1)
      DUR="$(($(date +%s) - T))s"
      if echo "$R4" | grep -qi "credit\|insufficient\|balance"; then
        result "Zero credits" "PASS" "Blocked correctly" "$DUR"
      elif [ "$(echo "$R4" | jq -r '.success')" = "true" ]; then
        result "Zero credits" "FAIL" "EXECUTED WITH 0 CREDITS!" "$DUR"
      else
        result "Zero credits" "PASS" "Blocked: $(echo "$R4" | jq -c '.' | head -c 100)" "$DUR"
      fi
    else
      DUR="$(($(date +%s) - T))s"
      result "Zero credits" "PASS" "Session creation blocked" "$DUR"
    fi
  else
    DUR="$(($(date +%s) - T))s"
    result "Zero credits" "FAIL" "No key: ${R3:0:200}" "$DUR"
  fi
fi

# ──────────────────────────────────────────────────────────────
# SUMMARY
# ──────────────────────────────────────────────────────────────
echo ""
echo "========================================"
echo " APERTURE v4 ADVERSARIAL TEST RESULTS"
echo "========================================"
echo -e "$RESULTS"
echo "────────────────────────────────────────"
echo "TOTAL: $((PASS+FAIL)) | ✅ PASS: $PASS | 🔴 FAIL: $FAIL"
echo "========================================"
