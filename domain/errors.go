package domain

import "fmt"

// ErrorCode is a stable machine-readable registry error code.
type ErrorCode string

const (
	ErrInvalidRequest ErrorCode = "invalid_request"
	ErrUnauthorized   ErrorCode = "unauthorized"
	ErrForbidden      ErrorCode = "forbidden"
	ErrNotFound       ErrorCode = "tool_not_found"
	ErrSchemaInvalid  ErrorCode = "schema_invalid"
	ErrUpstream       ErrorCode = "upstream_error"
	ErrNotImplemented ErrorCode = "not_implemented"
)

// RegistryError is a domain error carrying a code and an HTTP-mappable status.
type RegistryError struct {
	Code    ErrorCode
	Status  int
	Message string
}

func (e *RegistryError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// NewError builds a RegistryError.
func NewError(code ErrorCode, status int, msg string) *RegistryError {
	return &RegistryError{Code: code, Status: status, Message: msg}
}

var _ error = (*RegistryError)(nil)
