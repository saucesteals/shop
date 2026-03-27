package amazon

import (
	"context"
	"strings"
	"time"

	"github.com/saucesteals/shop"
)

// parseAddressLine parses "City, State, PostalCode" into the address fields.
// Handles formats like "Temecula, CA, 92591" and "Temecula, CA 92591".
func parseAddressLine(line string, addr *shop.Address) {
	parts := strings.Split(line, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}

	switch len(parts) {
	case 3:
		addr.City = parts[0]
		addr.State = parts[1]
		addr.PostalCode = parts[2]
	case 2:
		addr.City = parts[0]
		// "CA 92591" or just "CA"
		sp := strings.SplitN(parts[1], " ", 2)
		addr.State = sp[0]
		if len(sp) > 1 {
			addr.PostalCode = sp[1]
		}
	default:
		addr.City = line
	}
}

// Checkout initiates a purchase session, returning a checkout preview. This
// calls the TVSS purchaseInitiate endpoint which sets up the checkout on
// Amazon's side and returns destinations, payment methods, line items, and
// totals.
//
// TVSS endpoint: POST /marketplaces/{marketplace}/checkout/purchase/initiate
// Content-Type: application/vnd.com.amazon.tvss.api+json; type="checkout.purchase.initiate-request/v1"
func (s *Store) Checkout(ctx context.Context, opts *shop.CheckoutOpts) (*shop.CheckoutResult, error) {
	resp, err := s.initiatePurchaseSession(ctx)
	if err != nil {
		return nil, err
	}

	api, err := s.tvssAPI()
	if err != nil {
		return nil, err
	}

	// Apply user overrides to the purchase session before building the result.
	if opts != nil {
		if err := s.applyCheckoutOpts(ctx, api, resp, opts); err != nil {
			return nil, err
		}
	}

	cr := mapCheckoutResult(resp, s.handle, s.currency)

	if err := s.saveCheckout(resp); err != nil {
		return nil, err
	}

	return cr, nil
}

// applyCheckoutOpts sets payment method and shipping on the purchase session.
// For shipping: uses the explicit option if provided, otherwise auto-selects
// the fastest free option. Skips API calls when the desired state already matches.
func (s *Store) applyCheckoutOpts(ctx context.Context, api *tvssClient, sess *purchaseSession, opts *shop.CheckoutOpts) error {
	// Payment method override.
	if opts.PaymentMethodID != "" {
		current := ""
		if sess.PaymentMethods != nil && sess.PaymentMethods.CreditOrDebitCard != nil {
			current = sess.PaymentMethods.CreditOrDebitCard.ID
		}
		if opts.PaymentMethodID != current {
			if err := s.setPaymentMethod(ctx, api, sess.PurchaseID, opts.PaymentMethodID); err != nil {
				return err
			}
		}
	}

	// Shipping selection.
	target, current := s.pickShipping(sess, opts.ShippingOption)
	if target != "" && target != current {
		dg := sess.DeliveryGroups.DeliveryGroups[0]
		if err := s.setDeliveryOption(ctx, api, sess.PurchaseID, dg.ID, target); err != nil {
			return err
		}
		// Update the session so mapCheckoutResult sees the new selection.
		for i := range dg.Options {
			dg.Options[i].Selected = dg.Options[i].ID == target
		}
		sess.DeliveryGroups.DeliveryGroups[0] = dg
	}

	return nil
}

// pickShipping returns the desired shipping option ID and the currently
// selected one. If explicit is set, that's the target. Otherwise picks the
// first free option (TVSS lists fastest first).
func (s *Store) pickShipping(sess *purchaseSession, explicit string) (target, current string) {
	if sess.DeliveryGroups == nil || len(sess.DeliveryGroups.DeliveryGroups) == 0 {
		return
	}

	for _, opt := range sess.DeliveryGroups.DeliveryGroups[0].Options {
		if opt.Selected {
			current = opt.ID
		}
		if explicit != "" {
			if opt.ID == explicit {
				target = opt.ID
			}
		} else if target == "" && isFreeDelivery(opt.DisplayPrice) {
			target = opt.ID
		}
	}
	return
}

