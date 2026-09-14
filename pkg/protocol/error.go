package protocol

import "github.com/supabase-community/scim-go/pkg/scimerrors"

type (
	Error     = scimerrors.Error
	ErrorType = scimerrors.ErrorType
)

const (
	ScimTypeInvalidFilter = scimerrors.InvalidFilter
	ScimTypeInvalidPath   = scimerrors.InvalidPath
	ScimTypeInvalidSyntax = scimerrors.InvalidSyntax
	ScimTypeInvalidValue  = scimerrors.InvalidValue
	ScimTypeInvalidVers   = scimerrors.InvalidVersion
	ScimTypeMutability    = scimerrors.Mutability
	ScimTypeNoTarget      = scimerrors.NoTarget
	ScimTypeSensitive     = scimerrors.Sensitive
	ScimTypeTooMany       = scimerrors.TooMany
	ScimTypeUniqueness    = scimerrors.Uniqueness
)

var (
	NewError          = scimerrors.NewError
	ErrInvalidFilter  = scimerrors.ErrInvalidFilter
	ErrTooMany        = scimerrors.ErrTooMany
	ErrInvalidSyntax  = scimerrors.ErrInvalidSyntax
	ErrInvalidPath    = scimerrors.ErrInvalidPath
	ErrNoTarget       = scimerrors.ErrNoTarget
	ErrInvalidValue   = scimerrors.ErrInvalidValue
	ErrMutability     = scimerrors.ErrMutability
	ErrUniqueness     = scimerrors.ErrUniqueness
	ErrSensitive      = scimerrors.ErrSensitive
	ErrNotFound       = scimerrors.ErrNotFound
	ErrUnauthorized   = scimerrors.ErrUnauthorized
	ErrForbidden      = scimerrors.ErrForbidden
	ErrTooLarge       = scimerrors.ErrTooLarge
	ErrNotImplemented = scimerrors.ErrNotImplemented
	ErrInternal       = scimerrors.ErrInternal
)
