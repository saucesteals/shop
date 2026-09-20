package shop

import (
	"errors"
	"github.com/saucesteals/shop/internal/fault"
)

// Error is the structured error shared by shopping and tracking clients.
type Error = fault.Error

// ErrorCode identifies an error category.
type ErrorCode = fault.ErrorCode

const (
	ErrAuthRequired  = fault.ErrAuthRequired
	ErrAuthExpired   = fault.ErrAuthExpired
	ErrAuthFailed    = fault.ErrAuthFailed
	ErrAuthTimeout   = fault.ErrAuthTimeout
	ErrNotFound      = fault.ErrNotFound
	ErrOutOfStock    = fault.ErrOutOfStock
	ErrCartEmpty     = fault.ErrCartEmpty
	ErrCartChanged   = fault.ErrCartChanged
	ErrQuantityLimit = fault.ErrQuantityLimit
	ErrStoreNotFound = fault.ErrStoreNotFound
	ErrNotSupported  = fault.ErrNotSupported
	ErrRateLimited   = fault.ErrRateLimited
	ErrStoreError    = fault.ErrStoreError
	ErrInvalidInput  = fault.ErrInvalidInput
	ErrUpstream      = fault.ErrUpstream
	ErrInternal      = fault.ErrInternal
	ErrNetwork       = fault.ErrNetwork
	ErrConfigError   = fault.ErrConfigError
)

// Errorf creates a structured error with a formatted message.
func Errorf(code ErrorCode, format string, args ...any) *Error {
	return fault.Errorf(code, format, args...)
}

// NotImplemented returns a standard not-supported error for the given
// provider and operation. Shared across all provider stubs.
func NotImplemented(provider, op string) *Error {
	return &Error{
		Code:    ErrNotSupported,
		Message: provider + ": " + op + " not implemented",
	}
}

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
	ErrUpstream:      51,
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
