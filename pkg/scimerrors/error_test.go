package scimerrors_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

func TestNewError(t *testing.T) {
	t.Run("serializes to JSON correctly", func(t *testing.T) {
		body, err := json.Marshal(scimerrors.NewError(http.StatusNotFound, "", "Endpoint or resource does not exist"))

		require.NoError(t, err)
		assert.JSONEq(t, `{
			"schemas": ["urn:ietf:params:scim:api:messages:2.0:Error"],
			"status": "404",
			"detail": "Endpoint or resource does not exist"
		}`, string(body))
	})

	t.Run("includes the scimType when one is given", func(t *testing.T) {
		body, err := json.Marshal(scimerrors.NewError(http.StatusBadRequest, "invalidValue", "A required value was missing"))

		require.NoError(t, err)
		assert.JSONEq(t, `{
			"schemas": ["urn:ietf:params:scim:api:messages:2.0:Error"],
			"scimType": "invalidValue",
			"detail": "A required value was missing",
			"status": "400"
		}`, string(body))
	})

	t.Run("omits the optional attributes when they are empty", func(t *testing.T) {
		body, err := json.Marshal(scimerrors.NewError(http.StatusBadRequest, "", ""))

		require.NoError(t, err)
		assert.JSONEq(t, `{
			"schemas": ["urn:ietf:params:scim:api:messages:2.0:Error"],
			"status": "400"
		}`, string(body))
	})
}

func TestErrorIsAnError(t *testing.T) {
	t.Run("reports the status and the detail", func(t *testing.T) {
		var err error = scimerrors.NewError(http.StatusNotFound, "", "Endpoint or resource does not exist")

		require.EqualError(t, err, "scim: 404: Endpoint or resource does not exist")
	})

	t.Run("names the scimType when it carries one", func(t *testing.T) {
		var err error = scimerrors.ErrUniqueness(`userName "bjensen" is already in use`)

		require.EqualError(t, err, `scim: 409 uniqueness: userName "bjensen" is already in use`)
	})

	t.Run("reports the status alone when there is no detail", func(t *testing.T) {
		require.EqualError(t, scimerrors.NewError(http.StatusBadRequest, "", ""), "scim: 400")
		require.EqualError(t, scimerrors.ErrInvalidFilter(""), "scim: 400 invalidFilter")
	})

	t.Run("restates the wire status as a status code", func(t *testing.T) {
		assert.Equal(t, http.StatusConflict, scimerrors.ErrUniqueness("").StatusCode())
		assert.Equal(t, http.StatusInternalServerError, (&scimerrors.Error{Status: "not a number"}).StatusCode())
	})
}

func TestErrorIs(t *testing.T) {
	t.Run("matches another error of the same status and scimType", func(t *testing.T) {
		require.ErrorIs(t, scimerrors.ErrUniqueness("userName is already in use"), scimerrors.ErrUniqueness(""))
	})

	t.Run("distinguishes a different scimType", func(t *testing.T) {
		require.NotErrorIs(t, scimerrors.ErrInvalidPath("bad path"), scimerrors.ErrNoTarget(""))
	})

	t.Run("distinguishes a different status", func(t *testing.T) {
		require.NotErrorIs(t, scimerrors.ErrNotFound("no such user"), scimerrors.ErrNotImplemented(""))
	})

	t.Run("is recoverable from a wrapped error", func(t *testing.T) {
		wrapped := fmt.Errorf("listing users: %w", scimerrors.ErrTooMany("too many results"))

		var scimErr *scimerrors.Error
		require.ErrorAs(t, wrapped, &scimErr)
		assert.Equal(t, scimerrors.TooMany, scimErr.ScimType)
		require.ErrorIs(t, wrapped, scimerrors.ErrTooMany(""))
	})
}

func TestErrorConstructors(t *testing.T) {
	for _, tc := range []struct {
		name     string
		err      *scimerrors.Error
		status   int
		scimType scimerrors.ErrorType
		cite     string
	}{
		{"ErrInvalidFilter", scimerrors.ErrInvalidFilter(""), http.StatusBadRequest, scimerrors.InvalidFilter, "Section 3.4.2.2, Table 3"},
		{"ErrTooMany", scimerrors.ErrTooMany(""), http.StatusBadRequest, scimerrors.TooMany, "Section 3.4.2"},
		{"ErrTooLarge", scimerrors.ErrTooLarge(""), http.StatusRequestEntityTooLarge, "", "Section 3.12"},
		{"ErrInvalidSyntax", scimerrors.ErrInvalidSyntax(""), http.StatusBadRequest, scimerrors.InvalidSyntax, "Section 3.12"},
		{"ErrInvalidPath", scimerrors.ErrInvalidPath(""), http.StatusBadRequest, scimerrors.InvalidPath, "Section 3.5.2"},
		{"ErrNoTarget", scimerrors.ErrNoTarget(""), http.StatusBadRequest, scimerrors.NoTarget, "Section 3.5.2"},
		{"ErrInvalidValue", scimerrors.ErrInvalidValue(""), http.StatusBadRequest, scimerrors.InvalidValue, "Section 3.12, Table 8"},
		{"ErrMutability", scimerrors.ErrMutability(""), http.StatusBadRequest, scimerrors.Mutability, "Section 3.5.2"},
		{"ErrUniqueness", scimerrors.ErrUniqueness(""), http.StatusConflict, scimerrors.Uniqueness, "Section 3.3"},
		{"ErrSensitive", scimerrors.ErrSensitive(""), http.StatusForbidden, scimerrors.Sensitive, "Section 7.5.2"},
		{"ErrUnauthorized", scimerrors.ErrUnauthorized(""), http.StatusUnauthorized, "", "Section 3.12, Table 8"},
		{"ErrNotFound", scimerrors.ErrNotFound(""), http.StatusNotFound, "", "Section 3.12, Table 8"},
		{"ErrForbidden", scimerrors.ErrForbidden(""), http.StatusForbidden, "", "Section 3.12, Table 8"},
		{"ErrNotImplemented", scimerrors.ErrNotImplemented(""), http.StatusNotImplemented, "", "Section 3.12, Table 8"},
		{"ErrInternal", scimerrors.ErrInternal(""), http.StatusInternalServerError, "", "Section 3.12, Table 8"},
		{"ErrPreconditionFailed", scimerrors.ErrPreconditionFailed(""), http.StatusPreconditionFailed, "", "Section 3.14"},
	} {
		t.Run(tc.name+" pairs "+strconv.Itoa(tc.status)+" with "+string(tc.scimType), func(t *testing.T) {
			assert.Equal(t, tc.status, tc.err.StatusCode(), tc.cite)
			assert.Equal(t, strconv.Itoa(tc.status), tc.err.Status, tc.cite)
			assert.Equal(t, tc.scimType, tc.err.ScimType, tc.cite)
			assert.Equal(t, []core.SchemaURI{scimerrors.SchemaError}, tc.err.Schemas)
		})
	}

	t.Run("carries the detail it was given", func(t *testing.T) {
		assert.Equal(t, "Filtering is not supported on this endpoint", scimerrors.ErrInvalidFilter("Filtering is not supported on this endpoint").Detail)
	})
}
