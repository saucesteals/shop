package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/saucesteals/shop"
)

// isTTY reports whether the given file is a terminal.
func isTTY(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}

	return info.Mode()&os.ModeCharDevice != 0
}

// outputJSON writes v as JSON to stdout. Pretty-prints if the CLI is
// configured for it (explicit flag or TTY detection).
func (c *CLI) outputJSON(v any) error {
	pretty := c.pretty || (!c.jsonOutput && isTTY(os.Stdout))

	var data []byte
	var err error
	if pretty {
		data, err = json.MarshalIndent(v, "", "  ")
	} else {
		data, err = json.Marshal(v)
	}
	if err != nil {
		return shop.Errorf(shop.ErrInternal, "marshal output: %v", err)
	}

	_, err = fmt.Fprintln(os.Stdout, string(data))

	return err
}

// outputError writes a structured error to stderr and returns the
// appropriate exit code. Pretty-prints when stderr is a TTY.
func outputError(err error) int {
	var shopErr *shop.Error
	if !errors.As(err, &shopErr) {
		// Unknown errors are internal — not necessarily invalid input.
		shopErr = &shop.Error{
			Code:    shop.ErrInternal,
			Message: err.Error(),
		}
	}

	pretty := isTTY(os.Stderr)

	var data []byte
	var marshalErr error
	if pretty {
		data, marshalErr = json.MarshalIndent(shopErr, "", "  ")
	} else {
		data, marshalErr = json.Marshal(shopErr)
	}
	if marshalErr != nil {
		fmt.Fprintf(os.Stderr, `{"code":"internal","message":%q}`+"\n", err.Error())

		return 1
	}

	fmt.Fprintln(os.Stderr, string(data))

	return shop.ExitCode(shopErr)
}
