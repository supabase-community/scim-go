package protocol_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

func TestSend(t *testing.T) {
	t.Run("writes a JSON response with a SCIM media type", func(t *testing.T) {
		w := httptest.NewRecorder()

		err := protocol.Send(w, http.StatusTeapot, map[string]string{"key": "value"})
		require.NoError(t, err)

		assert.Equal(t, http.StatusTeapot, w.Code)
		assert.Equal(t, "application/scim+json", w.Header().Get("Content-Type"))
		assert.JSONEq(t, `{"key":"value"}`, w.Body.String())
	})

	t.Run("writes the status alone when there is nothing to send", func(t *testing.T) {
		w := httptest.NewRecorder()

		require.NoError(t, protocol.Send(w, http.StatusNoContent, nil))

		assert.Equal(t, http.StatusNoContent, w.Code)
		assert.Empty(t, w.Body.String())
	})

	t.Run("answers with an internal error instead of an empty 200 when the value cannot be encoded", func(t *testing.T) {
		w := httptest.NewRecorder()

		require.Error(t, protocol.Send(w, http.StatusOK, func() {}))

		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.NotEmpty(t, w.Body.String())
	})
}

func TestSendError(t *testing.T) {
	t.Run("writes the error in the SCIM error form", func(t *testing.T) {
		w := httptest.NewRecorder()

		require.NoError(t, protocol.SendError(w, scimerrors.ErrUniqueness(`userName "bjensen" is already in use`)))

		assert.Equal(t, http.StatusConflict, w.Code)
		assert.Equal(t, protocol.MediaType, w.Header().Get("Content-Type"))
		assert.JSONEq(t, `{
			"schemas": ["urn:ietf:params:scim:api:messages:2.0:Error"],
			"scimType": "uniqueness",
			"detail": "userName \"bjensen\" is already in use",
			"status": "409"
		}`, w.Body.String())
	})

	t.Run("finds the SCIM error inside a wrapped error", func(t *testing.T) {
		w := httptest.NewRecorder()

		require.NoError(t, protocol.SendError(w, fmt.Errorf("reading users: %w", scimerrors.ErrNotFound("Endpoint or resource does not exist"))))

		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Contains(t, w.Body.String(), "Endpoint or resource does not exist")
	})

	t.Run("reports an error it does not recognise without disclosing it", func(t *testing.T) {
		w := httptest.NewRecorder()
		cause := errors.New("pq: password authentication failed for user")

		err := protocol.SendError(w, cause)

		require.ErrorIs(t, err, cause)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Equal(t, protocol.MediaType, w.Header().Get("Content-Type"))
		assert.NotContains(t, w.Body.String(), "password")
		assert.JSONEq(t, `{
			"schemas": ["urn:ietf:params:scim:api:messages:2.0:Error"],
			"detail": "Internal server error",
			"status": "500"
		}`, w.Body.String())
	})
}
