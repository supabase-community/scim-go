#!/usr/bin/env bash
# `is` evals its argument, so single-quoted expressions and variables only read
# inside them are intentional.
# shellcheck disable=SC2016,SC2034
# Reproduces the RFC 7643/7644 conformance issues scimcheck found in the
# example server. Each issue prints PASS once fixed, or FAIL with the
# expected and actual behaviour and where to start looking.
#
# Usage:
#   scripts/conformance-issues.sh            # build and start ./cmd/server, run every issue
#   scripts/conformance-issues.sh 1 5 14     # run only these issues
#   BASE_URL=http://localhost:8080/scim/v2 SCIM_BEARER_TOKEN=secret scripts/conformance-issues.sh
#                                            # use a server that is already running
#   STRICT=1 scripts/conformance-issues.sh   # also fail on optional (MAY) issues
#
# Exit status: 0 when every MUST and SHOULD issue is fixed, 1 otherwise.
# Requires: go, curl, jq.
set -euo pipefail

for tool in curl jq; do
	command -v "$tool" >/dev/null || {
		echo "error: $tool is required" >&2
		exit 2
	}
done

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WORK="$(mktemp -d)"
SERVER_PID=""

cleanup() {
	if [ -n "$SERVER_PID" ]; then kill "$SERVER_PID" 2>/dev/null || true; fi
	rm -rf "$WORK"
}
trap cleanup EXIT

if [ -z "${BASE_URL:-}" ]; then
	PORT="${PORT:-18080}"
	export SCIM_BEARER_TOKEN="${SCIM_BEARER_TOKEN:-conformance-secret}"
	echo "building ./cmd/server ..."
	(cd "$ROOT" && go build -o "$WORK/scim-server" ./cmd/server)
	PORT="$PORT" "$WORK/scim-server" >"$WORK/server.log" 2>&1 &
	SERVER_PID=$!
	BASE_URL="http://localhost:$PORT/scim/v2"
	for _ in $(seq 1 50); do
		curl -sf -o /dev/null -H "Authorization: Bearer $SCIM_BEARER_TOKEN" "$BASE_URL/ServiceProviderConfig" && break
		sleep 0.1
	done
fi
: "${SCIM_BEARER_TOKEN:?set SCIM_BEARER_TOKEN for the server at BASE_URL}"

USER_URN="urn:ietf:params:scim:schemas:core:2.0:User"
GROUP_URN="urn:ietf:params:scim:schemas:core:2.0:Group"
ENTERPRISE_URN="urn:ietf:params:scim:schemas:extension:enterprise:2.0:User"
PATCH_URN="urn:ietf:params:scim:api:messages:2.0:PatchOp"
RUN="ci$(date +%s)$RANDOM"
PREFIX="conformance-$RUN"

# ----------------------------------------------------------------------------
# HTTP helpers. `req` sets STATUS, BODY and HEADERS.
# ----------------------------------------------------------------------------

STATUS=""
BODY=""
HEADERS=""

# req METHOD PATH [BODY] [extra curl arguments...]
req() {
	local method="$1" path="$2" data="${3:-}"
	shift 3 2>/dev/null || shift $#
	local url="$path"
	[[ "$url" == http* ]] || url="$BASE_URL$path"
	local args=(-s -o "$WORK/body" -D "$WORK/headers" -w '%{http_code}' -X "$method" "$url" -H "Content-Type: application/scim+json")
	if [ "${NO_AUTH:-}" != 1 ]; then args+=(-H "Authorization: Bearer $SCIM_BEARER_TOKEN"); fi
	if [ -n "$data" ]; then args+=(--data-binary "$data"); fi
	STATUS=$(curl "${args[@]}" "$@")
	BODY=$(cat "$WORK/body")
	HEADERS=$(tr -d '\r' <"$WORK/headers")
}

header() {
	grep -i "^$1:" <<<"$HEADERS" | head -1 | cut -d' ' -f2- || true
}

# get_query PATH NAME VALUE: GET with one URL-encoded query parameter.
get_query() {
	local encoded
	encoded=$(jq -rn --arg v "$3" '$v|@uri')
	req GET "$1?$2=$encoded"
}

user_json() { # user_json USERNAME EMAIL [EXTRA_JSON_OBJECT]
	local extra="${3:-}"
	[ -n "$extra" ] || extra='{}'
	jq -cn --arg u "$1" --arg e "$2" --argjson extra "$extra" --arg urn "$USER_URN" \
		'{schemas: [$urn], userName: $u, emails: [{value: $e, type: "work", primary: true}]} + $extra'
}

