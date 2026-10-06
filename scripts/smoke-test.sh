#!/usr/bin/env bash
set -euo pipefail

PORT="${PORT:-8089}"
TOKEN="smoke"
BASE_URL="http://localhost:$PORT/scim/v2"
SEARCH='"schemas":["urn:ietf:params:scim:api:messages:2.0:SearchRequest"]'
PATCH='"schemas":["urn:ietf:params:scim:api:messages:2.0:PatchOp"]'
USER_URN="urn:ietf:params:scim:schemas:core:2.0:User"
ENTERPRISE="urn:ietf:params:scim:schemas:extension:enterprise:2.0:User"

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

has() {
	[[ $1 == *"$2"* ]] && echo true || echo false
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

group() {
	call POST /Groups "{\"displayName\":\"$1\"}"
	field .id
}

ops() {
	echo "{$PATCH,\"Operations\":[$1]}"
}

names() {
	call GET "/Users?sortBy=userName&filter=$(uri "$1")"
	field '[.Resources[]?.userName] | join(",")'
}

echo "== RFC 6750 2.1 Authorization Request Header Field =="
anonymous=1 call GET /Users "" "Authorization: Bearer $TOKEN"
expect "RFC 6750 2.1" "accepts a valid bearer token" 200 "$status"
anonymous=1 call GET /Users "" "Authorization: bearer $TOKEN"
expect "RFC 6750 2.1" "matches the Bearer scheme case-insensitively" 200 "$status"

echo "== RFC 6750 3 The WWW-Authenticate Response Header Field =="
anonymous=1 call GET /Users
expect "RFC 6750 3" "omits error info without a token" '401 Bearer realm="scim"' "$status $(header WWW-Authenticate)"
anonymous=1 call GET /Users "" "Authorization: Basic dXNlcjpwYXNz"
expect "RFC 6750 3" "omits error info for a non-Bearer scheme" '401 Bearer realm="scim"' "$status $(header WWW-Authenticate)"

echo "== RFC 6750 3.1 Error Codes =="
anonymous=1 call GET /Users "" "Authorization: Bearer wrong"
expect "RFC 6750 3.1" "rejects an invalid token with invalid_token" '401 Bearer realm="scim", error="invalid_token", error_description="The access token is invalid"' "$status $(header WWW-Authenticate)"
anonymous=1 call GET /Users "" "Authorization: Bearer"
expect "RFC 6750 3.1" "rejects a Bearer scheme without a token with invalid_request" "400 true" "$status $(has "$(header WWW-Authenticate)" 'error="invalid_request"')"

echo "== RFC 7643 2.1 Attributes =="
call POST /Users '{"USERNAME":"attr-case","Name":{"GivenName":"Case"}}'
expect "RFC 7643 2.1" "reads attribute names case-insensitively" "201 attr-case Case" "$status $(field .userName) $(field .name.givenName)"
call POST /Users '{"userName":"attr-dup","USERNAME":"attr-dup2"}'
expect "RFC 7643 2.1" "rejects an attribute name that repeats in another case" "400 invalidSyntax" "$status $(field .scimType)"

echo "== RFC 7643 2.2 Attribute Characteristics =="
call POST /Users '{"active":true}'
expect "RFC 7643 2.2" "rejects a resource missing a required attribute" "400 invalidValue" "$status $(field .scimType)"
call POST /Users '{"userName":"char-email","emails":[{"value":"char@example.com","type":"bogus"}]}'
expect "RFC 7643 2.2" "rejects an element value outside the canonical values" "400 invalidValue" "$status $(field .scimType)"
user char-unique Unique >/dev/null
call POST /Users '{"userName":"CHAR-UNIQUE"}'
expect "RFC 7643 2.2" "rejects a unique value that differs only in case" "409 uniqueness" "$status $(field .scimType)"

echo "== RFC 7643 2.3.6 Binary =="
call POST /Users '{"userName":"bin-bad","x509Certificates":[{"value":"!!!not base64"}]}'
expect "RFC 7643 2.3.6" "rejects a value that is not base64" "400 invalidValue" "$status $(field .scimType)"
call POST /Users '{"userName":"bin-unpadded","x509Certificates":[{"value":"YQ"}]}'
expect "RFC 7643 2.3.6" "accepts a value with its trailing padding omitted" 201 "$status"

echo "== RFC 7643 2.4 Multi-Valued Attributes =="
call POST /Users '{"userName":"multi-primary","emails":[{"value":"a@example.com","type":"work","primary":true},{"value":"b@example.com","type":"home","primary":true}]}'
expect "RFC 7643 2.4" "rejects more than one primary value" "400 invalidValue" "$status $(field .scimType)"

echo "== RFC 7643 2.5 Unassigned and Null Values =="
unassigned=$(user unassigned Unassigned)
call PUT "/Users/$unassigned" '{"userName":"unassigned"}'
expect "RFC 7643 2.5" "clears an attribute a replace leaves out" "200 false" "$status $(field 'has("emails")')"
call POST /Users '{"userName":"unassigned-false","active":false}'
expect "RFC 7643 2.5" "treats false as a value" "201 false" "$status $(field .active)"

echo "== RFC 7643 3 Common Attributes =="
call POST /Users '{"userName":"common","externalId":"ext-1"}'
expect "RFC 7643 3" "sets lastModified to created on create" true "$(field '.meta.created == .meta.lastModified')"
expect "RFC 7643 3" "keeps externalId" ext-1 "$(field .externalId)"
expect "RFC 7643 3" "sets meta.resourceType" User "$(field .meta.resourceType)"
call GET "/Users/$(field .id)"
expect "RFC 7643 3" "sets the Content-Location header to meta.location" "$(field .meta.location)" "$(header Content-Location)"
call POST /Users '{"id":"client-chosen","userName":"common-ro","meta":{"created":"2000-01-01T00:00:00Z"}}'
expect "RFC 7643 3" "ignores readOnly id and meta" "201 true" "$status $(field '.id != "client-chosen" and (.meta.created | startswith("2000") | not)')"

echo "== RFC 7643 4.2 Group Resource Schema =="
call POST /Groups '{"displayName":"Engineering"}'
expect "RFC 7643 4.2" "creates a group" 201 "$status"
call POST /Groups '{}'
expect "RFC 7643 4.2" "requires displayName" "400 invalidValue" "$status $(field .scimType)"

echo "== RFC 7643 4.3 Enterprise User Schema Extension =="
call POST /Users "{\"userName\":\"ent\",\"$ENTERPRISE\":{\"employeeNumber\":\"701\"}}"
expect "RFC 7643 4.3" "lists the extension in the resource's schemas" true "$(field ".schemas | index(\"$ENTERPRISE\") != null")"
call POST /Users '{"userName":"ent-none"}'
expect "RFC 7643 4.3" "omits the extension from schemas when unset" "[\"$USER_URN\"]" "$(field .schemas)"
call GET /ResourceTypes/User
expect "RFC 7643 4.3" "advertises the extension on the resource type" "$ENTERPRISE" "$(field '.schemaExtensions[0].schema')"
call GET "/Schemas/$ENTERPRISE"
expect "RFC 7643 4.3" "publishes the extension schema" "200 $ENTERPRISE" "$status $(field .id)"

echo "== RFC 7643 5 Service Provider Configuration Schema =="
anonymous=1 call GET /ServiceProviderConfig
expect "RFC 7643 5" "reads ServiceProviderConfig without a token" 200 "$status"
expect "RFC 7643 5" "advertises the bearer token scheme" oauthbearertoken "$(field '.authenticationSchemes[0].type')"
expect "RFC 7643 5" "advertises its capabilities" '[true,true,true,true,false,false]' "$(field '[.filter.supported,.sort.supported,.patch.supported,.etag.supported,.bulk.supported,.changePassword.supported]')"

echo "== RFC 7643 7 Schema Definition =="
call GET /Schemas
expect "RFC 7643 7" "lists the User, Group and EnterpriseUser schemas" true "$(field "[.Resources[].id] | contains([\"$USER_URN\",\"urn:ietf:params:scim:schemas:core:2.0:Group\",\"$ENTERPRISE\"])")"

echo "== RFC 7644 2 Authentication and Authorization =="
anonymous=1 call POST /Users '{"userName":"anonymous"}'
expect "RFC 7644 2" "rejects a request without a token" 401 "$status"

echo "== RFC 7644 3.3 Creating Resources =="
call POST /Users '{"userName":"alice","active":true,"name":{"givenName":"Alice"},"password":"t1meMa$heen","groups":[{"value":"g-1"}]}'
alice=$(field .id)
expect "RFC 7644 3.3" "creates a user" 201 "$status"
expect "RFC 7644 3.3" "returns the Location of the user" "$BASE_URL/Users/$alice" "$(header Location)"
expect "RFC 7644 3.3" "never returns the writeOnly password" false "$(field 'has("password")')"
expect "RFC 7644 3.3" "ignores readOnly groups" false "$(field 'has("groups")')"
call POST /Users '{"userName":"alice"}'
expect "RFC 7644 3.3" "rejects a duplicate userName" "409 uniqueness" "$status $(field .scimType)"
call POST /Users '{"userName":'
expect "RFC 7644 3.3" "rejects malformed JSON" "400 invalidSyntax" "$status $(field .scimType)"
call POST /Users '{"userName":"trailing"} {}'
expect "RFC 7644 3.3" "rejects data after the JSON value" "400 invalidSyntax" "$status $(field .scimType)"
call POST /Users 'null'
expect "RFC 7644 3.3" "rejects a JSON null body" "400 invalidSyntax" "$status $(field .scimType)"
call POST /Users '{"userName":"dup","userName":"dup2"}'
expect "RFC 7644 3.3" "rejects a duplicate attribute name" "400 invalidSyntax" "$status $(field .scimType)"

echo "== RFC 7644 3.4.1 Retrieving a Known Resource =="
call GET "/Users/$alice"
expect "RFC 7644 3.4.1" "fetches the user by id" "200 alice" "$status $(field .userName)"
expect "RFC 7644 3.4.1" "never returns the writeOnly password" false "$(field 'has("password")')"
call GET /Users/does-not-exist
expect "RFC 7644 3.4.1" "returns 404 for an unknown id" "404 404" "$status $(field .status)"

echo "== RFC 7644 3.4.2 Query Resources =="
call GET "/Users?filter=$(uri 'userName eq "nobody"')"
expect "RFC 7644 3.4.2" "returns an empty ListResponse" '200 ["urn:ietf:params:scim:api:messages:2.0:ListResponse"] 0' "$status $(field .schemas) $(field .totalResults)"
call GET "/Users?filter=$(uri 'userName eq "alice"')"
expect "RFC 7644 3.4.2" "finds the user by userName" "$alice" "$(field '.Resources[0].id')"

echo "== RFC 7644 3.4.2.2 Filtering =="
ann=$(user flt-ann Ann)
call PATCH "/Users/$ann" "$(ops "{\"op\":\"add\",\"value\":{\"$ENTERPRISE:employeeNumber\":\"1\"}}")"
call POST /Users '{"userName":"flt-ben","active":false}'
call POST /Users '{"userName":"flt-cat","active":true,"title":"Lead","emails":[{"value":"cat@example.org","type":"home"}]}'
scope='userName sw "flt-"'
expect "RFC 7644 3.4.2.2" "filters with eq" flt-ann "$(names 'userName eq "flt-ann"')"
expect "RFC 7644 3.4.2.2" "filters with co" flt-ben "$(names "$scope and userName co \"-b\"")"
expect "RFC 7644 3.4.2.2" "filters with sw" flt-ann,flt-ben,flt-cat "$(names "$scope")"
expect "RFC 7644 3.4.2.2" "filters with ew" flt-cat "$(names "$scope and userName ew \"cat\"")"
expect "RFC 7644 3.4.2.2" "filters with pr" flt-ann,flt-cat "$(names "$scope and emails pr")"
expect "RFC 7644 3.4.2.2" "filters with eq null" flt-ann,flt-ben "$(names "$scope and title eq null")"
expect "RFC 7644 3.4.2.2" "filters with ne on a boolean" flt-ben "$(names "$scope and active ne true")"
expect "RFC 7644 3.4.2.2" "combines clauses with or" flt-ann,flt-cat "$(names "$scope and (userName eq \"flt-ann\" or title eq \"Lead\")")"
expect "RFC 7644 3.4.2.2" "negates a clause with not" flt-ben "$(names "$scope and not (active eq true)")"
expect "RFC 7644 3.4.2.2" "filters a dateTime with gt" flt-ann,flt-ben,flt-cat "$(names "$scope and meta.lastModified gt \"2000-01-01T00:00:00Z\"")"
expect "RFC 7644 3.4.2.2" "filters a dateTime with lt" "" "$(names "$scope and meta.lastModified lt \"2000-01-01T00:00:00Z\"")"
expect "RFC 7644 3.4.2.2" "filters with a value path" flt-ann "$(names "$scope and emails[type eq \"work\"]")"
expect "RFC 7644 3.4.2.2" "filters by a URI-qualified extension attribute" flt-ann "$(names "$ENTERPRISE:employeeNumber eq \"1\"")"
expect "RFC 7644 3.4.2.2" "treats attribute names and operators as case insensitive" flt-ann "$(names 'USERNAME EQ "flt-ann"')"
call GET "/Users?filter=$(uri 'userName zz "bob"')"
expect "RFC 7644 3.4.2.2" "rejects a bad filter" "400 invalidFilter" "$status $(field .scimType)"

