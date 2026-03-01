package git_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/lucasreiners/docker-cd/internal/git"
)

func TestValidationError_Error_WithoutCause(t *testing.T) {
	err := &git.ValidationError{
		Type:    git.ErrInvalidURL,
		Message: "invalid repository URL",
	}
	result := err.Error()
	if result != "invalid repository URL" {
		t.Errorf("expected 'invalid repository URL', got %q", result)
	}
}

func TestValidationError_Error_WithCause(t *testing.T) {
	err := &git.ValidationError{
		Type:    git.ErrAuthFailed,
		Message: "authentication failed",
		Cause:   errors.New("permission denied"),
	}
	result := err.Error()
	expected := "authentication failed: permission denied"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestValidationError_Unwrap_NoCause(t *testing.T) {
	err := &git.ValidationError{
		Type:    git.ErrInvalidURL,
		Message: "invalid URL",
	}
	cause := err.Unwrap()
	if cause != nil {
		t.Errorf("expected nil cause, got %v", cause)
	}
}

func TestValidationError_Unwrap_WithCause(t *testing.T) {
	originalErr := errors.New("denied")
	err := &git.ValidationError{
		Type:    git.ErrAuthFailed,
		Message: "auth failed",
		Cause:   originalErr,
	}
	cause := err.Unwrap()
	if cause != originalErr {
		t.Errorf("expected cause to be original error, got %v", cause)
	}
}

func TestValidationError_ErrorTypes(t *testing.T) {
	types := []git.ValidationErrorType{
		git.ErrMissingConfig,
		git.ErrInvalidURL,
		git.ErrAuthFailed,
		git.ErrRefNotFound,
		git.ErrPathNotFound,
		git.ErrUnknown,
	}
	for i, errType := range types {
		err := &git.ValidationError{
			Type:    errType,
			Message: "test error",
		}
		if err.Type != errType {
			t.Errorf("test %d: expected type %v, got %v", i, errType, err.Type)
		}
	}
}

func TestValidationError_ErrorChaining(t *testing.T) {
	originalErr := errors.New("network timeout")
	validationErr := &git.ValidationError{
		Type:    git.ErrAuthFailed,
		Message: "failed to connect",
		Cause:   originalErr,
	}
	if !errors.Is(validationErr, originalErr) {
		t.Error("expected errors.Is to find original error in chain")
	}
	unwrapped := errors.Unwrap(validationErr)
	if unwrapped != originalErr {
		t.Errorf("expected unwrapped error to be original, got %v", unwrapped)
	}
}

func TestValidationError_MessageFormatting(t *testing.T) {
	err := &git.ValidationError{
		Type:    git.ErrPathNotFound,
		Message: "deploy directory not found",
		Cause:   errors.New("path does not exist"),
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, "deploy directory not found") {
		t.Errorf("error message should contain main message, got %q", errMsg)
	}
	if !strings.Contains(errMsg, "path does not exist") {
		t.Errorf("error message should contain cause, got %q", errMsg)
	}
}
