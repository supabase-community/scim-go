package scimerrors

import (
	"net/http"
	"strconv"

	"github.com/supabase-community/scim-go/pkg/core"
)

// SchemaError is the schema URN of the error message form, per RFC 7644, Section 3.12.
const SchemaError core.SchemaURI = "urn:ietf:params:scim:api:messages:2.0:Error"

// ErrorType is a detail error keyword from RFC 7644, Table 9.
type ErrorType string

const (
	InvalidFilter         ErrorType = "invalidFilter"
	ScimTypeInvalidPath   ErrorType = "invalidPath"
	ScimTypeInvalidSyntax ErrorType = "invalidSyntax"
	ScimTypeInvalidValue  ErrorType = "invalidValue"
	ScimTypeInvalidVers   ErrorType = "invalidVers"
	ScimTypeMutability    ErrorType = "mutability"
	ScimTypeNoTarget      ErrorType = "noTarget"
	ScimTypeSensitive     ErrorType = "sensitive"
	ScimTypeTooMany       ErrorType = "tooMany"
	ScimTypeUniqueness    ErrorType = "uniqueness"
)

// Error is the error message form defined in RFC 7644, Section 3.12.
type Error struct {
	Schemas  []core.SchemaURI `json:"schemas"`
	ScimType ErrorType        `json:"scimType,omitempty"`
	Detail   string           `json:"detail,omitempty"`
	Status   string           `json:"status"`
}

func NewError(status int, scimType ErrorType, detail string) *Error {
	return &Error{
		Schemas:  []core.SchemaURI{SchemaError},
		ScimType: scimType,
		Detail:   detail,
		Status:   strconv.Itoa(status),
	}
}

func (e *Error) Error() string {
	message := "scim: " + e.Status
	if e.ScimType != "" {
		message += " " + string(e.ScimType)
	}
	if e.Detail != "" {
		message += ": " + e.Detail
	}
	return message
}

func (e *Error) StatusCode() int {
	status, err := strconv.Atoi(e.Status)
	if err != nil {
		return http.StatusInternalServerError
	}
	return status
}

func (e *Error) Is(target error) bool {
	other, ok := target.(*Error)
	return ok && other.Status == e.Status && other.ScimType == e.ScimType
}
