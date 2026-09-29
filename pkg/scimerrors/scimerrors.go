package scimerrors

import "net/http"

// ErrInvalidFilter is the invalidFilter error of RFC 7644 Section 3.4.2.2.
func ErrInvalidFilter(detail string) *Error {
	return NewError(http.StatusBadRequest, InvalidFilter, detail)
}

// ErrTooMany is the tooMany error of RFC 7644 Section 3.4.2.1.
func ErrTooMany(detail string) *Error {
	return NewError(http.StatusBadRequest, TooMany, detail)
}

// ErrInvalidSyntax is the invalidSyntax error of RFC 7644 Section 3.12, Table 9.
func ErrInvalidSyntax(detail string) *Error {
	return NewError(http.StatusBadRequest, InvalidSyntax, detail)
}

// ErrInvalidPath is the invalidPath error of RFC 7644 Section 3.12, Table 9.
func ErrInvalidPath(detail string) *Error {
	return NewError(http.StatusBadRequest, InvalidPath, detail)
}

// ErrNoTarget is the noTarget error of RFC 7644 Section 3.5.2.
func ErrNoTarget(detail string) *Error {
	return NewError(http.StatusBadRequest, NoTarget, detail)
}

// ErrInvalidValue is the invalidValue error of RFC 7644 Section 3.12, Table 9.
func ErrInvalidValue(detail string) *Error {
	return NewError(http.StatusBadRequest, InvalidValue, detail)
}

// ErrMutability is the mutability error of RFC 7644 Sections 3.5.1 and 3.5.2.
func ErrMutability(detail string) *Error {
	return NewError(http.StatusBadRequest, Mutability, detail)
}

// ErrUniqueness is the 409 uniqueness error of RFC 7644 Section 3.3.
func ErrUniqueness(detail string) *Error {
	return NewError(http.StatusConflict, Uniqueness, detail)
}

// ErrSensitive is the 403 sensitive error of RFC 7644 Section 7.5.2.
func ErrSensitive(detail string) *Error {
	return NewError(http.StatusForbidden, Sensitive, detail)
}

// ErrNotFound is the 404 of RFC 7644 Section 3.12, Table 8.
func ErrNotFound(detail string) *Error {
	return NewError(http.StatusNotFound, "", detail)
}

// ErrUnauthorized is the 401 of RFC 7644 Section 3.12, Table 8.
func ErrUnauthorized(detail string) *Error {
	return NewError(http.StatusUnauthorized, "", detail)
}

func ErrForbidden(detail string) *Error {
	return NewError(http.StatusForbidden, "", detail)
}

// ErrTooLarge is the 413 of RFC 7644 Section 3.12, Table 8.
func ErrTooLarge(detail string) *Error {
	return NewError(http.StatusRequestEntityTooLarge, "", detail)
}

func ErrNotImplemented(detail string) *Error {
	return NewError(http.StatusNotImplemented, "", detail)
}

func ErrInternal(detail string) *Error {
	return NewError(http.StatusInternalServerError, "", detail)
}

// ErrPreconditionFailed is the 412 of RFC 7644 Section 3.12, Table 8, for a failed If-Match (Section 3.14).
func ErrPreconditionFailed(detail string) *Error {
	return NewError(http.StatusPreconditionFailed, "", detail)
}
