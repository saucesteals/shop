// Package carriers selects the available shipment tracking source.
package carriers

import (
	"context"

	"github.com/saucesteals/shop"
	"github.com/saucesteals/shop/internal/tracking"
	"github.com/saucesteals/shop/internal/tracking/trackglobal"
	"github.com/saucesteals/shop/internal/tracking/ups"
)

// Client routes supported carrier identifiers directly and uses the general source otherwise.
type Client struct{}

// Track retrieves history through the appropriate source.
func (c *Client) Track(ctx context.Context, number string) (*shop.TrackingSnapshot, error) {
	number, err := tracking.Number(number)
	if err != nil {
		return nil, err
	}
	if ups.Matches(number) {
		return (&ups.Client{}).Track(ctx, number)
	}

	return (&trackglobal.Client{}).Track(ctx, number)
}