// setPaymentMethod selects a payment method on the purchase session.
func (s *Store) setPaymentMethod(ctx context.Context, api *tvssClient, purchaseID, paymentMethodID string) error {
	u := api.tvssPath([]string{"checkout", "purchases", purchaseID, "payment-methods"}, nil)
	req := map[string]any{
		"paymentMethodId":    paymentMethodID,
		"useGiftCardBalance": false,
		"usePromoBalance":    false,
	}
	return api.doPut(ctx, u, "checkout.purchase.payment-methods/v1", req, nil)
}

// isFreeDelivery checks if a delivery option's display price indicates free
// shipping. Handles "$0.00", "FREE", empty string, etc.
func isFreeDelivery(displayPrice string) bool {
	p := strings.TrimSpace(displayPrice)
	if p == "" || strings.EqualFold(p, "free") {
		return true
	}
	return toMoney(flexString(p), "USD").Amount == 0
}

// setDeliveryOption selects a shipping speed on a delivery group.
func (s *Store) setDeliveryOption(ctx context.Context, api *tvssClient, purchaseID, deliveryGroupID, optionID string) error {
	u := api.tvssPath([]string{"checkout", "purchases", purchaseID, "delivery-groups", deliveryGroupID, "options"}, nil)
	req := struct {
		DeliveryOptionID string `json:"deliveryOptionId"`
	}{DeliveryOptionID: optionID}
	return api.doPut(ctx, u, "checkout.purchase.delivery-groups.options.set-request/v1", req, nil)
}

// PlaceOrder commits the purchase. The checkoutID must be a purchaseId from a
// prior Checkout() call. This calls the TVSS purchaseSign endpoint.
//
// WARNING: This places a real order! Do not call in tests.
//
// TVSS endpoint: POST /marketplaces/{marketplace}/checkout/purchases/{purchaseId}/sign
// Content-Type: application/vnd.com.amazon.tvss.api+json; type="checkout.purchase.sign-request/v1"
func (s *Store) PlaceOrder(ctx context.Context, checkoutID string) (*shop.Order, error) {
	if checkoutID == "" {
		return nil, shop.Errorf(shop.ErrInvalidInput, "checkoutID (purchaseId) is required")
	}

	// Load persisted checkout session — contains the cart items needed for sign.
	checkout, err := s.loadCheckout(checkoutID)
	if err != nil {
		return nil, err
	}

	api, err := s.tvssAPI()
	if err != nil {
		return nil, err
	}

	// Step 1: Fetch the order ID BEFORE signing.
	// The APK calls checkoutOrdersGet before purchaseSign — the orders
	// endpoint only works while the purchase session is unsigned.
	ordersURL := api.tvssPath([]string{"checkout", "purchases", checkoutID, "orders"}, nil)
	var ordersResp tvssCheckoutOrdersResponse
	if err := api.doGet(ctx, ordersURL, &ordersResp); err != nil {
		return nil, shop.Errorf(shop.ErrStoreError, "fetch order details: %v", err)
	}
	if len(ordersResp.Orders) == 0 {
		return nil, shop.Errorf(shop.ErrStoreError, "no orders returned for purchase %s", checkoutID)
	}

	// Step 2: Sign the purchase — this commits the order.
	signItems := make([]purchaseSignItem, len(checkout.Items))
	for i, item := range checkout.Items {
		signItems[i] = purchaseSignItem{
			ID:      item.ID,
			ASIN:    item.ASIN,
			OfferID: item.OfferID,
		}
	}

	signURL := api.tvssPath([]string{"checkout", "purchases", checkoutID, "sign"}, nil)
	var signResp tvssPurchaseResponse
	if err := api.doPost(ctx, signURL, "checkout.purchase.sign-request/v1", purchaseSignRequest{CartItems: signItems}, &signResp); err != nil {
		return nil, err
	}

	// Verify the purchase was actually placed.
	for _, state := range signResp.PurchaseState {
		if state != "SIGNED" {
			return nil, shop.Errorf(shop.ErrStoreError, "order not placed: %s", state)
		}
	}

	for _, r := range signResp.PurchaseRestrictions {
		if r.Reason != "" {
			return nil, shop.Errorf(shop.ErrStoreError, "order not placed: %s", r.Reason)
		}
	}

	// Sign succeeded — clean up checkout, persist order.
	_ = s.deleteCheckout(checkoutID)

	order := &shop.Order{
		OrderID:  ordersResp.Orders[0].ID,
		Status:   "placed",
		PlacedAt: time.Now().UTC().Format(time.RFC3339),
	}

	if signResp.PurchaseTotals != nil && signResp.PurchaseTotals.PurchaseTotal != nil {
		order.Total = currencyAmount(signResp.PurchaseTotals.PurchaseTotal.Amount, s.currency)
	}

	_ = s.saveOrder(order, checkoutID)

	return order, nil
}

