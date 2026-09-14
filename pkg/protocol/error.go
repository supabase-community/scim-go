package protocol

import "github.com/supabase-community/scim-go/pkg/scimerrors"

type (
	Error     = scimerrors.Error
	ErrorType = scimerrors.ErrorType
)

const (
	ScimTypeInvalidFilter = scimerrors.ScimTypeInvalidFilter
	ScimTypeInvalidPath   = scimerrors.ScimTypeInvalidPath
	ScimTypeInvalidSyntax = scimerrors.ScimTypeInvalidSyntax
	ScimTypeInvalidValue  = scimerrors.ScimTypeInvalidValue
	ScimTypeInvalidVers   = scimerrors.ScimTypeInvalidVers
	ScimTypeMutability    = scimerrors.ScimTypeMutability
	ScimTypeNoTarget      = scimerrors.ScimTypeNoTarget
	ScimTypeSensitive     = scimerrors.ScimTypeSensitive
	ScimTypeTooMany       = scimerrors.ScimTypeTooMany
	ScimTypeUniqueness    = scimerrors.ScimTypeUniqueness
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
