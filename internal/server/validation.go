package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"

	"github.com/supabase-community/go-scim/pkg/core"
	"github.com/supabase-community/go-scim/pkg/protocol"
)

func validateUser(user *core.User) *protocol.Error {
	if user.UserName == "" {
		return protocol.ErrInvalidValue(`"userName" is required`)
	}
	if !slices.Contains(user.Schemas, core.SchemaUser) {
		return protocol.ErrInvalidValue(`"schemas" must include the User schema URN`)
	}
	return nil
}

func decodeUser(r *http.Request) (*core.User, error) {
	body, err := readBody(r)
	if err != nil {
		return nil, err
	}

	user := new(core.User)
	if err := json.Unmarshal(body, user); err != nil {
		return nil, protocol.ErrInvalidSyntax("request body is not a valid User")
	}
	return user, nil
}

func validateGroup(group *core.Group) *protocol.Error {
	if group.DisplayName == "" {
		return protocol.ErrInvalidValue(`"displayName" is required`)
	}
	if !slices.Contains(group.Schemas, core.SchemaGroup) {
		return protocol.ErrInvalidValue(`"schemas" must include the Group schema URN`)
	}
	return nil
}

func decodeGroup(r *http.Request) (*core.Group, error) {
	body, err := readBody(r)
	if err != nil {
		return nil, err
	}

	group := new(core.Group)
	if err := json.Unmarshal(body, group); err != nil {
		return nil, protocol.ErrInvalidSyntax("request body is not a valid Group")
	}
	return group, nil
}

func readBody(r *http.Request) ([]byte, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			return nil, protocol.ErrTooLarge("the request body is too large")
		}
		return nil, protocol.ErrInvalidSyntax("could not read the request body")
	}
	return body, nil
}
