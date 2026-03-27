package amazon

import (
	"context"

	"github.com/saucesteals/shop"
)

// cartImpl implements shop.Cart for Amazon using the TVSS cart API.
type cartImpl struct {
	store *Store
}

// Add adds an item to the Amazon cart.
//
// TVSS endpoint: PUT /marketplaces/{marketplace}/cart/items
// Content-Type: application/vnd.com.amazon.tvss.api+json; type="cart.add.request/v1"
func (c *cartImpl) Add(ctx context.Context, id string, quantity int) (*shop.CartContents, error) {
	if err := validateASIN(id); err != nil {
		return nil, err
	}

	api, err := c.store.tvssAPI()
	if err != nil {
		return nil, err
	}

	if quantity < 1 {
		quantity = 1
	}

	// Fetch the offerId from the product detail. TVSS requires it.
	prodURL := api.tvssPath([]string{"products", id}, nil)

	var tp tvssProduct
	if err := api.doGet(ctx, prodURL, &tp); err != nil {
		return nil, shop.Errorf(shop.ErrNotFound, "product %s: %v", id, err)
	}

	if tp.OfferID == "" {
		// Product exists in the catalog but has no current offer. This
		// commonly happens when the product is out of stock in TVSS or
		// the device type doesn't expose pricing.
		return nil, shop.Errorf(shop.ErrOutOfStock, "product %s has no available offer", id)
	}

	req := tvssAddToCartRequest{
		Items: []tvssAddToCartItem{
			{
				ASIN:     id,
				OfferID:  tp.OfferID,
				Quantity: quantity,
			},
		},
	}

	u := api.tvssPath([]string{"cart", "items"}, nil)

	var resp tvssAddToCartResponse
	if err := api.doPut(ctx, u, "cart.add.request/v1", req, &resp); err != nil {
		return nil, err
	}

	// After adding, fetch the full cart to return a consistent snapshot.
	return c.View(ctx)
}

// Remove removes an item from the Amazon cart by setting its quantity to 0.
//
// TVSS endpoint: PATCH /marketplaces/{marketplace}/cart/items
// Content-Type: application/vnd.com.amazon.tvss.api+json; type="cart.modify.request/v1"
func (c *cartImpl) Remove(ctx context.Context, id string) (*shop.CartContents, error) {
	api, err := c.store.tvssAPI()
	if err != nil {
		return nil, err
	}

	// First, get the cart to find the itemId for this ASIN.
	viewURL := api.tvssPath([]string{"cart", "items"}, nil)

	var cart tvssCartResponse
	if err := api.doGet(ctx, viewURL, &cart); err != nil {
		return nil, err
	}

	// Find the item by ASIN (skip saved-for-later items).
	var itemID string
	for _, item := range cart.Items {
		if item.SavedItem {
			continue
		}

		if item.ASIN == id || item.ItemID == id {
			itemID = item.ItemID
			break
		}
	}
	if itemID == "" {
		return nil, shop.Errorf(shop.ErrNotFound, "item %q not in cart", id)
	}

	req := tvssModifyCartRequest{
		Items: []tvssModifyCartItem{
			{
				ID:       itemID,
				Quantity: 0,
			},
		},
	}

	u := api.tvssPath([]string{"cart", "items"}, nil)

	var resp tvssCartResponse
	if err := api.doPatch(ctx, u, "cart.modify.request/v1", req, &resp); err != nil {
		return nil, err
	}

	return mapCartResponse(&resp, c.store.handle, c.store.currency), nil
}

// View returns the current cart snapshot.
//
// TVSS endpoint: GET /marketplaces/{marketplace}/cart/items
func (c *cartImpl) View(ctx context.Context) (*shop.CartContents, error) {
	api, err := c.store.tvssAPI()
	if err != nil {
		return nil, err
	}

	u := api.tvssPath([]string{"cart", "items"}, nil)

	var resp tvssCartResponse
	if err := api.doGet(ctx, u, &resp); err != nil {
		return nil, err
	}

	return mapCartResponse(&resp, c.store.handle, c.store.currency), nil
}

// Clear empties the cart by setting quantity=0 on all items.
func (c *cartImpl) Clear(ctx context.Context) (*shop.CartContents, error) {
	api, err := c.store.tvssAPI()
	if err != nil {
		return nil, err
	}

	// Get current cart.
	viewURL := api.tvssPath([]string{"cart", "items"}, nil)

	var cart tvssCartResponse
	if err := api.doGet(ctx, viewURL, &cart); err != nil {
		return nil, err
	}

	if len(cart.Items) == 0 {
		return mapCartResponse(&cart, c.store.handle, c.store.currency), nil
	}

	// Build a patch request to zero out active cart items (skip saved-for-later).
	var items []tvssModifyCartItem
	for _, item := range cart.Items {
		if item.SavedItem {
			continue
		}

		items = append(items, tvssModifyCartItem{
			ID:       item.ItemID,
			Quantity: 0,
		})
	}

	req := tvssModifyCartRequest{Items: items}
	u := api.tvssPath([]string{"cart", "items"}, nil)

	var resp tvssCartResponse
	if err := api.doPatch(ctx, u, "cart.modify.request/v1", req, &resp); err != nil {
		return nil, err
	}

	return mapCartResponse(&resp, c.store.handle, c.store.currency), nil
}

// mapCartResponse converts a TVSS cart response to shop.CartContents.
// domain and currency are used for URL generation and price parsing.
func mapCartResponse(resp *tvssCartResponse, domain, currency string) *shop.CartContents {
	cc := &shop.CartContents{
		Subtotal: toMoney(resp.Subtotal, currency),
	}

	for _, item := range resp.Items {
		if item.SavedItem {
			continue
		}

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

		if item.OfferID != "" {
			entry.Product.Attributes = map[string]any{
				"offerId": item.OfferID,
				"itemId":  item.ItemID,
			}
		}

		cc.Items = append(cc.Items, entry)
	}

	return cc
}