echo "== RFC 7644 3.4.2.3 Sorting =="
for name in srt-bob:3 srt-dave:1 srt-carol:2; do
	call POST /Users "{\"userName\":\"${name%:*}\",\"$ENTERPRISE\":{\"employeeNumber\":\"${name#*:}\"}}"
done
scope=$(uri 'userName sw "srt-"')
call GET "/Users?filter=$scope&sortBy=userName"
expect "RFC 7644 3.4.2.3" "sorts ascending by default" '["srt-bob","srt-carol","srt-dave"]' "$(field '[.Resources[].userName]')"
call GET "/Users?filter=$scope&sortBy=userName&sortOrder=descending"
expect "RFC 7644 3.4.2.3" "sorts users descending" '["srt-dave","srt-carol","srt-bob"]' "$(field '[.Resources[].userName]')"
call GET "/Users?filter=$scope&sortBy=$ENTERPRISE:employeeNumber"
expect "RFC 7644 3.4.2.3" "sorts by a URI-qualified extension attribute" '["srt-dave","srt-carol","srt-bob"]' "$(field '[.Resources[].userName]')"
call GET "/Users?sortBy=userName&sortOrder=sideways"
expect "RFC 7644 3.4.2.3" "rejects an unknown sortOrder" "400 invalidValue" "$status $(field .scimType)"
call GET "/Users?sortBy=bogus"
expect "RFC 7644 3.4.2.3" "rejects an unknown sortBy" "400 invalidValue" "$status $(field .scimType)"

