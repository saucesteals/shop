package cli

import (
	"fmt"
	"strings"

	"github.com/saucesteals/shop"
)

func (o *textOutput) address(address *shop.Address) {
	if address == nil {
		return
	}
	o.field("Address ID", address.ID)
	o.field("Label", address.Label)
	o.field("Recipient", address.Name)
	o.field("Street", address.Line1)
	o.field("Street (continued)", address.Line2)
	o.field("City", address.City)
	o.field("State", address.State)
	o.field("Postal code", address.PostalCode)
	o.field("Country", address.Country)
	o.field("Phone", address.Phone)
	if address.IsDefault {
		o.field("Default address", "yes")
	}
}

func (o *textOutput) payment(method *shop.PaymentMethod) {
	if method == nil {
		return
	}
	o.field("Payment ID", method.ID)
	o.field("Payment", nonempty(method.Label, method.Type))
	o.field("Last four", method.Last4)
	if method.ExpMonth != 0 && method.ExpYear != 0 {
		o.field("Expires", fmt.Sprintf("%02d/%04d", method.ExpMonth, method.ExpYear))
	}
	if method.IsDefault {
		o.field("Default payment", "yes")
	}
}

func (o *textOutput) account(account *shop.AccountInfo) {
	o.field("Account", account.AccountName)
	o.field("Account ID", account.AccountID)
	o.field("Email", account.Email)
	o.field("Expires", account.ExpiresAt)
}

func (o *textOutput) capabilities(capabilities shop.Capabilities) {
	var names []string
	for _, entry := range []struct {
		name    string
		enabled bool
	}{
		{"search", capabilities.Search},
		{"reviews", capabilities.Reviews},
		{"offers", capabilities.Offers},
		{"variants", capabilities.Variants},
		{"cart", capabilities.Cart},
		{"checkout", capabilities.Checkout},
		{"addresses", capabilities.Addresses},
		{"payment methods", capabilities.PaymentMethods},
		{"shipping options", capabilities.ShippingOptions},
		{"coupons", capabilities.Coupons},
	} {
		if entry.enabled {
			names = append(names, entry.name)
		}
	}
	o.field("Supported", nonempty(strings.Join(names, ", "), "none"))
}
