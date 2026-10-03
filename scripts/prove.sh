#!/usr/bin/env bash
# Proves Flagpole works end to end against the running stack (make up, api,
# worker, seed). It signs in as each demo user, creates a throwaway flag, takes
# it through a reviewed production change, and checks what an SDK sees:
# evaluation, the snapshot ETag, the live stream, exposures, key revocation.
# It archives the flag and revokes its key when it's done.
#
#   scripts/prove.sh                      # against http://localhost:8080
#   API=http://localhost:3000 scripts/prove.sh   # through the container web app
set -uo pipefail

API=${API:-http://localhost:8080}
PASSWORD=${FLAGPOLE_PASSWORD:-flagpole-demo-password}
PROJECT=web-app
FLAG="proof-$(date +%H%M%S)"
TMP=$(mktemp -d)
STREAM=
trap '[ -n "$STREAM" ] && kill $STREAM 2>/dev/null; rm -rf "$TMP"' EXIT

passed=0 failed=0
ok() { printf '  \033[32m✓\033[0m %s\n' "$1"; passed=$((passed + 1)); }
no() { printf '  \033[31m✗\033[0m %s\n      %s\n' "$1" "$2"; failed=$((failed + 1)); }
check() { # check "description" actual expected
  if [ "$2" = "$3" ]; then ok "$1"; else no "$1" "expected $3, got $2"; fi
}
step() { printf '\n\033[1m%s\033[0m\n' "$1"; }

# call WHO METHOD PATH [JSON]: sets $status and $body. WHO is a cookie jar.
call() {
  local who=$1 method=$2 path=$3 data=${4:-}
  local token
  token=$(awk '$6 == "flagpole_csrf" { print $7 }' "$TMP/$who" 2>/dev/null)
  local args=(-s -b "$TMP/$who" -c "$TMP/$who" -X "$method" -H "X-CSRF-Token: $token" -o "$TMP/body" -w '%{http_code}')
  [ -n "$data" ] && args+=(-H 'Content-Type: application/json' -d "$data")
  status=$(curl "${args[@]}" "$API$path")
  body=$(cat "$TMP/body")
}

# sdk METHOD PATH [JSON] [extra curl args...]: the same, with the SDK key.
sdk() {
  local method=$1 path=$2 data=${3:-}
  shift 3 2>/dev/null || shift $#
  local args=(-s -X "$method" -H "Authorization: Bearer $KEY" -D "$TMP/headers" -o "$TMP/body" -w '%{http_code}' "$@")
  [ -n "$data" ] && args+=(-H 'Content-Type: application/json' -d "$data")
  status=$(curl "${args[@]}" "$API$path")
  body=$(cat "$TMP/body")
}

# eventually "description" COMMAND EXPECTED: retries for up to 10s, because
# changes reach SDKs through Kafka and the worker.
eventually() {
  local got
  for _ in $(seq 1 40); do
    got=$(eval "$2")
    [ "$got" = "$3" ] && { ok "$1"; return; }
    sleep 0.25
  done
  no "$1" "expected $3, got $got"
}

flag_path="/api/v1/projects/$PROJECT/flags/$FLAG"
version_in() { call grace GET "$flag_path"; echo "$body" | jq -r ".environments[] | select(.environment == \"$1\") | .version"; }

step "Sign in"
curl -sf "$API/healthz" -o /dev/null || { echo "The API isn't answering at $API. Start it with make api."; exit 1; }
for who in ada grace linus vera; do
  call "$who" GET /api/v1/auth/me # picks up the CSRF cookie
  call "$who" POST /api/v1/auth/login "{\"email\":\"$who@example.com\",\"password\":\"$PASSWORD\"}"
  check "$who signs in" "$status" 200
done
call vera POST /api/v1/auth/login '{"email":"vera@example.com","password":"wrong-password-123"}'
check "a wrong password is refused" "$status" 401

step "Roles"
call grace POST "/api/v1/projects/$PROJECT/flags" "{\"key\":\"$FLAG\",\"name\":\"Proof $FLAG\",\"kind\":\"boolean\",\"tags\":[\"proof\"]}"
check "editor grace creates flag $FLAG" "$status" 201
call vera PUT "$flag_path/environments/development" '{"enabled":true,"default_variant":"on","off_variant":"off","base_version":1}'
check "viewer vera can't change it" "$status" 403
call grace POST "/api/v1/projects/$PROJECT/environments/production/sdk-keys" '{"name":"proof"}'
check "editor grace can't create SDK keys" "$status" 403
call ada POST "/api/v1/projects/$PROJECT/environments/production/sdk-keys" "{\"name\":\"$FLAG\"}"
check "admin ada creates a production SDK key" "$status" 201
KEY=$(echo "$body" | jq -r .key)
KEY_ID=$(echo "$body" | jq -r .id)
call grace POST "/api/v1/projects/$PROJECT/flags" '{"key":"Not A Key!","name":"x","kind":"boolean"}'
check "a bad flag key is a 422 naming the field" "$status:$(echo "$body" | jq -r '.errors | keys[0]')" "422:key"

step "Optimistic concurrency"
call grace PUT "$flag_path/environments/development" '{"enabled":true,"default_variant":"on","off_variant":"off","base_version":1}'
check "grace turns it on in development (v1 → v2)" "$status" 200
call grace PUT "$flag_path/environments/development" '{"enabled":false,"default_variant":"on","off_variant":"off","base_version":1}'
check "a second write from v1 is a 409, not a silent overwrite" "$status" 409

step "Reviewed change to production"
curl -sN -m 30 -H "Authorization: Bearer $KEY" "$API/sdk/v1/stream" > "$TMP/stream" &
STREAM=$!
disown
sleep 0.5
sdk POST /sdk/v1/evaluate "{\"flags\":[\"$FLAG\"],\"context\":{\"key\":\"u1\",\"attributes\":{\"country\":\"uk\"}}}"
check "before: the SDK gets off in production" "$(echo "$body" | jq -r ".flags[\"$FLAG\"].variant")" off
base=$(version_in production)
rule="{\"enabled\":true,\"default_variant\":\"off\",\"off_variant\":\"off\",\"rules\":[{\"description\":\"UK first\",\"conditions\":[{\"attribute\":\"country\",\"operator\":\"in\",\"values\":[\"uk\"]}],\"variant\":\"on\"}],\"base_version\":$base"
call grace PUT "$flag_path/environments/production" "$rule}"
check "grace can't change production directly" "$status" 409
call grace POST "$flag_path/environments/production/change-requests" "$rule,\"comment\":\"UK first\"}"
check "grace requests the change instead" "$status" 201
CR=$(echo "$body" | jq -r .id)
call grace POST "/api/v1/projects/$PROJECT/change-requests/$CR/approve" '{}'
check "grace can't approve her own request" "$status" 403
announced() { grep -c "\"flag\":\"$FLAG\"" "$TMP/stream" | tr -d ' '; }
before=$(announced)
call linus POST "/api/v1/projects/$PROJECT/change-requests/$CR/approve" '{"comment":"ship it"}'
check "approver linus approves it" "$status:$(echo "$body" | jq -r .status)" "200:applied"

step "What SDKs see"
eval_for() { sdk POST /sdk/v1/evaluate "{\"flags\":[\"$FLAG\"],\"context\":{\"key\":\"$1\",\"attributes\":{\"country\":\"$2\"}}}"; echo "$body" | jq -r ".flags[\"$FLAG\"] | \"\(.variant)/\(.reason)\""; }
eventually "a UK user gets on, by the rule" "eval_for u1 uk" "on/rule"
check "a French user gets off, the default" "$(eval_for u2 fr)" "off/default"
eventually "the live stream announced the change" '[ "$(announced)" -gt "$before" ] && echo yes || echo no' yes
sdk GET /sdk/v1/flags ""
etag=$(awk 'tolower($1) == "etag:" { print $2 }' "$TMP/headers" | tr -d '\r')
check "the snapshot has the flag with its rule" "$(echo "$body" | jq -r ".flags[\"$FLAG\"].rules | length")" 1
sdk GET /sdk/v1/flags "" -H "If-None-Match: $etag"
check "asking again with the ETag is a 304" "$status" 304

step "Exposures"
sdk POST /sdk/v1/exposures "{\"exposures\":[{\"flag\":\"$FLAG\",\"variant\":\"on\"},{\"flag\":\"$FLAG\",\"variant\":\"on\"},{\"flag\":\"$FLAG\",\"variant\":\"off\"}]}"
check "the SDK reports three exposures" "$status" 202
exposures() { call ada GET "$flag_path/environments/production/exposures?hours=1"; echo "$body" | jq -r '"\(.totals.on // 0) on, \(.totals.off // 0) off"'; }
eventually "they're counted through Kafka" exposures "2 on, 1 off"

step "Audit"
call ada GET "/api/v1/projects/$PROJECT/audit?limit=50"
echo "$body" | jq -r ".events[] | select(.summary | contains(\"$FLAG\")) | \"      · \" + .summary" | tail -r 2>/dev/null || true
check "every step was logged" "$(echo "$body" | jq "[.events[] | select(.summary | contains(\"$FLAG\"))] | length >= 4")" true

step "Clean up"
call ada DELETE "/api/v1/projects/$PROJECT/environments/production/sdk-keys/$KEY_ID"
check "ada revokes the key" "$status" 204
sdk POST /sdk/v1/evaluate '{}'
check "the revoked key stops working at once" "$status" 401
call grace POST "$flag_path/archive"
check "grace archives the flag" "$status" 200

printf '\n%s passed, %s failed\n' "$passed" "$failed"
[ "$failed" -eq 0 ]
