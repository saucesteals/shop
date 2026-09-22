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

// output writes a command result in the selected presentation format.
func (c *CLI) output(v any) error {
	if c.text {
		return writeText(os.Stdout, v)
	}

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

// outputError writes an error in the selected format to stderr and returns
// the same exit code regardless of presentation.
func outputError(err error, text bool) int {
	var shopErr *shop.Error
	if !errors.As(err, &shopErr) {
		shopErr = trackingError(err)
		if shopErr == nil {
			shopErr = shop.Errorf(shop.ErrInternal, "%s", err)
		}
	}

	if text {
		fmt.Fprintf(os.Stderr, "Error [%s]: %s\n", cleanText(string(shopErr.Code)), cleanText(shopErr.Message))
		if retry, ok := shopErr.Details["retryAfter"]; ok {
			fmt.Fprintf(os.Stderr, "Retry after: %s seconds\n", cleanText(fmt.Sprint(retry)))
		}

		return shop.ExitCode(shopErr)
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
