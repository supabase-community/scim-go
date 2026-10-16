package scimtest

import (
	"net/http"
	"slices"
	"strings"
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
	t.Run("accepts a repeated value that is not unique", r.checkNotUnique)
	t.Run("rejects a missing required attribute", r.checkRequired)
	t.Run("rejects an empty required attribute", r.checkEmptyRequired)
	t.Run("rejects a unique value in another case", r.checkUniqueCase)
	t.Run("accepts schemas in any order", r.checkSchemasOrder)
	t.Run("rejects a value of the wrong type", r.checkMistyped)
	t.Run("stores every value of a full resource", r.checkFull)
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

// RFC 7644 Section 3.3: readOnly attributes in the request body SHALL be ignored.
func (r creating) checkReadOnlyAttributes(t *testing.T) {
	for _, o := range readOnlyIn(t, r.resource) {
		t.Run(o.path(), func(t *testing.T) {
			marker := "scimtest-readonly-" + newUnique()
			body := r.body(t, newUnique())
			o.set(body, marker)
			body.Set(schemasAttr, r.schemaURIs(body))

			_, created := r.createBody(t, body)
			expectIgnored(t, "POST "+r.kind.Endpoint+" (RFC 7644 3.3)", created, marker)
			expectIgnored(t, "GET "+r.path(created)+" (RFC 7644 3.3)", r.read(t, created), marker)
		})
	}
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
	if kind, ok := meta["resourceType"]; (ok || r.suite.strict) && kind != string(r.kind.Name) {
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

func (r creating) checkNotUnique(t *testing.T) {
	r.suite.should(t, "RFC 7643 2.2 a uniqueness none value is accepted twice")
	value := placeholder + " " + newUnique()
	r.createBody(t, r.sharing(t, newUnique(), value))

	r.createBody(t, r.sharing(t, newUnique(), value))
}

func (r creating) checkRequired(t *testing.T) {
	r.suite.should(t, "RFC 7644 Table 9 400 invalidValue for a missing required attribute")
	body := r.body(t, newUnique())
	body.Remove(r.attribute(t, "required attribute in its fixture", required(body)).Name)

	res, _ := r.post(t, body)
	RequireError(t, res, http.StatusBadRequest, scimerrors.InvalidValue)
}

func (r creating) checkEmptyRequired(t *testing.T) {
	r.suite.should(t, "RFC 7644 Table 9 400 invalidValue for an empty required attribute")
	body := r.body(t, newUnique())
	attribute := r.attribute(t, "required string attribute in its fixture", func(a *core.Attribute) bool {
		return required(body)(a) && a.Type == core.TypeString && !a.MultiValued
	})
	body.Set(attribute.Name, "")

	res, _ := r.post(t, body)
	RequireError(t, res, http.StatusBadRequest, scimerrors.InvalidValue)
}

// RFC 7644 Section 3.3: a duplicate MUST be 409; RFC 7643 Section 2.2: caseExact false compares without case.
func (r creating) checkUniqueCase(t *testing.T) {
	body := r.body(t, newUnique())
	attribute := r.attribute(t, "case-insensitive unique string attribute in its fixture", func(a *core.Attribute) bool {
		return isUnique(a) && !a.CaseExact && a.Type == core.TypeString && !a.MultiValued && body.Has(a.Name)
	})
	r.createBody(t, body)
	value, _ := body.Get(attribute.Name).(string)
	duplicate := r.body(t, newUnique())
	duplicate.Set(attribute.Name, strings.ToUpper(value))

	res, _ := r.post(t, duplicate)
	RequireError(t, res, http.StatusConflict, scimerrors.Uniqueness)
}

// RFC 7643 Section 3: the order of "schemas" MUST NOT impact behavior.
func (r creating) checkSchemasOrder(t *testing.T) {
	extension, attribute := r.extended(t, isFilterable)
	value := "scimtest " + newUnique()
	body := r.body(t, newUnique())
	r.withExtension(body, extension, attribute.Name, value)
	uris := r.schemaURIs(body)
	slices.Reverse(uris)
	body.Set(schemasAttr, uris)

	_, created := r.createBody(t, body)
	if got := core.Object(objectOf(r.read(t, created).Get(string(extension.ID)))).Get(attribute.Name); got != value {
		t.Errorf("scimtest: POST %s (RFC 7643 3): with schemas %v, %s:%s is %v, want %v", r.kind.Endpoint, uris, extension.ID, attribute.Name, got, value)
	}
}

func (r creating) checkFull(t *testing.T) {
	r.suite.should(t, "RFC 7643 2.2 readWrite and immutable values are stored and returned")
	body := r.fullBody(t)
	_, created := r.createBody(t, body)
	stored := keysOf(storedLeaves(r.schemas, r.read(t, created)))

	for _, sent := range storedLeaves(r.schemas, body) {
		if !stored[sent.key()] {
			t.Errorf("scimtest: GET %s (RFC 7643 2.2): %s %s is missing", r.path(created), sent.path, sent.literal())
		}
	}
}

func (r creating) checkMistyped(t *testing.T) {
	r.suite.should(t, "RFC 7644 Table 9 400 invalidValue for a value of the wrong type")
	body := r.body(t, newUnique())
	r.mistype(t, body)

	expectTypeError(t, r.client().Post(t, r.kind.Endpoint, body))
}
