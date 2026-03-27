package shop

import (
	"errors"
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

// NotImplemented returns a standard not-supported error for the given
// provider and operation. Shared across all provider stubs.
func NotImplemented(provider, op string) *Error {
	return &Error{
		Code:    ErrNotSupported,
		Message: provider + ": " + op + " not implemented",
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

	// System errors.
	ErrInternal    ErrorCode = "internal"
	ErrNetwork     ErrorCode = "network"
	ErrConfigError ErrorCode = "config_error"
)

// ExitCodes maps error codes to CLI exit codes.
var ExitCodes = map[ErrorCode]int{
	ErrAuthRequired:  10,
	ErrAuthExpired:   11,
	ErrAuthFailed:    12,
	ErrAuthTimeout:   13,
	ErrStoreNotFound: 20,
	ErrNotSupported:  21,
	ErrNotFound:      30,
	ErrOutOfStock:    31,
	ErrCartEmpty:     40,
	ErrCartChanged:   41,
	ErrQuantityLimit: 42,
	ErrRateLimited:   50,
	ErrStoreError:    51,
	ErrNetwork:       60,
	ErrInvalidInput:  2,
	ErrConfigError:   3,
	ErrInternal:      1,
}

// ExitCode returns the CLI exit code for the given error. Returns 1 for
// unknown errors.
func ExitCode(err error) int {
	var e *Error
	if errors.As(err, &e) {
		if code, ok := ExitCodes[e.Code]; ok {
			return code
		}
	}

	return 1
}

// IsAuthRequired reports whether err is an auth_required error.
func IsAuthRequired(err error) bool {
	var e *Error

	return errors.As(err, &e) && e.Code == ErrAuthRequired
}

// IsAuthExpired reports whether err is an auth_expired error.
func IsAuthExpired(err error) bool {
	var e *Error

	return errors.As(err, &e) && e.Code == ErrAuthExpired
}

// IsNotFound reports whether err is a not_found error.
func IsNotFound(err error) bool {
	var e *Error

	return errors.As(err, &e) && e.Code == ErrNotFound
}

// IsStoreNotFound reports whether err is a store_not_found error.
func IsStoreNotFound(err error) bool {
	var e *Error

	return errors.As(err, &e) && e.Code == ErrStoreNotFound
}

// IsNotSupported reports whether err is a not_supported error.
func IsNotSupported(err error) bool {
	var e *Error

	return errors.As(err, &e) && e.Code == ErrNotSupported
}

// IsCartChanged reports whether err is a cart_changed error.
func IsCartChanged(err error) bool {
	var e *Error

	return errors.As(err, &e) && e.Code == ErrCartChanged
}

// IsRateLimited reports whether err is a rate_limited error.
func IsRateLimited(err error) bool {
	var e *Error

	return errors.As(err, &e) && e.Code == ErrRateLimited
}
