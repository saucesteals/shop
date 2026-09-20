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
	if structured := trackingError(err); structured != nil {
		return structured
	}
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

func trackingError(err error) *shop.Error {
	var code shop.ErrorCode
	switch {
	case errors.Is(err, tracking.ErrInvalidInput):
		code = shop.ErrInvalidInput
	case errors.Is(err, tracking.ErrNotSupported):
		code = shop.ErrNotSupported
	case errors.Is(err, tracking.ErrRateLimited):
		code = shop.ErrRateLimited
	case errors.Is(err, tracking.ErrNetwork):
		code = shop.ErrNetwork
	case errors.Is(err, tracking.ErrUpstream):
		code = shop.ErrUpstream
	case errors.Is(err, tracking.ErrInternal):
		code = shop.ErrInternal
	default:
		return nil
	}
	result := shop.Errorf(code, "%s", err)
	var failure *tracking.Error
	if !errors.As(err, &failure) {
		return result
	}
	if failure.Reason != "" || failure.StatusCode != 0 || code == shop.ErrRateLimited {
		result.Details = make(map[string]any)
		if failure.Reason != "" {
			result.Details["reason"] = failure.Reason
		}
		if failure.StatusCode != 0 {
			result.Details["status"] = failure.StatusCode
		}
		if code == shop.ErrRateLimited {
			result.Details["retryAfter"] = failure.RetryAfter
		}
	}
	return result
}