// Addresses returns the saved shipping addresses by initiating a checkout
// session and fetching available destinations.
//
// TVSS endpoint: GET /marketplaces/{marketplace}/checkout/purchases/{id}/destinations/available
func (s *Store) Addresses(ctx context.Context) ([]shop.Address, error) {
	resp, err := s.initiatePurchaseSession(ctx)
	if err != nil {
		return nil, err
	}

	if resp.Destinations == nil {
		return nil, nil
	}

	// Build address list from the initiate response destinations.
	// Use destinationList for full address details when available.
	detailByIndex := make(map[int]*tvssDetailAddress)
	for i, d := range resp.Destinations.DestinationList {
		if d.Address != nil {
			detailByIndex[i] = d.Address
		}
	}

	var addresses []shop.Address
	for i, d := range resp.Destinations.Destinations {
		id := d.ID
		if id == "" {
			id = d.AddressID
		}

		addr := shop.Address{
			ID:        id,
			Name:      d.Name,
			City:      d.City,
			Label:     d.DisplayString,
			IsDefault: i == 0,
		}

		// Enrich with full address from destinationList if available.
		if detail, ok := detailByIndex[i]; ok && detail.Display != nil {
			addr.Label = detail.Display.SingleLine
			lines := detail.Display.MultiLine
			if len(lines) > 0 {
				addr.Line1 = lines[0]
			}
			if len(lines) > 1 {
				parseAddressLine(lines[1], &addr)
			}
			if len(lines) > 2 {
				addr.Country = lines[2]
			}
			if len(lines) > 3 {
				addr.Phone = strings.TrimPrefix(lines[3], "Phone number: ")
			}
		}

		addresses = append(addresses, addr)
	}

	return addresses, nil
}

// PaymentMethods returns saved payment methods by initiating a checkout
// session and fetching available payment methods.
//
// TVSS endpoint: GET /marketplaces/{marketplace}/checkout/purchases/{id}/payment-methods/available
func (s *Store) PaymentMethods(ctx context.Context) ([]shop.PaymentMethod, error) {
	api, err := s.tvssAPI()
	if err != nil {
		return nil, err
	}

	resp, err := s.initiatePurchaseSession(ctx)
	if err != nil {
		return nil, err
	}

	pmURL := api.tvssPath([]string{"checkout", "purchases", resp.PurchaseID, "payment-methods", "available"}, nil)

	var pmResp tvssAvailablePaymentMethods
	if err := api.doGet(ctx, pmURL, &pmResp); err != nil {
		return nil, err
	}

	var methods []shop.PaymentMethod

	for i, card := range pmResp.CreditOrDebitCards {
		pm := shop.PaymentMethod{
			ID:        card.ID,
			Type:      "credit_card",
			Label:     string(card.Issuer) + " ending in " + card.EndingIn,
			Last4:     card.EndingIn,
			IsDefault: i == 0,
		}
		if card.Expiry != nil {
			pm.ExpMonth = card.Expiry.Month
			pm.ExpYear = card.Expiry.Year
		}
		methods = append(methods, pm)
	}

	for _, ba := range pmResp.BankAccounts {
		methods = append(methods, shop.PaymentMethod{
			Type:  "bank_account",
			Label: "Bank ending in " + ba.EndingIn,
			Last4: ba.EndingIn,
		})
	}

	if pmResp.GiftCardAndPromo != nil && pmResp.GiftCardAndPromo.Balance != nil {
		methods = append(methods, shop.PaymentMethod{
			Type:  "gift_card",
			Label: "Gift Card Balance: " + pmResp.GiftCardAndPromo.Balance.DisplayString,
		})
	}

	return methods, nil
}

