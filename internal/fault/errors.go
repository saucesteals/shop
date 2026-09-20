package fault

import (
	"fmt"
)

// Error is the structured error type. Every error from every provider gets
// normalized into this shape before the CLI outputs it.
type Error struct {
	Code    ErrorCode      `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

// Error implements the error interface.
func (e *Error) Error() string { return e.Message }

// WithDetails returns a copy of the error with the given details merged into
// any existing details. New keys overwrite existing ones.
func (e *Error) WithDetails(details map[string]any) *Error {
	merged := make(map[string]any, len(e.Details)+len(details))
	for k, v := range e.Details {
		merged[k] = v
	}
	for k, v := range details {
		merged[k] = v
	}

	return &Error{
		Code:    e.Code,
		Message: e.Message,
		Details: merged,
	}
}

// Errorf creates a new Error with the given code and formatted message.
func Errorf(code ErrorCode, format string, args ...any) *Error {
	return &Error{
		Code:    code,
		Message: fmt.Sprintf(format, args...),
	}
}

// ErrorCode identifies the category of an error.
type ErrorCode string

const (
	// Auth errors.
	ErrAuthRequired ErrorCode = "auth_required"
	ErrAuthExpired  ErrorCode = "auth_expired"
	ErrAuthFailed   ErrorCode = "auth_failed"
	ErrAuthTimeout  ErrorCode = "auth_timeout"

	// Product/search errors.
	ErrNotFound   ErrorCode = "not_found"
	ErrOutOfStock ErrorCode = "out_of_stock"

	// Cart errors.
	ErrCartEmpty     ErrorCode = "cart_empty"
	ErrCartChanged   ErrorCode = "cart_changed"
	ErrQuantityLimit ErrorCode = "quantity_limit"

	// Store errors.
	ErrStoreNotFound ErrorCode = "store_not_found"
	ErrNotSupported  ErrorCode = "not_supported"
	ErrRateLimited   ErrorCode = "rate_limited"
	ErrStoreError    ErrorCode = "store_error"

	// Input errors.
	ErrInvalidInput ErrorCode = "invalid_input"

	// Upstream service errors.
	ErrUpstream ErrorCode = "upstream_error"

	// System errors.
	ErrInternal    ErrorCode = "internal"
	ErrNetwork     ErrorCode = "network"
	ErrConfigError ErrorCode = "config_error"
)
