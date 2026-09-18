package tracking

import "strings"

// Carrier identifies a declared delivery network, independently of its data provider.
type Carrier string

const (
	// Unknown means the number cannot be classified confidently.
	Unknown Carrier = ""
	// UPS identifies standard UPS package numbers.
	UPS Carrier = "ups"
	// USPS identifies supported domestic and international postal numbers.
	USPS Carrier = "usps"
)

// DetectCarrier recognizes unambiguous supported formats, not shipment validity.
// Ambiguous formats deliberately use the general tracking source.
func DetectCarrier(number string) Carrier {
	number = strings.ToUpper(strings.TrimSpace(number))
	if len(number) == 18 && strings.HasPrefix(number, "1Z") && asciiIdentifier(number[2:], true) {
		return UPS
	}
	if len(number) == 22 && strings.HasPrefix(number, "9") && asciiIdentifier(number, false) {
		return USPS
	}
	if len(number) == 13 && strings.HasSuffix(number, "US") && number[0] >= 'A' && number[0] <= 'Z' && number[1] >= 'A' && number[1] <= 'Z' && asciiIdentifier(number[2:11], false) {
		return USPS
	}

	return Unknown
}

func asciiIdentifier(value string, letters bool) bool {
	for _, ch := range value {
		if ch >= '0' && ch <= '9' {
			continue
		}
		if letters && ch >= 'A' && ch <= 'Z' {
			continue
		}

		return false
	}

	return true
}
