package scimtest

import (
	"net/http"
	"testing"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

type creating struct {
	*resource
}

func (r creating) run(t *testing.T) {
	t.Helper()

	t.Run("returns 201 with a Location", r.checkLocation)
	t.Run("ignores a readOnly id", r.checkReadOnly)
	t.Run("ignores other readOnly attributes", r.checkReadOnlyAttributes)
	t.Run("assigns meta and ignores the client's", r.checkMeta)
	t.Run("rejects a duplicate unique value", r.checkUniqueness)
	t.Run("rejects a missing required attribute", r.checkRequired)
}

func (r creating) checkLocation(t *testing.T) {
	res, created := r.create(t, newUnique())
	location, _ := metaOf(created)["location"].(string)
	if got := res.Header.Get("Location"); got == "" || got != location {
		t.Errorf("scimtest: POST %s (RFC 7644 3.3): Location %q, want meta.location %q", r.kind.Endpoint, got, location)
	}
	r.expectLocated(t, res)
}

func (r creating) checkReadOnly(t *testing.T) {
	body := r.body(t, newUnique())
	body.Set("id", "scimtest-readonly")
	if _, created := r.createBody(t, body); idOf(created) == "scimtest-readonly" {
		t.Errorf("scimtest: POST %s (RFC 7644 3.3): kept the readOnly id from the request", r.kind.Endpoint)
	}
}

func (r creating) checkReadOnlyAttributes(t *testing.T) {
	attribute := r.attribute(t, "readOnly attribute", isReadOnly)
	marker := "scimtest-readonly-" + newUnique()
	body := r.body(t, newUnique())
	body.Set(attribute.Name, readOnlyValue(attribute, marker))

	_, created := r.createBody(t, body)
	expectIgnored(t, "POST "+r.kind.Endpoint+" (RFC 7644 3.3)", created.Get(attribute.Name), marker)
}

// RFC 7643 Section 3.1: "meta" SHALL be ignored when provided by clients.
func (r creating) checkMeta(t *testing.T) {
	forged := map[string]any{"resourceType": placeholder, "created": "2000-01-01T00:00:00Z", "lastModified": "2000-01-01T00:00:00Z", "location": "https://scimtest.invalid/scimtest", "version": `W/"scimtest"`}
	body := r.body(t, newUnique())
	body.Set("meta", forged)

	_, created := r.createBody(t, body)
	meta := metaOf(created)
	for key, value := range forged {
		if meta[key] == value {
			t.Errorf("scimtest: POST %s (RFC 7643 3.1): kept the client's meta.%s %v", r.kind.Endpoint, key, value)
		}
	}
	if kind, ok := meta["resourceType"]; ok && kind != string(r.kind.Name) {
		t.Errorf("scimtest: POST %s (RFC 7643 3.1): meta.resourceType %v, want %s", r.kind.Endpoint, kind, r.kind.Name)
	}
	if meta["created"] != meta["lastModified"] {
		t.Errorf("scimtest: POST %s (RFC 7643 3.1): meta.lastModified %v, want meta.created %v", r.kind.Endpoint, meta["lastModified"], meta["created"])
	}
}

func (r creating) checkUniqueness(t *testing.T) {
	unique := newUnique()
	body := r.body(t, unique)
	r.attribute(t, "unique attribute in its fixture", func(a *core.Attribute) bool { return isUnique(a) && body.Has(a.Name) })
	r.createBody(t, body)

	res, _ := r.post(t, r.body(t, unique))
	RequireError(t, res, http.StatusConflict, scimerrors.Uniqueness)
}

func (r creating) checkRequired(t *testing.T) {
	r.suite.should(t, "RFC 7644 3.12 400 invalidValue for a missing required attribute")
	body := r.body(t, newUnique())
	body.Remove(r.attribute(t, "required attribute in its fixture", required(body)).Name)

	res, _ := r.post(t, body)
	expectStatus(t, res, http.StatusBadRequest, "RFC 7644 3.12")
}
