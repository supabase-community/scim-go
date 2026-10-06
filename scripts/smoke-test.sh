#!/usr/bin/env bash
set -euo pipefail

PORT="${PORT:-8089}"
TOKEN="smoke"
BASE_URL="http://localhost:$PORT/scim/v2"
SEARCH='"schemas":["urn:ietf:params:scim:api:messages:2.0:SearchRequest"]'
PATCH='"schemas":["urn:ietf:params:scim:api:messages:2.0:PatchOp"]'

tmp=$(mktemp -d)
passed=0
failed=0

PORT="$PORT" SCIM_BEARER_TOKEN="$TOKEN" SCIM_BASE_URL="$BASE_URL" bin/scim-server >"$tmp/server.log" 2>&1 &
server=$!
trap 'kill "$server" 2>/dev/null; rm -rf "$tmp"' EXIT

for _ in $(seq 50); do
	curl -sf -o /dev/null "$BASE_URL/ServiceProviderConfig" && break
	sleep 0.1
done

call() {
	local method=$1 path=$2 data=${3:-}
	local args=(-sS -X "$method" -o "$tmp/body" -D "$tmp/headers" -w '%{http_code}' -H "Content-Type: application/scim+json")
	[[ -n $data ]] && args+=(--data "$data")
	for header in "${@:4}"; do args+=(-H "$header"); done
	[[ ${anonymous:-} ]] || args+=(-H "Authorization: Bearer $TOKEN")
	status=$(curl "${args[@]}" "$BASE_URL$path")
	body=$(<"$tmp/body")
}

header() {
	grep -i "^$1:" "$tmp/headers" | cut -d' ' -f2- | tr -d '\r' || true
}

field() {
	jq -rc "$1" <<<"$body"
}

uri() {
	jq -rn --arg v "$1" '$v|@uri'
}

expect() {
	local section=$1 name=$2 want=$3 got=$4
	if [[ $want == "$got" ]]; then
		passed=$((passed + 1))
		echo "PASS  $section  $name"
	else
		failed=$((failed + 1))
		echo "FAIL  $section  $name: want $want, got $got"
	fi
}

user() {
	call POST /Users "{\"userName\":\"$1\",\"active\":true,\"name\":{\"givenName\":\"$2\"},\"emails\":[{\"value\":\"$1@example.com\",\"type\":\"work\"}]}"
	field .id
}

echo "== discovery =="
anonymous=1 call GET /ServiceProviderConfig
expect "RFC 7644 4" "reads ServiceProviderConfig without a token" 200 "$status"
expect "RFC 7644 4" "advertises filter support" true "$(field .filter.supported)"
call GET /ResourceTypes
expect "RFC 7644 4" "lists User and Group resource types" '["Group","User"]' "$(field '[.Resources[].name] | sort')"
call GET /Schemas
expect "RFC 7643 7" "lists the User schema" true "$(field '[.Resources[].id] | index("urn:ietf:params:scim:schemas:core:2.0:User") != null')"

echo "== IdP provisioning =="
call POST /Users '{"userName":"alice","active":true,"name":{"givenName":"Alice"}}'
alice=$(field .id)
expect "RFC 7644 3.3" "creates a user" 201 "$status"
expect "RFC 7644 3.3" "returns the Location of the user" "$BASE_URL/Users/$alice" "$(header Location)"
expect "RFC 7644 3.14" "returns a weak ETag" 'W/"' "$(header ETag | cut -c1-3)"
call POST /Users '{"userName":"alice"}'
expect "RFC 7644 3.3" "rejects a duplicate userName" "409 uniqueness" "$status $(field .scimType)"
call GET "/Users?filter=$(uri 'userName eq "alice"')"
expect "RFC 7644 3.4.2.2" "finds the user by userName" "$alice" "$(field '.Resources[0].id')"
call GET "/Users/$alice"
expect "RFC 7644 3.4.1" "fetches the user by id" "200 alice" "$status $(field .userName)"
call PATCH "/Users/$alice" "{$PATCH,\"Operations\":[{\"op\":\"replace\",\"path\":\"active\",\"value\":false}]}"
expect "RFC 7644 3.5.2.3" "deactivates the user with PATCH" "200 false" "$status $(field .active)"
call PUT "/Users/$alice" '{"userName":"alice","active":true,"name":{"givenName":"Alicia"}}'
expect "RFC 7644 3.5.1" "replaces the user with PUT" "200 Alicia" "$status $(field .name.givenName)"
call DELETE "/Users/$alice"
expect "RFC 7644 3.6" "deletes the user" 204 "$status"
call GET "/Users/$alice"
expect "RFC 7644 3.6" "returns 404 for the deleted user" 404 "$status"

