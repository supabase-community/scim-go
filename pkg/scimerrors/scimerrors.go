package scimerrors

import "net/http"

// ErrInvalidFilter reports a filter this provider cannot honour, per Section 3.4.2.2.
func ErrInvalidFilter(detail string) *Error {
	return NewError(http.StatusBadRequest, InvalidFilter, detail)
}

// ErrTooMany reports a query whose result set is larger than this provider is willing to process, per Section 3.4.2.
func ErrTooMany(detail string) *Error {
	return NewError(http.StatusBadRequest, TooMany, detail)
}

// ErrInvalidSyntax reports a request body that does not conform to the request schema, per Section 3.12.
func ErrInvalidSyntax(detail string) *Error {
	return NewError(http.StatusBadRequest, InvalidSyntax, detail)
}

// ErrInvalidPath reports a malformed PATCH path, per Section 3.5.2.
func ErrInvalidPath(detail string) *Error {
	return NewError(http.StatusBadRequest, InvalidPath, detail)
}

// ErrNoTarget reports a PATCH path that yielded nothing to operate on, per Section 3.5.2.
func ErrNoTarget(detail string) *Error {
	return NewError(http.StatusBadRequest, NoTarget, detail)
}

// ErrInvalidValue reports a required value that is missing or unacceptable, per Section 3.12.
func ErrInvalidValue(detail string) *Error {
	return NewError(http.StatusBadRequest, InvalidValue, detail)
}

// ErrMutability reports a modification the target attribute does not allow, per Section 3.5.2.
func ErrMutability(detail string) *Error {
	return NewError(http.StatusBadRequest, Mutability, detail)
}

// ErrUniqueness reports a value already in use, per Section 3.3.
func ErrUniqueness(detail string) *Error {
	return NewError(http.StatusConflict, Uniqueness, detail)
}

// ErrSensitive reports a request that would disclose sensitive information in a URI, per Section 7.5.2.
func ErrSensitive(detail string) *Error {
	return NewError(http.StatusForbidden, Sensitive, detail)
}

// ErrNotFound is one of the errors of Table 8, Section 3.12 that carry no scimType.
func ErrNotFound(detail string) *Error {
	return NewError(http.StatusNotFound, "", detail)
}

// ErrUnauthorized is the 401 of Section 3.12
func ErrUnauthorized(detail string) *Error {
	return NewError(http.StatusUnauthorized, "", detail)
}

func ErrForbidden(detail string) *Error {
	return NewError(http.StatusForbidden, "", detail)
}

// ErrTooLarge reports a request body larger than this provider will accept, the 413 of Section 3.12.
func ErrTooLarge(detail string) *Error {
	return NewError(http.StatusRequestEntityTooLarge, "", detail)
}

func ErrNotImplemented(detail string) *Error {
	return NewError(http.StatusNotImplemented, "", detail)
}

func ErrInternal(detail string) *Error {
	return NewError(http.StatusInternalServerError, "", detail)
}

// ErrPreconditionFailed reports an If-Match precondition that did not hold, per Section 3.14.
func ErrPreconditionFailed(detail string) *Error {
	return NewError(http.StatusPreconditionFailed, "", detail)
}
