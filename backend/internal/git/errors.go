package git

import "fmt"

// ValidationErrorType categorises repository validation failures.
type ValidationErrorType int

const (
	ErrMissingConfig ValidationErrorType = iota
	ErrInvalidURL
	ErrAuthFailed
	ErrRefNotFound
	ErrPathNotFound
	ErrUnknown
)

// ValidationError is returned when repository validation fails.
type ValidationError struct {
	Type    ValidationErrorType
	Message string
	Cause   error
}

func (e *ValidationError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

func (e *ValidationError) Unwrap() error {
	return e.Cause
}