echo "== RFC 7644 3.4.2.4 Pagination =="
call GET "/Users?filter=$scope&sortBy=userName&startIndex=2&count=1"
expect "RFC 7644 3.4.2.4" "pages with startIndex and count" "3 2 1 srt-carol" "$(field '"\(.totalResults) \(.startIndex) \(.itemsPerPage) \(.Resources[0].userName)"')"
call GET "/Users?filter=$scope&count=0"
expect "RFC 7644 3.4.2.4" "counts without returning resources" "3 0 0" "$(field '"\(.totalResults) \(.itemsPerPage) \(.Resources | length)"')"
call GET "/Users?startIndex=two"
expect "RFC 7644 3.4.2.4" "rejects a non-integer startIndex" "400 invalidValue" "$status $(field .scimType)"

echo "== RFC 7644 3.4.2.5 Attributes =="
bob=$(user bob Bob)
call GET "/Users/$bob?attributes=userName"
expect "RFC 7644 3.4.2.5" "returns only the requested attributes" '["id","schemas","userName"]' "$(field 'keys')"
call GET "/Users/$bob?excludedAttributes=emails"
expect "RFC 7644 3.4.2.5" "drops excluded attributes" false "$(field 'has("emails")')"
call GET "/Users/$bob?attributes=userName&excludedAttributes=emails"
expect "RFC 7644 3.4.2.5" "rejects attributes with excludedAttributes" 400 "$status"
call POST "/Users?attributes=userName" '{"userName":"shaped","title":"Lead"}'
expect "RFC 7644 3.4.2.5" "shapes the resource returned by a create" '201 ["id","schemas","userName"]' "$status $(field 'keys')"