// purchaseSession bundles the TVSS purchase response with the cart items
// that were used to initiate it.
type purchaseSession struct {
	tvssPurchaseResponse
	CartItems []tvssCartItem
}

// initiatePurchaseSession fetches the current cart and creates a TVSS
// purchase session. Shared by Checkout, Addresses, and PaymentMethods.
func (s *Store) initiatePurchaseSession(ctx context.Context) (*purchaseSession, error) {
	api, err := s.tvssAPI()
	if err != nil {
		return nil, err
	}

	cartURL := api.tvssPath([]string{"cart", "items"}, nil)

	var cart tvssCartResponse
	if err := api.doGet(ctx, cartURL, &cart); err != nil {
		return nil, err
	}

	// Filter out saved-for-later items — only active cart items can be purchased.
	var items []tvssPurchaseInitiateItem
	for _, item := range cart.Items {
		if item.SavedItem {
			continue
		}

		items = append(items, tvssPurchaseInitiateItem{
			ASIN:     item.ASIN,
			OfferID:  item.OfferID,
			Quantity: item.Quantity,
		})
	}

	if len(items) == 0 {
		return nil, shop.Errorf(shop.ErrCartEmpty, "cart is empty")
	}

	req := tvssPurchaseInitiateRequest{Items: items}
	initURL := api.tvssPath([]string{"checkout", "purchase", "initiate"}, nil)

	var resp tvssPurchaseResponse
	if err := api.doPost(ctx, initURL, "checkout.purchase.initiate-request/v1", req, &resp); err != nil {
		return nil, err
	}

	if resp.PurchaseID == "" {
		return nil, shop.Errorf(shop.ErrStoreError, "no purchaseId returned from checkout")
	}

	// Collect active cart items (excluding saved-for-later).
	var activeItems []tvssCartItem
	for _, item := range cart.Items {
		if !item.SavedItem {
			activeItems = append(activeItems, item)
		}
	}

	return &purchaseSession{
		tvssPurchaseResponse: resp,
		CartItems:            activeItems,
	}, nil
}

// mapCartItemsToEntries converts TVSS cart items to shop.CartEntry slice.
func mapCartItemsToEntries(items []tvssCartItem, domain, currency string) []shop.CartEntry {
	entries := make([]shop.CartEntry, 0, len(items))
	for _, item := range items {
		entry := shop.CartEntry{
			Quantity: item.Quantity,
			Product: shop.Product{
				ID:    item.ASIN,
				Title: item.Title,
				Brand: item.ByLine,
				URL:   productURL(domain, item.ASIN),
				Availability: shop.Availability{
					Status: shop.AvailabilityInStock,
				},
			},
		}
		if item.Price != "" {
			p := toMoney(item.Price, currency)
			entry.Product.Price = &p
		}
		if item.ImageURL != "" {
			entry.Product.Images = []shop.Image{{URL: item.ImageURL}}
		}
		entries = append(entries, entry)
	}
	return entries
}

// currencyAmount converts a tvssCurrency to shop.Money. Uses the Amount field
// (numeric string like "8.99") and the embedded CurrencyCode when available,
// falling back to the store's currency.
func currencyAmount(c *tvssCurrency, fallbackCurrency string) shop.Money {
	if c == nil {
		return shop.Money{Currency: fallbackCurrency}
	}

	cur := c.CurrencyCode
	if cur == "" {
		cur = fallbackCurrency
	}

	return toMoney(flexString(c.Amount), cur)
}

