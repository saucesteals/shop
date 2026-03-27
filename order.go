package shop

// CartContents is a snapshot of the current cart state.
type CartContents struct {
	Items    []CartEntry `json:"items"`
	Subtotal Money       `json:"subtotal"`
}

// CartEntry is a single line item in the cart.
type CartEntry struct {
	Product  Product `json:"product"`
	Quantity int     `json:"quantity"`
}

// CheckoutOpts controls checkout preview behavior.
type CheckoutOpts struct {
	AddressID       string `json:"addressId,omitempty"`
	PaymentMethodID string `json:"paymentMethodId,omitempty"`
	ShippingOption  string `json:"shippingOption,omitempty"`
	CouponCode      string `json:"couponCode,omitempty"`
}

// CheckoutResult is the order preview returned by Store.Checkout().
type CheckoutResult struct {
	CheckoutID        string           `json:"checkoutId"`
	Items             []CartEntry      `json:"items"`
	Subtotal          Money            `json:"subtotal"`
	Shipping          Money            `json:"shipping"`
	Tax               Money            `json:"tax"`
	Discount          Money            `json:"discount"`
	Total             Money            `json:"total"`
	ShippingAddress   *Address         `json:"shippingAddress,omitempty"`
	PaymentMethod     *PaymentMethod   `json:"paymentMethod,omitempty"`
	ShippingOptions   []ShippingOption `json:"shippingOptions,omitempty"`
	SelectedShipping  *ShippingOption  `json:"selectedShipping,omitempty"`
	EstimatedDelivery string           `json:"estimatedDelivery,omitempty"`
	Warnings          []string         `json:"warnings,omitempty"`
	Attributes        map[string]any   `json:"attributes,omitempty"`
}

// Order is the result of a successfully placed order.
type Order struct {
	OrderID           string         `json:"orderId"`
	Status            string         `json:"status"`
	Items             []CartEntry    `json:"items"`
	Total             Money          `json:"total"`
	ShippingAddress   *Address       `json:"shippingAddress,omitempty"`
	PaymentMethod     *PaymentMethod `json:"paymentMethod,omitempty"`
	EstimatedDelivery string         `json:"estimatedDelivery,omitempty"`
	PlacedAt          string         `json:"placedAt"`
	Attributes        map[string]any `json:"attributes,omitempty"`
}
