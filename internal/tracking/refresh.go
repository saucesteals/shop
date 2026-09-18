package tracking

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/saucesteals/shop"
	"github.com/saucesteals/shop/internal/config"
)

// Tracker retrieves a shipment's available history.
type Tracker interface {
	Track(context.Context, string) (*shop.TrackingResult, error)
}

// RefreshResult summarizes a batch without discarding individual failures.
type RefreshResult struct {
	Total     int       `json:"total"`
	Refreshed int       `json:"refreshed"`
	Failed    int       `json:"failed"`
	Shipments []Summary `json:"shipments"`
}

// Summary combines local attribution and the latest successful lookup.
type Summary struct {
	shop.Shipment
	Latest    *shop.TrackingEvent `json:"latest,omitempty"`
	FetchedAt string              `json:"fetchedAt,omitempty"`
	URL       string              `json:"url,omitempty"`
	Freshness string              `json:"freshness"`
	Refreshed bool                `json:"refreshed"`
	Error     *shop.Error         `json:"error,omitempty"`
}

// Refresh updates saved snapshots, preserving previous history on lookup failure.
// An empty number selects the whole ledger. Lookups share the caller's deadline.
func (l Ledger) Refresh(ctx context.Context, tracker Tracker, number string) (*RefreshResult, error) {
	if number != "" {
		var err error
		number, err = Number(number)
		if err != nil {
			return nil, err
		}
	}
	entries, err := l.List()
	if err != nil {
		return nil, err
	}
	result := &RefreshResult{Shipments: make([]Summary, 0, len(entries))}
	for _, entry := range entries {
		if number != "" && entry.TrackingNumber != number {
			continue
		}
		summary := Summary{Shipment: entry, Freshness: "unknown"}
		previous, loadErr := config.LoadState(l.ConfigDir, "", "shipment-history", entry.TrackingNumber)
		if loadErr != nil {
			return nil, ledgerError(loadErr)
		}
		if previous != nil {
			var snapshot shop.TrackingResult
			if err := json.Unmarshal(previous, &snapshot); err != nil {
				return nil, ledgerError(err)
			}
			summary.apply(&snapshot)
		}
		snapshot, lookupErr := tracker.Track(ctx, entry.TrackingNumber)
		if lookupErr == nil && (snapshot == nil || len(snapshot.Events) == 0 || snapshot.TrackingNumber != entry.TrackingNumber) {
			lookupErr = shop.Errorf(shop.ErrUpstream, "tracking source returned an invalid snapshot")
		}
		if lookupErr == nil {
			data, marshalErr := json.Marshal(snapshot)
			if marshalErr != nil {
				lookupErr = marshalErr
			} else {
				lookupErr = config.SaveState(l.ConfigDir, "", "shipment-history", entry.TrackingNumber, data)
			}
			if lookupErr == nil {
				summary.apply(snapshot)
				summary.Refreshed = true
				result.Refreshed++
			} else {
				lookupErr = ledgerError(lookupErr)
			}
		}
		if lookupErr != nil {
			var structured *shop.Error
			if !errors.As(lookupErr, &structured) {
				structured = shop.Errorf(shop.ErrNetwork, "shipment refresh failed")
			}
			summary.Error = structured
			result.Failed++
		}
		result.Shipments = append(result.Shipments, summary)
	}
	result.Total = len(result.Shipments)
	if number != "" && result.Total == 0 {
		return nil, shop.Errorf(shop.ErrNotFound, "shipment is not saved; use track add first")
	}

	return result, nil
}

func (s *Summary) apply(snapshot *shop.TrackingResult) {
	if len(snapshot.Events) > 0 {
		event := snapshot.Events[0]
		s.Latest = &event
	}
	s.FetchedAt = snapshot.FetchedAt
	s.URL = snapshot.URL
	s.Freshness = snapshot.Freshness
}