// mapCheckoutResult converts a TVSS purchase response to shop.CheckoutResult.
func mapCheckoutResult(sess *purchaseSession, domain, currency string) *shop.CheckoutResult {
	resp := &sess.tvssPurchaseResponse

	cr := &shop.CheckoutResult{
		CheckoutID: resp.PurchaseID,
		Discount:   shop.Money{Currency: currency},
	}

	// Items come from the cart — the initiate response doesn't include them.
	cr.Items = mapCartItemsToEntries(sess.CartItems, domain, currency)

	// Map totals — match on Type codes, not display strings.
	if resp.PurchaseTotals != nil {
		if pt := resp.PurchaseTotals.PurchaseTotal; pt != nil {
			cr.Total = currencyAmount(pt.Amount, currency)
		}

		for _, sub := range resp.PurchaseTotals.Subtotals {
			if sub.Amount == nil {
				continue
			}

			money := currencyAmount(sub.Amount, currency)

			switch sub.Type {
			case "ITEMS_TAX_EXCLUSIVE", "ITEMS_TAX_INCLUSIVE":
				cr.Subtotal = money
			case "SHIPPING_TAX_EXCLUSIVE", "SHIPPING_TAX_INCLUSIVE":
				cr.Shipping = money
			case "PROMO_EXCLUSIVE_TAX_TOTAL_ESTIMATE", "TAX_TOTAL", "TAX_ESTIMATE":
				cr.Tax = money
			case "DISCOUNT", "PROMO_SAVINGS":
				cr.Discount = money
			}
		}
	}

	// Map shipping address from destinations + destinationList for full details.
	if resp.Destinations != nil && len(resp.Destinations.Destinations) > 0 {
		d := resp.Destinations.Destinations[0]

		id := d.ID
		if id == "" {
			id = d.AddressID
		}

		cr.ShippingAddress = &shop.Address{
			ID:        id,
			Name:      d.Name,
			City:      d.City,
			Label:     d.DisplayString,
			IsDefault: true,
		}

		// Enrich from destinationList if available.
		if len(resp.Destinations.DestinationList) > 0 {
			if detail := resp.Destinations.DestinationList[0].Address; detail != nil && detail.Display != nil {
				cr.ShippingAddress.Label = detail.Display.SingleLine
				lines := detail.Display.MultiLine
				if len(lines) > 0 {
					cr.ShippingAddress.Line1 = lines[0]
				}
				if len(lines) > 1 {
					parseAddressLine(lines[1], cr.ShippingAddress)
				}
				if len(lines) > 2 {
					cr.ShippingAddress.Country = lines[2]
				}
				if len(lines) > 3 {
					cr.ShippingAddress.Phone = strings.TrimPrefix(lines[3], "Phone number: ")
				}
			}
		}
	}

	// Map payment method.
	if resp.PaymentMethods != nil && resp.PaymentMethods.CreditOrDebitCard != nil {
		card := resp.PaymentMethods.CreditOrDebitCard
		cr.PaymentMethod = &shop.PaymentMethod{
			ID:    card.ID,
			Type:  "credit_card",
			Label: string(card.Issuer) + " ending in " + card.EndingIn,
			Last4: card.EndingIn,
		}
		if card.Expiry != nil {
			cr.PaymentMethod.ExpMonth = card.Expiry.Month
			cr.PaymentMethod.ExpYear = card.Expiry.Year
		}
	}

	// Map delivery options to shipping options.
	if resp.DeliveryGroups != nil {
		for _, dg := range resp.DeliveryGroups.DeliveryGroups {
			for _, opt := range dg.Options {
				so := shop.ShippingOption{
					ID:        opt.ID,
					Label:     opt.DisplayString,
					Price:     toMoney(flexString(opt.DisplayPrice), currency),
					IsDefault: opt.Selected,
				}
				cr.ShippingOptions = append(cr.ShippingOptions, so)
				if opt.Selected {
					selected := so
					cr.SelectedShipping = &selected
				}
			}
		}
	}

	// Map restrictions to warnings.
	for _, r := range resp.PurchaseRestrictions {
		msg := r.Message
		if msg == "" {
			msg = r.Reason
		}
		if msg != "" {
			cr.Warnings = append(cr.Warnings, msg)
		}
	}

	return cr
}
