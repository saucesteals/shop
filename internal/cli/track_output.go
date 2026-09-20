package cli

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/saucesteals/shop"
	"github.com/saucesteals/shop/tracking"
)

// JSON formatting belongs to the CLI, not the storage or tracking API.
type refreshSummary struct {
	Total     int               `json:"total"`
	Refreshed int               `json:"refreshed"`
	Failed    int               `json:"failed"`
	Shipments []shipmentSummary `json:"shipments"`
}

type shipmentSummary struct {
	tracking.Shipment
	ExpectedDelivery string          `json:"expectedDelivery,omitempty"`
	Latest           *tracking.Event `json:"latest,omitempty"`
	FetchedAt        time.Time       `json:"fetchedAt,omitzero"`
	URL              string          `json:"url,omitempty"`
	Freshness        string          `json:"freshness"`
	Refreshed        bool            `json:"refreshed"`
	Error            *shop.Error     `json:"error,omitempty"`
}

func summarizeRefresh(results []tracking.Result) refreshSummary {
	summary := refreshSummary{
		Total:     len(results),
		Shipments: make([]shipmentSummary, 0, len(results)),
	}
	for _, result := range results {
		item := shipmentSummary{
			Shipment:  result.Shipment,
			Freshness: "unknown",
			Refreshed: result.Err == nil,
		}
		if snapshot := item.Tracking; snapshot != nil {
			item.ExpectedDelivery = snapshot.ExpectedDelivery
			item.Latest = snapshot.Latest()
			item.FetchedAt = snapshot.FetchedAt
			item.URL = snapshot.URL
			item.Freshness = snapshot.Freshness
		}
		item.Tracking = nil
		if result.Err != nil {
			item.Error = shipmentError(result.Err, shop.ErrNetwork)
			summary.Failed++
		} else {
			summary.Refreshed++
		}
		summary.Shipments = append(summary.Shipments, item)
	}

	return summary
}

func shipmentError(err error, fallback shop.ErrorCode) *shop.Error {
	var structured *shop.Error
	if errors.As(err, &structured) {
		return structured
	}
	if errors.Is(err, os.ErrExist) {
		return shop.Errorf(shop.ErrInvalidInput, "shipment already saved")
	}
	if errors.Is(err, os.ErrNotExist) {
		return shop.Errorf(shop.ErrNotFound, "shipment is not saved; use track add first")
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return shop.Errorf(shop.ErrNetwork, "shipment operation canceled: %v", err)
	}

	return shop.Errorf(fallback, "shipment operation failed: %v", err)
}
