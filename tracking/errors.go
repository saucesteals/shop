package tracking

import "errors"

// Error categories support errors.Is independently of a carrier's wording.
var (
	ErrInvalidInput = errors.New("invalid tracking input")
	ErrNotSupported = errors.New("tracking carrier is not supported")
	ErrRateLimited  = errors.New("tracking source rate limited")
	ErrUpstream     = errors.New("carrier did not provide usable history")
	ErrNetwork      = errors.New("tracking request failed")
	ErrInternal     = errors.New("tracking client error")
)

// Error describes a tracking failure. Kind supports errors.Is; HTTP and carrier
// diagnostics are typed fields, independent of CLI JSON and exit codes.
type Error struct {
	Kind       error
	Message    string
	StatusCode int
	RetryAfter string
	Reason     string
}

// Error returns the operation's message, or the category when none was supplied.
func (e *Error) Error() string {
	if e.Message != "" {
		return e.Message
	}

	if e.Kind != nil {
		return e.Kind.Error()
	}

	return "tracking error"
}

// Is reports whether the failure belongs to the requested category.
func (e *Error) Is(target error) bool { return errors.Is(e.Kind, target) }
