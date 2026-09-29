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
	InvalidFilter  ErrorType = "invalidFilter"
	InvalidPath    ErrorType = "invalidPath"
	InvalidSyntax  ErrorType = "invalidSyntax"
	InvalidValue   ErrorType = "invalidValue"
	InvalidVersion ErrorType = "invalidVers"
	Mutability     ErrorType = "mutability"
	NoTarget       ErrorType = "noTarget"
	Sensitive      ErrorType = "sensitive"
	TooMany        ErrorType = "tooMany"
	Uniqueness     ErrorType = "uniqueness"
)

var descriptions = map[ErrorType]string{
	InvalidFilter:  "The specified filter syntax was invalid, or the specified attribute and filter comparison combination is not supported.",
	InvalidPath:    `The "path" attribute was invalid or malformed.`,
	InvalidSyntax:  "The request body message structure was invalid or did not conform to the request schema.",
	InvalidValue:   "A required value was missing, or the value specified was not compatible with the operation or attribute type, or resource schema.",
	InvalidVersion: "The specified SCIM protocol version is not supported.",
	Mutability:     "The attempted modification is not compatible with the target attribute's mutability or current state.",
	NoTarget:       `The specified "path" did not yield an attribute or attribute value that could be operated on.`,
	Sensitive:      "The specified request cannot be completed, due to the passing of sensitive (e.g., personal) information in a request URI.",
	TooMany:        "The specified filter yields many more results than the server is willing to calculate or process.",
	Uniqueness:     "One or more of the attribute values are already in use or are reserved.",
}

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

// Description returns the RFC 7644 Table 9 description of the detail error keyword.
func (t ErrorType) Description() string {
	return descriptions[t]
}
