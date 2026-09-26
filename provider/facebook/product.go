package facebook

import (
	"context"
	"encoding/json"

	"github.com/saucesteals/shop"
)

// Product returns details for a Marketplace listing ID or item URL.
func (s *Store) Product(ctx context.Context, value string) (*shop.Product, error) {
	id, err := productID(value)
	if err != nil {
		return nil, err
	}
	body, err := s.document(ctx, "/marketplace/item/"+id+"/")
	if err != nil {
		return nil, err
	}
	var item listing
	found := false
	err = relayData(body, func(raw json.RawMessage) error {
		var data struct {
			Viewer struct {
				Details struct {
					Target json.RawMessage `json:"target"`
				} `json:"marketplace_product_details_page"`
			} `json:"viewer"`
		}
		if err := json.Unmarshal(raw, &data); err != nil {
			return shop.Errorf(shop.ErrUpstream, "decode marketplace details: %v", err)
		}
		target := data.Viewer.Details.Target
		if len(target) == 0 {
			return nil
		}
		var identity struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(target, &identity); err != nil {
			return shop.Errorf(shop.ErrUpstream, "decode marketplace listing identity: %v", err)
		}
		if identity.ID != id {
			return nil
		}
		if err := json.Unmarshal(target, &item); err != nil {
			return shop.Errorf(shop.ErrUpstream, "decode marketplace listing: %v", err)
		}
		found = true

		return nil
	})
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, shop.Errorf(shop.ErrUpstream, "marketplace detail Relay payload missing for listing %s", id)
	}

	return item.product(false)
}