echo "== RFC 7644 3.4.3 Querying Resources Using HTTP POST =="
carol=$(user carol Carol)
call POST /Users/.search "{$SEARCH,\"filter\":\"userName eq \\\"carol\\\"\",\"attributes\":[\"userName\"]}"
expect "RFC 7644 3.4.3" "searches users with POST" "200 1 carol" "$status $(field '"\(.totalResults) \(.Resources[0].userName)"')"
engineering=$(group Search)
call POST /Groups/.search "{$SEARCH,\"filter\":\"displayName eq \\\"Search\\\"\"}"
expect "RFC 7644 3.4.3" "searches groups with POST" "200 $engineering" "$status $(field '.Resources[0].id')"
call POST /Users/.search '{"filter":"userName pr"}'
expect "RFC 7644 3.4.3" "rejects a search without the SearchRequest schema" "400 invalidSyntax" "$status $(field .scimType)"
call POST /.search "{$SEARCH}"
expect "RFC 7644 3.4.3" "declines a search from the root" 501 "$status"

echo "== RFC 7644 3.5.1 Replacing with PUT =="
call GET "/Users/$alice"
before=$(header ETag)
created=$(field .meta.created)
call PUT "/Users/$alice" '{"userName":"alice","active":true,"name":{"givenName":"Alicia"}}' "If-Match: $before"
expect "RFC 7644 3.5.1" "replaces the user with PUT" "200 Alicia" "$status $(field .name.givenName)"
expect "RFC 7644 3.5.1" "returns a new ETag" true "$([[ $(header ETag) != "$before" ]] && echo true || echo false)"
expect "RFC 7644 3.5.1" "keeps meta.created" "$created" "$(field .meta.created)"
call PUT "/Users/$alice" '{"userName":"alice"}' "If-Match: $before"
expect "RFC 7644 3.5.1" "rejects a stale If-Match" 412 "$status"
call PUT /Users/does-not-exist '{"userName":"ghost"}'
expect "RFC 7644 3.5.1" "returns 404 for an unknown id" 404 "$status"
call PUT "/Users/$alice" '{"active":true}'
expect "RFC 7644 3.5.1" "rejects a replace missing a required attribute" "400 invalidValue" "$status $(field .scimType)"

