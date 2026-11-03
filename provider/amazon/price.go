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
// (cents for USD/GBP/EUR/CAD/AUD, whole yen for JPY). The decimal separator
// is detected from the string itself, so both "€2,243.99" and "2.243,99 €"
// give 224399. JPY has no minor units ("2,999" → 2999).
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

	// The decimal separator is whichever of '.' or ',' comes last with one
	// or two digits after it; every other separator groups thousands. This
	// does not depend on the currency: under the en-US Accept-Language this
	// client sends, amazon.fr and amazon.de format EUR as "€2,243.99".
	var frac string
	if i := strings.LastIndexAny(num, ".,"); i >= 0 && len(num)-i-1 >= 1 && len(num)-i-1 <= 2 {
		frac = num[i+1:]
		num = num[:i]
	}
	num = strings.NewReplacer(".", "", ",", "").Replace(num)

	major, err := strconv.ParseInt(num, 10, 64)
	if err != nil {
		return 0
	}

	var minor int64
	if frac != "" {
		if len(frac) == 1 {
			frac += "0"
		}
		minor, _ = strconv.ParseInt(frac, 10, 64)
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
