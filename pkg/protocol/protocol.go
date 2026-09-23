// Package protocol implements the SCIM 2.0 protocol defined in RFC 7644.
package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

// MediaType is the SCIM media type registered in RFC 7644, Section 8.1.
const MediaType = "application/scim+json"

func Send(w http.ResponseWriter, status int, obj any) error {
	var body []byte
	if obj != nil {
		var err error
		if body, err = json.Marshal(obj); err != nil {
			return SendError(w, fmt.Errorf("scim: encoding %T: %w", obj, err))
		}
	}

	w.Header().Set("Content-Type", MediaType)
	w.WriteHeader(status)

	if len(body) == 0 {
		return nil
	}

	_, err := w.Write(body)
	return err
}

// SendError answers the request with err in the error form of RFC 7644, Section 3.12.
func SendError(w http.ResponseWriter, err error) error {
	if scimErr, ok := errors.AsType[*scimerrors.Error](err); ok {
		return Send(w, scimErr.StatusCode(), scimErr)
	}
	return errors.Join(err, Send(w, http.StatusInternalServerError, scimerrors.ErrInternal("Internal server error")))
}