echo "== RFC 7644 3.5.2 Modifying with PATCH =="
call PATCH "/Users/$alice" "$(ops '{"op":"replace","path":"active","value":false}')"
expect "RFC 7644 3.5.2" "deactivates the user with PATCH" "200 false" "$status $(field .active)"
expect "RFC 7644 3.5.2" "returns an ETag" "$(field .meta.version)" "$(header ETag)"
call PATCH "/Users/$alice" '{"Operations":[{"op":"replace","path":"active","value":true}]}'
expect "RFC 7644 3.5.2" "rejects a patch without the PatchOp schema" "400 invalidSyntax" "$status $(field .scimType)"
call PATCH "/Users/$alice" "$(ops '{"op":"replace","path":"id","value":"other"}')"
expect "RFC 7644 3.5.2" "rejects a patch that targets a readOnly attribute" "400 mutability" "$status $(field .scimType)"
call PATCH "/Users/$alice" "$(ops '{"op":"replace","path":"bogus","value":"x"}')"
expect "RFC 7644 3.5.2" "rejects a patch that targets an unknown attribute" "400 invalidPath" "$status $(field .scimType)"
call PATCH "/Users/$alice" "$(ops '{"op":"replace","path":"title","value":"Lead"},{"op":"replace","path":"bogus","value":"x"}')"
call GET "/Users/$alice"
expect "RFC 7644 3.5.2" "leaves the resource unchanged when a later operation fails" false "$(field 'has("title")')"
call PATCH "/Users/$alice" "$(ops "{\"op\":\"add\",\"path\":\"$ENTERPRISE:department\",\"value\":\"Tour\"}")"
expect "RFC 7644 3.5.2" "patches an extension attribute by its URN-qualified path" "200 Tour" "$status $(field ".\"$ENTERPRISE\".department")"
team=$(group Team)
call PATCH "/Groups/$team" "$(ops "{\"op\":\"add\",\"path\":\"members\",\"value\":[{\"value\":\"$bob\",\"type\":\"User\"},{\"value\":\"$carol\",\"type\":\"User\"}]}")"
expect "RFC 7644 3.5.2" "adds two members" 204 "$status"
call PATCH "/Groups/$team" "$(ops "{\"op\":\"replace\",\"path\":\"members[value eq \\\"$bob\\\"].type\",\"value\":\"Group\"}")"
expect "RFC 7644 3.5.2" "rejects a changed immutable member type" "400 mutability" "$status $(field .scimType)"

