package tracking

import (
	"context"
	"errors"
	"time"

	"github.com/saucesteals/shop"
)

// RefreshResult summarizes a batch without discarding individual failures.
type RefreshResult struct {
	Total     int       `json:"total"`
	Refreshed int       `json:"refreshed"`
	Failed    int       `json:"failed"`
	Shipments []Summary `json:"shipments"`
}

// Summary combines local attribution and the latest successful lookup.
type Summary struct {
	Shipment
	ExpectedDelivery string      `json:"expectedDelivery,omitempty"`
	Latest           *Event      `json:"latest,omitempty"`
	FetchedAt        time.Time   `json:"fetchedAt,omitzero"`
	URL              string      `json:"url,omitempty"`
	Freshness        string      `json:"freshness"`
	Refreshed        bool        `json:"refreshed"`
	Error            *shop.Error `json:"error,omitempty"`
}

// Refresh updates saved snapshots, preserving previous history on lookup failure.
// An empty number applies selection to the ledger. An explicit number bypasses
// selection. Lookups share the caller's deadline.
func (l Ledger) Refresh(ctx context.Context, tracker Tracker, number string, selection Selection) (*RefreshResult, error) {
	if err := selection.Validate(); err != nil {
		return nil, err
	}
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
	if number == "" {
		entries = selection.Select(entries, time.Now())
	}
	result := &RefreshResult{Shipments: make([]Summary, 0, len(entries))}
	for _, entry := range entries {
		if number != "" && entry.TrackingNumber != number {
			continue
		}
		summary := Summary{Shipment: entry, Freshness: "unknown"}
		// Keep the full history in storage/list output, not the compact summary.
		summary.Tracking = nil
		if entry.Tracking != nil {
			summary.apply(entry.Tracking)
		}
		snapshot, lookupErr := tracker.Track(ctx, entry.TrackingNumber)
		if lookupErr == nil && (snapshot == nil || len(snapshot.Events) == 0 || snapshot.TrackingNumber != entry.TrackingNumber) {
			lookupErr = shop.Errorf(shop.ErrUpstream, "tracking source returned an invalid snapshot")
		}
		if lookupErr == nil {
			entry.Tracking = snapshot
			lookupErr = l.save(entry)
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

func (s *Summary) apply(snapshot *Snapshot) {
	if len(snapshot.Events) > 0 {
		event := snapshot.Events[0]
		s.Latest = &event
	}
	s.ExpectedDelivery = snapshot.ExpectedDelivery
	s.FetchedAt = snapshot.FetchedAt
	s.URL = snapshot.URL
	s.Freshness = snapshot.Freshness
}