create_user() { # create_user USERNAME EMAIL -> prints id
	req POST /Users "$(user_json "$1" "$2")"
	[ "$STATUS" = 201 ] || {
		echo "setup failed: POST /Users returned $STATUS: $BODY" >&2
		exit 2
	}
	jq -r .id <<<"$BODY"
}

patch_json() { # patch_json OPERATION_JSON...
	jq -cn --arg urn "$PATCH_URN" '{schemas: [$urn], Operations: $ARGS.positional}' --jsonargs "$@"
}

# ----------------------------------------------------------------------------
# Reporting
# ----------------------------------------------------------------------------

FAILED=0
FAILED_OPTIONAL=0
PASSED=0
SELECTED=" $* "

selected() { [ "$SELECTED" = "  " ] || [[ "$SELECTED" == *" $1 "* ]]; }

# report NUMBER LEVEL REF TITLE OK EXPECTED HINT
report() {
	local number="$1" level="$2" ref="$3" title="$4" ok="$5" expected="$6" hint="$7"
	if [ "$ok" = 1 ]; then
		PASSED=$((PASSED + 1))
		printf '[PASS] %2s %-6s %-18s %s\n' "$number" "$level" "$ref" "$title"
		return
	fi
	if [ "$level" = MAY ]; then FAILED_OPTIONAL=$((FAILED_OPTIONAL + 1)); else FAILED=$((FAILED + 1)); fi
	printf '[FAIL] %2s %-6s %-18s %s\n' "$number" "$level" "$ref" "$title"
	printf '       expected: %s\n' "$expected"
	printf '       actual:   HTTP %s %s\n' "$STATUS" "$(head -c 300 <<<"$BODY")"
	printf '       look at:  %s\n' "$hint"
}

is() { if eval "$1"; then echo 1; else echo 0; fi; }

# ----------------------------------------------------------------------------
# Shared fixtures: three users whose primary emails sort b, c, a, and a group.
# ----------------------------------------------------------------------------

echo "server: $BASE_URL (run $RUN)"
ALICE=$(create_user "$PREFIX-alice" "c.$PREFIX-alice@example.com")
BOB=$(create_user "$PREFIX-bob" "a.$PREFIX-bob@example.com")
CAROL=$(create_user "$PREFIX-carol" "b.$PREFIX-carol@example.com")
req POST /Groups "$(jq -cn --arg urn "$GROUP_URN" --arg n "$PREFIX-group" --arg m "$ALICE" '{schemas: [$urn], displayName: $n, members: [{value: $m}]}')"
GROUP=$(jq -r .id <<<"$BODY")
CREATED=("/Groups/$GROUP" "/Users/$ALICE" "/Users/$BOB" "/Users/$CAROL")

delete_created() {
	for path in "${CREATED[@]}"; do req DELETE "$path"; done
}
trap 'delete_created >/dev/null 2>&1 || true; cleanup' EXIT

echo
echo "MUST"

if selected 1; then
	encoded=$(jq -rn --arg v "userName sw \"$PREFIX\"" '$v|@uri')
	req GET "/Users?filter=$encoded&sortBy=emails"
	order=$(jq -r '[.Resources[]?.userName | sub(".*-"; "")] | join(",")' <<<"$BODY" 2>/dev/null || true)
	report 1 MUST "RFC7644 §3.4.2.3" "sortBy on a multi-valued attribute sorts by its primary value" \
		"$(is '[ "$STATUS" = 200 ] && [ "$order" = "bob,carol,alice" ]')" \
		"200 with Resources ordered bob,carol,alice (primary emails a., b., c.)" \
		"pkg/protocol/search_request.go (\"sortBy\" must name a sub-attribute) and pkg/server/sort.go"
fi

echo
echo "SHOULD"

if selected 2; then
	req POST /Users "$(jq -cn --arg u "$PREFIX-noschemas" '{userName: $u}')"
	[ "$STATUS" = 201 ] && CREATED+=("/Users/$(jq -r .id <<<"$BODY")")
	report 2 SHOULD "RFC7643 §3" "a resource without \"schemas\" is rejected" \
		"$(is '[ "$STATUS" = 400 ]')" \
		"400 invalidValue or invalidSyntax: \"schemas\" is REQUIRED on every resource" \
		"pkg/protocol/resource.go (DecodeResource)"
fi

if selected 3; then
	req POST /Users "$(jq -cn --arg u "$PREFIX-badschema" '{schemas: ["urn:example:unknown"], userName: $u}')"
	[ "$STATUS" = 201 ] && CREATED+=("/Users/$(jq -r .id <<<"$BODY")")
	report 3 SHOULD "RFC7643 §3" "a resource with an unknown schema URN is rejected" \
		"$(is '[ "$STATUS" = 400 ]')" \
		"400: \"schemas\" may only list the resource type's schema and its extensions" \
		"pkg/protocol/resource.go (DecodeResource)"
