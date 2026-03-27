package amazon

import (
	"encoding/json"
	"time"

	"github.com/saucesteals/shop"
	"github.com/saucesteals/shop/internal/config"
)

const (
	nsCheckouts = "checkouts"
	nsOrders    = "orders"
)

// checkoutState is the persisted checkout session. Contains everything
// PlaceOrder needs to sign the purchase without any additional network calls.
type checkoutState struct {
	PurchaseID string               `json:"purchaseId"`
	Items      []checkoutStateItem  `json:"items"`
	CreatedAt  string               `json:"createdAt"`
}

// checkoutStateItem is the minimal data needed for the sign request.
// Fields map directly to the TVSS CheckoutPurchaseSignItem from the APK:
//   id (cartId), asin, offerId, quantity (always 0).
type checkoutStateItem struct {
	ID      string `json:"id"`
	ASIN    string `json:"asin"`
	OfferID string `json:"offerId"`
}

// orderState is a persisted order record.
type orderState struct {
	OrderID    string      `json:"orderId"`
	PurchaseID string      `json:"purchaseId"`
	Total      *shop.Money `json:"total,omitempty"`
	PlacedAt   string      `json:"placedAt"`
}

// saveCheckout persists a checkout session to disk.
func (s *Store) saveCheckout(sess *purchaseSession) error {
	state := checkoutState{
		PurchaseID: sess.PurchaseID,
		CreatedAt:  time.Now().UTC().Format(time.RFC3339),
	}

	for _, item := range sess.CartItems {
		state.Items = append(state.Items, checkoutStateItem{
			ID:      item.ItemID,
			ASIN:    item.ASIN,
			OfferID: item.OfferID,
		})
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return shop.Errorf(shop.ErrInternal, "marshal checkout state: %v", err)
	}

	return config.SaveState(s.configDir, s.handle, nsCheckouts, sess.PurchaseID, data)
}

// loadCheckout reads a persisted checkout session.
func (s *Store) loadCheckout(purchaseID string) (*checkoutState, error) {
	raw, err := config.LoadState(s.configDir, s.handle, nsCheckouts, purchaseID)
	if err != nil {
		return nil, shop.Errorf(shop.ErrConfigError, "load checkout: %v", err)
	}
	if raw == nil {
		return nil, shop.Errorf(shop.ErrNotFound, "no checkout session for %s", purchaseID)
	}

	var state checkoutState
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, shop.Errorf(shop.ErrConfigError, "parse checkout state: %v", err)
	}

	return &state, nil
}

// deleteCheckout removes a persisted checkout session.
func (s *Store) deleteCheckout(purchaseID string) error {
	return config.DeleteState(s.configDir, s.handle, nsCheckouts, purchaseID)
}

// saveOrder persists an order record to disk.
func (s *Store) saveOrder(order *shop.Order, purchaseID string) error {
	state := orderState{
		OrderID:    order.OrderID,
		PurchaseID: purchaseID,
		Total:      &order.Total,
		PlacedAt:   order.PlacedAt,
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return shop.Errorf(shop.ErrInternal, "marshal order state: %v", err)
	}

	return config.SaveState(s.configDir, s.handle, nsOrders, order.OrderID, data)
}
