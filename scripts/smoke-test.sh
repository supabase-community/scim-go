#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080/scim/v2}"

post_user() {
	curl -sS -X POST "$BASE_URL/Users" -H 'Content-Type: application/scim+json' -H "Authorization: Bearer $SCIM_BEARER_TOKEN" -d "$1"
}

filter() {
	curl -sS -G "$BASE_URL/Users" -H 'Content-Type: application/scim+json' -H "Authorization: Bearer $SCIM_BEARER_TOKEN" --data-urlencode "filter=$1"
}

echo "== create users =="
alice=$(post_user '{"userName":"alice","active":true,"name":{"givenName":"Alice","familyName":"Anderson"},"emails":[{"value":"alice@example.com","type":"work","primary":true}]}')
bob=$(post_user '{"userName":"bob","active":false,"name":{"givenName":"Bob","familyName":"Brown"},"emails":[{"value":"bob@example.com","type":"work"}]}')
carol=$(post_user '{"userName":"carol","active":true,"name":{"givenName":"Carol","familyName":"Clark"},"emails":[{"value":"carol@other.example","type":"home"}]}')

alice_id=$(jq -r .id <<<"$alice")
bob_id=$(jq -r .id <<<"$bob")

echo "== list all =="
curl -sS "$BASE_URL/Users" | jq '.Resources | length'

echo "== filter: userName eq \"alice\" =="
filter 'userName eq "alice"' | jq '.Resources[].userName'

echo "== filter: userName eq \"ALICE\" (case-insensitive by default) =="
filter 'userName eq "ALICE"' | jq '.Resources[].userName'

echo "== filter: active eq true =="
filter 'active eq true' | jq '.Resources[].userName'

echo "== filter: emails.value co \"@example.com\" (multi-valued 'any match') =="
filter 'emails.value co "@example.com"' | jq '.Resources[].userName'

echo "== filter: name.givenName eq \"Bob\" =="
filter 'name.givenName eq "Bob"' | jq '.Resources[].userName'

echo "== filter: userName pr =="
filter 'userName pr' | jq '.Resources | length'

echo "== get by id =="
curl -sS "$BASE_URL/Users/$alice_id" | jq '.userName'

echo "== replace: deactivate bob =="
curl -sS -X PUT "$BASE_URL/Users/$bob_id" -H 'Content-Type: application/scim+json' -d '{"userName":"bob","active":false,"name":{"givenName":"Bob","familyName":"Brown"}}' | jq '.active'

echo "== filter after update: active eq false =="
filter 'active eq false' | jq '.Resources[].userName'

echo "== delete carol =="
curl -sS -o /dev/null -w '%{http_code}\n' -X DELETE "$BASE_URL/Users/$(jq -r .id <<<"$carol")"

echo "== list after delete =="
curl -sS "$BASE_URL/Users" | jq '.Resources | length'