echo "== RFC 7644 3.5.2.1 Add Operation =="
call PATCH "/Users/$bob" "$(ops '{"op":"add","path":"emails","value":[{"value":"bob@example.org","type":"home"}]}')"
expect "RFC 7644 3.5.2.1" "adds a value" '["bob@example.com","bob@example.org"]' "$(field '[.emails[].value]')"
version=$(field .meta.version)
call PATCH "/Users/$bob" "$(ops '{"op":"add","path":"emails","value":[{"value":"bob@example.org","type":"home"}]}')"
expect "RFC 7644 3.5.2.1" "does not append a value already present" "2 $version" "$(field '.emails | length') $(field .meta.version)"
call PATCH "/Users/$bob" "$(ops '{"op":"add","value":{"title":"Lead","nickName":"Bobby"}}')"
expect "RFC 7644 3.5.2.1" "adds the attributes of the value when the path is omitted" "200 Lead Bobby" "$status $(field .title) $(field .nickName)"

echo "== RFC 7644 3.5.2.2 Remove Operation =="
call PATCH "/Groups/$team" "$(ops "{\"op\":\"remove\",\"path\":\"members[value eq \\\"$bob\\\"]\"}")"
expect "RFC 7644 3.5.2.2" "removes one member by filter" 204 "$status"
call GET "/Groups/$team"
expect "RFC 7644 3.5.2.2" "keeps only the other member" "[\"$carol\"]" "$(field '[.members[].value]')"
call PATCH "/Users/$bob" "$(ops '{"op":"remove","path":"title","value":"Lead"}')"
expect "RFC 7644 3.5.2.2" "rejects a remove that carries a value" "400 invalidSyntax" "$status $(field .scimType)"
call PATCH "/Users/$bob" "$(ops '{"op":"remove","path":"userName"}')"
expect "RFC 7644 3.5.2.2" "rejects a remove of a required attribute" "400 mutability" "$status $(field .scimType)"
call PATCH "/Users/$bob" "$(ops '{"op":"remove"}')"
expect "RFC 7644 3.5.2.2" "rejects a remove without a path" "400 noTarget" "$status $(field .scimType)"

echo "== RFC 7644 3.5.2.3 Replace Operation =="
call PATCH "/Users/$bob" "$(ops '{"op":"replace","path":"name","value":{"familyName":"Builder"}}')"
expect "RFC 7644 3.5.2.3" "merges the sub-attributes of a complex attribute" "Bob Builder" "$(field .name.givenName) $(field .name.familyName)"
call PATCH "/Users/$bob" "$(ops '{"op":"replace","path":"emails[type eq \"other\"].value","value":"x@example.com"}')"
expect "RFC 7644 3.5.2.3" "rejects a value path that matches nothing" "400 noTarget" "$status $(field .scimType)"

