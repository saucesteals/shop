package cli

import (
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/saucesteals/shop"
	"github.com/saucesteals/shop/internal/config"
	"github.com/saucesteals/shop/tracking"
)

type removalResult struct {
	Removed bool `json:"removed"`
}

// textOutput builds plain, line-oriented output. Rendering is kept separate from
// I/O so a formatter failure cannot leave half of a result on stdout.
type textOutput struct {
	strings.Builder
}

func writeText(w io.Writer, value any) error {
	var out textOutput
	switch v := value.(type) {
	case *shop.SearchResult:
		out.search(v)
	case *shop.Product:
		out.product(v)
	case *shop.ReviewsResult:
		out.reviews(v)
	case *shop.OffersResult:
		out.offers(v)
	case *shop.VariantsResult:
		out.variants(v)
	case *shop.CartContents:
		out.line("Cart")
		if len(v.Items) == 0 {
			out.line("No items")
		}
		out.cartItems(v.Items)
		out.field("Subtotal", money(&v.Subtotal))
	case *shop.CheckoutResult:
		out.checkout(v)
	case *shop.Order:
		out.order(v)
	case *tracking.Snapshot:
		out.line(v.TrackingNumber)
		out.snapshot(v, true)
	case *tracking.Shipment:
		out.shipment(*v)
	case []tracking.Shipment:
		out.line(fmt.Sprintf("Saved shipments (%d)", len(v)))
		for _, shipment := range v {
			out.section()
			out.shipment(shipment)
		}
	case refreshSummary:
		out.refresh(v)
	case removalResult:
		out.field("Removed", yesNo(v.Removed))
	case []config.RegistryEntry:
		out.line(fmt.Sprintf("Stores (%d)", len(v)))
		for _, entry := range v {
			out.section()
			out.line(entry.Name)
			out.field("Store", entry.Alias)
			out.field("Domain", entry.Domain)
			out.field("Provider", entry.Provider)
			out.field("Country", entry.Country)
			out.field("Currency", entry.Currency)
		}
	case storeInfoOutput:
		out.line(v.Name)
		out.field("Domain", v.Domain)
		out.field("Provider", v.Provider)
		out.field("Country", v.Country)
		out.field("Currency", v.Currency)
		out.capabilities(v.Capabilities)
	case shop.Capabilities:
		out.capabilities(v)
	case *shop.LoginResult:
		out.field("Authenticated", yesNo(v.Authenticated))
		if v.Account != nil {
			out.account(v.Account)
		}
		if v.Challenge != nil {
			out.field("Continue at", v.Challenge.URL)
			out.field("Code", v.Challenge.Code)
			out.field("Expires", v.Challenge.ExpiresAt)
			out.field("Next step", v.Challenge.Message)
		}
	case *shop.AccountInfo:
		out.field("Authenticated", yesNo(v.Authenticated))
		out.account(v)
	case []shop.Address:
		out.line(fmt.Sprintf("Addresses (%d)", len(v)))
		for _, address := range v {
			out.section()
			out.address(&address)
		}
	case []shop.PaymentMethod:
		out.line(fmt.Sprintf("Payment methods (%d)", len(v)))
		for _, method := range v {
			out.section()
			out.payment(&method)
		}
	case map[string]string:
		if key, ok := v["key"]; ok && len(v) == 2 {
			out.field(key, nonempty(v["value"], "(not set)"))
		} else {
			out.fields(v)
		}
	case map[string]bool:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		for _, key := range keys {
			out.field(key, yesNo(v[key]))
		}
	default:
		return shop.Errorf(shop.ErrInternal, "text output is not implemented for %T", value)
	}

	_, err := io.WriteString(w, out.String())

	return err
}

// Never interpret upstream terminal controls. Keep values on a single line;
// the formatter, not the carrier or merchant, owns indentation and line breaks.
func cleanText(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return ' '
		}
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf) {
			return -1
		}

		return r
	}, value)
}

func (o *textOutput) line(value string) {
	o.WriteString(cleanText(value))
	o.WriteByte('\n')
}

func (o *textOutput) section() {
	o.WriteByte('\n')
}

func (o *textOutput) field(label, value string) {
	if value != "" {
		o.line("  " + label + ": " + value)
	}
}

func (o *textOutput) fields(values map[string]string) {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		o.field(key, nonempty(values[key], "(not set)"))
	}
}

func nonempty(value, fallback string) string {
	if value == "" {
		return fallback
	}

	return value
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}

	return "no"
}

// Money stays integer-based, including negative values and math.MinInt64.
// Unknown currencies retain explicit minor units instead of guessing a scale.
func money(value *shop.Money) string {
	if value == nil || value.Currency == "" {
		return "Not provided"
	}
	var decimals int
	switch value.Currency {
	case "USD", "GBP", "EUR", "CAD", "AUD", "CHF", "CNY", "HKD", "NZD", "SGD", "INR", "SEK", "NOK", "DKK", "PLN", "MXN", "BRL", "AED", "SAR", "TRY", "ZAR":
		decimals = 2
	case "JPY", "KRW", "VND", "CLP", "ISK", "XAF", "XOF", "XPF":
		decimals = 0
	case "BHD", "KWD", "OMR", "JOD", "TND", "LYD", "IQD":
		decimals = 3
	default:
		return fmt.Sprintf("%s %d minor units", nonempty(value.Currency, "Unknown currency"), value.Amount)
	}
	digits := strconv.FormatInt(value.Amount, 10)
	sign := ""
	if strings.HasPrefix(digits, "-") {
		sign, digits = "-", digits[1:]
	}
	if decimals > 0 {
		if len(digits) <= decimals {
			digits = strings.Repeat("0", decimals+1-len(digits)) + digits
		}
		cut := len(digits) - decimals
		digits = digits[:cut] + "." + digits[cut:]
	}

	return value.Currency + " " + sign + digits
}
