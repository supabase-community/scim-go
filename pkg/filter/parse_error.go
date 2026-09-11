package filter

import (
	"errors"
	"fmt"
)

var ErrInvalidFilter = errors.New("scim: invalid filter")

type ParseError struct {
	Input    string
	Position int
}

func NewParseError(input string, position int) *ParseError {
	return &ParseError{
		Input:    input,
		Position: position,
	}
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("scim: invalid filter at position %d: %q", e.Position, e.Input)
}

func (e *ParseError) Unwrap() error {
	return ErrInvalidFilter
}