echo "== RFC 7644 3.6 Deleting Resources =="
call DELETE "/Users/$alice" "" "If-Match: W/\"stale\""
expect "RFC 7644 3.6" "rejects a delete with a stale If-Match" 412 "$status"
call DELETE "/Users/$alice"
expect "RFC 7644 3.6" "deletes the user" 204 "$status"
call GET "/Users/$alice"
expect "RFC 7644 3.6" "returns 404 for the deleted user" 404 "$status"

echo "== RFC 7644 3.7 Bulk Operations =="
call POST /Bulk '{}'
expect "RFC 7644 3.7" "declines Bulk" "501 501" "$status $(field .status)"

echo "== RFC 7644 3.8 Data Input/Output Formats =="
call POST /Users '{"userName":"plain-json"}' "Content-Type: application/json" "Accept: application/json"
expect "RFC 7644 3.8" "accepts application/json and answers with application/scim+json" "201 application/scim+json" "$status $(header Content-Type)"

echo "== RFC 7644 3.11 /Me Authenticated Subject Alias =="
call GET /Me
expect "RFC 7644 3.11" "declines Me" "501 501" "$status $(field .status)"

echo "== RFC 7644 3.12 HTTP Status and Error Response Handling =="
call GET /Unknown
expect "RFC 7644 3.12" "returns a SCIM error for an unknown endpoint" "404 404" "$status $(field .status)"
call DELETE /ServiceProviderConfig
expect "RFC 7644 3.12" "returns 405 with the allowed methods" "405 405 true" "$status $(field .status) $([[ -n $(header Allow) ]] && echo true || echo false)"

echo "== RFC 7644 3.14 Versioning Resources =="
call GET "/Users/$bob"
current=$(header ETag)
expect "RFC 7644 3.14" "returns a weak ETag" 'W/"' "${current:0:3}"
expect "RFC 7644 3.14" "meta.version matches the ETag" "$current" "$(field .meta.version)"
call PUT "/Users/$bob" '{"userName":"bob","active":false}' "If-Match: W/\"stale\""
expect "RFC 7644 3.14" "rejects a stale If-Match" 412 "$status"
call PUT "/Users/$bob" '{"userName":"bob","active":false}' "If-Match: $current"
expect "RFC 7644 3.14" "accepts the current If-Match" 200 "$status"
call PUT "/Users/$bob" '{"userName":"bob","active":true}' "If-Match: *"
expect "RFC 7644 3.14" "If-Match * matches any version" 200 "$status"

echo "== RFC 7644 4 Service Provider Configuration Endpoints =="
anonymous=1 call GET /ServiceProviderConfig
expect "RFC 7644 4" "advertises filter support" true "$(field .filter.supported)"
expect "RFC 7644 4" "advertises its own location" "$BASE_URL/ServiceProviderConfig" "$(field .meta.location)"
call GET /ResourceTypes
expect "RFC 7644 4" "lists User and Group resource types" '["Group","User"]' "$(field '[.Resources[].name] | sort')"
call GET /ResourceTypes/Group
expect "RFC 7644 4" "fetches a resource type by id" "200 /Groups" "$status $(field .endpoint)"
call GET /ResourceTypes/Bogus
expect "RFC 7644 4" "returns 404 for an unknown resource type" 404 "$status"
call GET "/Schemas/$USER_URN"
expect "RFC 7644 4" "fetches a schema by id" "200 User" "$status $(field .name)"
call GET /Schemas/urn:bogus
expect "RFC 7644 4" "returns 404 for an unknown schema" 404 "$status"
call GET "/Schemas?filter=$(uri 'id eq "x"')"
expect "RFC 7644 4" "rejects a filter on /Schemas" 403 "$status"
call GET "/ResourceTypes?filter=$(uri 'id eq "x"')"
expect "RFC 7644 4" "rejects a filter on /ResourceTypes" 403 "$status"

echo "== RFC 7644 7.5.2 Disclosure of Sensitive Information in URIs =="
call GET "/Users?filter=$(uri 'password eq "hunter2"')"
expect "RFC 7644 7.5.2" "rejects a GET filter on the password" "403 sensitive false" "$status $(field .scimType) $(has "$body" hunter2)"

echo "== $passed passed, $failed failed =="
[[ $failed -eq 0 ]]
