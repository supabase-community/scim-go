package scimtest

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"slices"

	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

func Decode[T any](t TB, res *http.Response) T {
	t.Helper()

	var value T
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("scimtest: reading %s %s: %v", res.Request.Method, res.Request.URL.Path, err)
		return value
	}
	res.Body = io.NopCloser(bytes.NewReader(body))
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatalf("scimtest: decoding %s %s into %T: %v\n%s", res.Request.Method, res.Request.URL.Path, value, err, body)
	}
	return value
}

// RequireError checks the error response form of RFC 7644, Section 3.12.
func RequireError(t TB, res *http.Response, status int, scimType scimerrors.ErrorType) scimerrors.Error {
	t.Helper()

	where := res.Request.Method + " " + res.Request.URL.Path
	if res.StatusCode != status {
		t.Fatalf("scimtest: %s: status %d, want %d", where, res.StatusCode, status)
		return scimerrors.Error{}
	}
	if mediaType, _, _ := mime.ParseMediaType(res.Header.Get("Content-Type")); mediaType != protocol.MediaType {
		t.Fatalf("scimtest: %s: Content-Type %q, want %q", where, mediaType, protocol.MediaType)
		return scimerrors.Error{}
	}

	body := Decode[scimerrors.Error](t, res)
	if !slices.Contains(body.Schemas, protocol.SchemaError) {
		t.Fatalf("scimtest: %s: schemas %v, want %s", where, body.Schemas, protocol.SchemaError)
	}
	if scimType != "" && body.ScimType != scimType {
		t.Fatalf("scimtest: %s: scimType %q, want %q", where, body.ScimType, scimType)
	}
	return body
}