fi

if selected 4; then
	body=$(user_json "$PREFIX-enterprise" "$PREFIX-enterprise@example.com" "$(jq -cn --arg e "$ENTERPRISE_URN" --arg urn "$USER_URN" '{schemas: [$urn, $e], ($e): {employeeNumber: "42"}}')")
	req POST /Users "$body"
	id=$(jq -r .id <<<"$BODY")
	CREATED+=("/Users/$id")
	req PUT "/Users/$id" "$(user_json "$PREFIX-enterprise" "$PREFIX-enterprise@example.com")"
	report 4 SHOULD "RFC7643 §3" "\"schemas\" drops an extension URN once the extension is removed" \
		"$(is '[ "$STATUS" = 200 ] && [ "$(jq --arg e "$ENTERPRISE_URN" "(.schemas | index(\$e)) == null" <<<"$BODY")" = true ]')" \
		"200 whose schemas lists only $USER_URN" \
		"pkg/server/repository.go (schemaURIs lists every registered schema)"
fi

if selected 5; then
	get_query /Users filter "userName sw \"$PREFIX\" and emails co \"a.$PREFIX\""
	report 5 SHOULD "RFC7644 §3.4.2.2" "a filter on a multi-valued attribute without a sub-attribute compares its value" \
		"$(is '[ "$STATUS" = 200 ] && [ "$(jq .totalResults <<<"$BODY")" = 1 ]')" \
		"200 with totalResults 1 (the RFC's own example: emails co \"example.com\")" \
		"pkg/protocol/visitor.go (operator %q is not valid for %q)"
fi

if selected 6; then
	req POST /Bulk '{"schemas":["urn:ietf:params:scim:api:messages:2.0:BulkRequest"],"Operations":[]}'
	report 6 SHOULD "RFC7644 §3.12" "POST /Bulk answers 501 when bulk is unsupported" \
		"$(is '[ "$STATUS" = 501 ]')" \
		"501 Not Implemented with a SCIM Error body, as /Me already does" \
		"pkg/server/server.go (register a /Bulk handler like me())"
fi

if selected 7; then
	req PATCH "/Users/$CAROL" "$(patch_json '{"op":"add","path":"emails","value":[{"value":"carol@home.example.com","type":"home"}]}')"
	req PATCH "/Users/$CAROL" "$(patch_json '{"op":"add","path":"emails","value":[{"value":"carol@home.example.com","type":"home"}]}')"
	req GET "/Users/$CAROL"
	report 7 SHOULD "RFC7644 §3.5.2.1" "PATCH add of a value that is already present changes nothing" \
		"$(is '[ "$(jq "[.emails[] | select(.value == \"carol@home.example.com\")] | length" <<<"$BODY")" = 1 ]')" \
		"carol@home.example.com appears once in emails" \
		"pkg/patch (add on multi-valued attributes)"
fi

if selected 8; then
	req PATCH "/Groups/$GROUP" "$(patch_json "$(jq -cn --arg m "$ALICE" '{op: "add", path: "members", value: [{value: $m}]}')")"
	req GET "/Groups/$GROUP"
	report 8 SHOULD "RFC7644 §3.5.2.1" "PATCH add of an existing member does not duplicate it" \
		"$(is '[ "$(jq --arg m "$ALICE" "[.members[] | select(.value == \$m)] | length" <<<"$BODY")" = 1 ]')" \
		"alice listed once in members" \
		"pkg/patch (add on multi-valued attributes)"
fi

if selected 9; then
	req GET "/Groups/$GROUP"
	report 9 SHOULD "RFC7643 §4.2" "group members carry type \"User\"" \
		"$(is '[ "$(jq -r ".members[0].type // empty" <<<"$BODY")" = User ]')" \
		"members[0].type is \"User\"" \
		"pkg/core/group.go and pkg/server/repository.go (populate member type on write)"
fi

if selected 10; then
	req GET "/Groups/$GROUP"
	report 10 SHOULD "RFC7643 §4.2" "group members carry a \$ref to the member resource" \
		"$(is '[[ "$(jq -r ".members[0][\"\$ref\"] // empty" <<<"$BODY")" == *"/Users/$ALICE" ]]')" \
		"members[0].\$ref ends with /Users/$ALICE" \
		"pkg/core/group.go (Ref is never set) and pkg/server/repository.go"
fi

