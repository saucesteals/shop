package tracking

import (
	"strings"
	"unicode"
)

// NormalizeNumber validates and normalizes a tracking identifier without losing leading zeroes.
func NormalizeNumber(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 100 || strings.ContainsFunc(value, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		return "", &Error{Kind: ErrInvalidInput, Message: "provide a tracking number containing only letters and digits"}
	}

	return value, nil
}