echo "== group membership =="
bob=$(user bob Bob)
carol=$(user carol Carol)
call POST /Groups '{"displayName":"Engineering"}'
group=$(field .id)
expect "RFC 7643 4.2" "creates a group" 201 "$status"
call PATCH "/Groups/$group" "{$PATCH,\"Operations\":[{\"op\":\"add\",\"path\":\"members\",\"value\":[{\"value\":\"$bob\"},{\"value\":\"$carol\"}]}]}"
expect "RFC 7644 3.5.2.1" "adds two members" 204 "$status"
call PATCH "/Groups/$group" "{$PATCH,\"Operations\":[{\"op\":\"remove\",\"path\":\"members[value eq \\\"$bob\\\"]\"}]}"
expect "RFC 7644 3.5.2.2" "removes one member by filter" 204 "$status"
call GET "/Groups/$group"
expect "RFC 7644 3.5.2" "keeps only the other member" "[\"$carol\"]" "$(field '[.members[].value]')"

echo "== query =="
user dave Dave >/dev/null
call GET "/Users?sortBy=userName&sortOrder=descending"
expect "RFC 7644 3.4.2.3" "sorts users descending" '["dave","carol","bob"]' "$(field '[.Resources[].userName]')"
call GET "/Users?sortBy=userName&startIndex=2&count=1"
expect "RFC 7644 3.4.2.4" "pages with startIndex and count" "3 2 1 carol" "$(field '"\(.totalResults) \(.startIndex) \(.itemsPerPage) \(.Resources[0].userName)"')"
call GET "/Users/$bob?attributes=userName"
expect "RFC 7644 3.4.2.5" "returns only the requested attributes" '["id","schemas","userName"]' "$(field 'keys')"
call GET "/Users/$bob?excludedAttributes=emails"
expect "RFC 7644 3.4.2.5" "drops excluded attributes" false "$(field 'has("emails")')"
call POST /Users/.search "{$SEARCH,\"filter\":\"userName sw \\\"c\\\"\",\"attributes\":[\"userName\"]}"
expect "RFC 7644 3.4.3" "searches users with POST" "200 1 carol" "$status $(field '"\(.totalResults) \(.Resources[0].userName)"')"
call POST /Groups/.search "{$SEARCH,\"filter\":\"displayName eq \\\"Engineering\\\"\"}"
expect "RFC 7644 3.4.3" "searches groups with POST" "200 $group" "$status $(field '.Resources[0].id')"
call POST /Users/.search '{"filter":"userName pr"}'
expect "RFC 7644 3.4.3" "rejects a search without the SearchRequest schema" "400 invalidSyntax" "$status $(field .scimType)"
call POST /.search "{$SEARCH}"
expect "RFC 7644 3.4.3" "declines a search from the root" 501 "$status"

echo "== concurrency and errors =="
call GET "/Users/$bob"
current=$(header ETag)
call PUT "/Users/$bob" '{"userName":"bob","active":false}' "If-Match: W/\"stale\""
expect "RFC 7644 3.14" "rejects a stale If-Match" 412 "$status"
call PUT "/Users/$bob" '{"userName":"bob","active":false}' "If-Match: $current"
expect "RFC 7644 3.14" "accepts the current If-Match" 200 "$status"
anonymous=1 call GET /Users
expect "RFC 7644 2" "rejects a request without a token" 401 "$status"
call GET /Users/does-not-exist
expect "RFC 7644 3.12" "returns 404 for an unknown id" 404 "$status"
call POST /Users '{"userName":'
expect "RFC 7644 3.12" "rejects malformed JSON" "400 invalidSyntax" "$status $(field .scimType)"
call GET "/Users?filter=$(uri 'userName zz "bob"')"
expect "RFC 7644 3.4.2.2" "rejects a bad filter" "400 invalidFilter" "$status $(field .scimType)"
call POST /Bulk '{}'
expect "RFC 7644 3.12" "declines Bulk" 501 "$status"
call GET /Me
expect "RFC 7644 3.12" "declines Me" 501 "$status"

echo "== $passed passed, $failed failed =="
[[ $failed -eq 0 ]]