if selected 11; then
	req GET "/Users/$ALICE"
	report 11 SHOULD "RFC7643 §4.1.2" "User.groups lists the groups the user belongs to" \
		"$(is '[ "$(jq --arg g "$GROUP" "[.groups[]? | select(.value == \$g)] | length" <<<"$BODY")" = 1 ]')" \
		"groups contains {\"value\": \"$GROUP\", ...}" \
		"pkg/server (derive User.groups from Group.members on read)"
fi

if selected 12; then
	req PATCH "/Groups/$GROUP" "$(patch_json "$(jq -cn --arg p "members[value eq \"$ALICE\"].value" --arg v "$BOB" '{op: "replace", path: $p, value: $v}')")"
	report 12 SHOULD "RFC7643 §4.2" "an existing member's value is immutable" \
		"$(is '[ "$STATUS" = 400 ] && [ "$(jq -r ".scimType // empty" <<<"$BODY")" = mutability ]')" \
		"400 mutability: members.value is immutable once assigned" \
		"pkg/server/validators.go (immutableElements matches elements by value, so a changed value looks like a new member)"
	# Undo the change if it was applied so later issues see alice as a member.
	req PUT "/Groups/$GROUP" "$(jq -cn --arg urn "$GROUP_URN" --arg n "$PREFIX-group" --arg m "$ALICE" '{schemas: [$urn], displayName: $n, members: [{value: $m}]}')"
fi

echo
echo "MAY (optional; reported, not counted unless STRICT=1)"

if selected 13; then
	req POST /Users/.search "$(jq -cn --arg f "userName sw \"$PREFIX\"" '{schemas: ["urn:ietf:params:scim:api:messages:2.0:SearchRequest"], filter: $f}')"
	report 13 MAY "RFC7644 §3.4.3" "POST /Users/.search runs a SearchRequest" \
		"$(is '[ "$STATUS" = 200 ] && [ "$(jq .totalResults <<<"$BODY")" = 3 ]')" \
		"200 ListResponse with totalResults 3" \
		"pkg/server/resource.go (add \"POST <path>/.search\") and pkg/protocol/search_request.go"
fi

if selected 14; then
	req GET "/Users/$ALICE"
	etag=$(header ETag)
	req GET "/Users/$ALICE" "" -H "If-None-Match: $etag"
	report 14 MAY "RFC7644 §3.14" "If-None-Match with the current ETag returns 304" \
		"$(is '[ "$STATUS" = 304 ]')" \
		"304 Not Modified with no body" \
		"pkg/server/controller.go (ByID); see the skipped test in pkg/server/server_test.go"
fi

if selected 15; then
	get_query / filter "userName sw \"$PREFIX\""
	report 15 MAY "RFC7644 §3.4.2.1" "GET /?filter= queries every resource type" \
		"$(is '[ "$STATUS" = 200 ]')" \
		"200 ListResponse spanning Users and Groups" \
		"pkg/server/server.go (no route for the server root)"
fi

if selected 16; then
	req POST /.search "$(jq -cn --arg f "userName sw \"$PREFIX\"" '{schemas: ["urn:ietf:params:scim:api:messages:2.0:SearchRequest"], filter: $f}')"
	report 16 MAY "RFC7644 §3.4.3" "POST /.search at the server root" \
		"$(is '[ "$STATUS" = 200 ]')" \
		"200 ListResponse spanning every resource type" \
		"pkg/server/server.go"
fi

if selected 17; then
	NO_AUTH=1 req GET /ServiceProviderConfig
	report 17 MAY "RFC7644 §4" "discovery endpoints are readable without credentials" \
		"$(is '[ "$STATUS" = 200 ]')" \
		"200 without an Authorization header" \
		"cmd/server/main.go and pkg/server (authentication wraps every route)"
fi

if selected 18; then
	temp=$(create_user "$PREFIX-temp" "$PREFIX-temp@example.com")
	req PATCH "/Groups/$GROUP" "$(patch_json "$(jq -cn --arg m "$temp" '{op: "add", path: "members", value: [{value: $m}]}')")"
	req DELETE "/Users/$temp"
	req GET "/Groups/$GROUP"
	report 18 MAY "RFC7644 §3.6" "deleting a user removes it from group members" \
		"$(is '[ "$(jq --arg m "$temp" "[.members[]? | select(.value == \$m)] | length" <<<"$BODY")" = 0 ]')" \
		"the deleted user is no longer in members" \
		"pkg/server/service.go (Delete)"
fi

echo
echo "$PASSED passed, $FAILED required issues remaining, $FAILED_OPTIONAL optional issues remaining"
if [ "$FAILED" -gt 0 ] || { [ "${STRICT:-}" = 1 ] && [ "$FAILED_OPTIONAL" -gt 0 ]; }; then exit 1; fi
