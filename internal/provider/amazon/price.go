package amazon

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/saucesteals/shop"
)

// priceRe matches price strings with common currency symbols ($, £, €, ¥).
// Handles symbol-first formats ($29.99, £29.99, €29,99, ¥2999) and
// symbol-last formats (29,99 €).
var priceRe = regexp.MustCompile(`[$£€¥]\s*([\d.,]+)|([\d.,]+)\s*[$£€¥]`)

// parsePriceCents converts a formatted price string to minor currency units
// (cents for USD/GBP/EUR/CAD/AUD, whole yen for JPY). The currency
// parameter controls decimal separator handling:
//   - EUR: comma is decimal separator ("29,99" → 2999, "1.234,56" → 123456)
//   - JPY: no minor units ("2,999" → 2999)
//   - USD/GBP/CAD/AUD: period is decimal separator ("29.99" → 2999)
//
// Returns 0 if the string is empty or unparseable.
func parsePriceCents(s string, currency string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}

	// Try regex for symbol-prefixed/suffixed prices first.
	var num string
	if m := priceRe.FindStringSubmatch(s); m != nil {
		num = m[1]
		if num == "" {
			num = m[2]
		}
	}

	// Fall back to plain numeric string (e.g. "8.99" from TVSS Amount fields).
	if num == "" {
		cleaned := strings.ReplaceAll(s, ",", "")
		if _, err := strconv.ParseFloat(cleaned, 64); err == nil {
			num = s
		}
	}

	if num == "" {
		return 0
	}

	// JPY has no minor units — return whole yen.
	if currency == "JPY" {
		num = strings.ReplaceAll(num, ",", "")
		num = strings.ReplaceAll(num, ".", "")
		yen, err := strconv.ParseInt(num, 10, 64)
		if err != nil {
			return 0
		}

		return yen
	}

	// EUR: period is thousands separator, comma is decimal.
	// "1.234,56" → "1234.56"
	if currency == "EUR" {
		num = strings.ReplaceAll(num, ".", "")
		num = strings.Replace(num, ",", ".", 1)
	} else {
		// Standard format: comma is thousands separator, period is decimal.
		num = strings.ReplaceAll(num, ",", "")
	}

	parts := strings.SplitN(num, ".", 2)

	major, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0
	}

	var minor int64
	if len(parts) == 2 {
		c := parts[1]
		if len(c) == 1 {
			c += "0"
		}
		minor, _ = strconv.ParseInt(c, 10, 64)
	}

	return major*100 + minor
}

// toMoney converts an Amazon price string to a shop.Money for the given
// currency.
func toMoney(s flexString, currency string) shop.Money {
	return shop.Money{
		Amount:   parsePriceCents(string(s), currency),
		Currency: currency,
	}
}
