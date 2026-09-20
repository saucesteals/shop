package tracking

import (
	"context"
	"fmt"
	"time"
)

// RefreshOptions controls a saved-shipment batch. Zero values select active
// shipments and use only the caller's deadline. Timeout bounds each lookup
// independently; a failure does not consume the next shipment's allowance.
type RefreshOptions struct {
	Filter  Filter
	Timeout time.Duration
}

// Result is one refresh outcome. Shipment contains the saved record, including
// its full snapshot. Err is nil only after the new snapshot has been persisted.
// On failure, Shipment retains the last record read; it is not a fresh lookup.
type Result struct {
	Shipment Shipment
	Err      error
}

// Refresh fetches and saves one existing shipment, regardless of delivery age.
// On lookup or save failure it returns the previous saved record with the error.
// No record is written on lookup failure. Caller controls cancellation and timeout.
func (s *Service) Refresh(ctx context.Context, number string) (*Shipment, error) {
	shipment, err := s.Get(ctx, number)
	if err != nil {
		return nil, err
	}
	if s.tracker == nil {
		return shipment, fmt.Errorf("tracking client is required")
	}
	snapshot, err := s.Track(ctx, shipment.TrackingNumber)
	if ctx.Err() != nil {
		return shipment, ctx.Err()
	}
	if err != nil {
		return shipment, err
	}
	// Re-read after the network request so an intervening removal or metadata
	// edit is observed. This is not a cross-process compare-and-swap operation.
	current, err := s.Get(ctx, shipment.TrackingNumber)
	if err != nil {
		return shipment, err
	}
	previous := *current
	current.Tracking = snapshot
	if err := s.Update(ctx, *current); err != nil {
		return &previous, err
	}

	return current, nil
}

// RefreshAll refreshes the selected records sequentially in tracking-number order.
// Per-shipment errors remain in results and do not abort the batch. A top-level
// error means invalid options, a listing failure, or caller cancellation. On
// cancellation, results for attempted shipments are returned alongside the error.
func (s *Service) RefreshAll(ctx context.Context, options RefreshOptions) ([]Result, error) {
	if s.tracker == nil {
		return nil, fmt.Errorf("tracking client is required")
	}
	if options.Timeout < 0 {
		return nil, fmt.Errorf("refresh timeout must not be negative")
	}
	shipments, err := s.List(ctx, options.Filter)
	if err != nil {
		return nil, err
	}
	results := make([]Result, 0, len(shipments))
	for _, shipment := range shipments {
		if err := ctx.Err(); err != nil {
			return results, err
		}
		lookupCtx := ctx
		cancel := func() {}
		if options.Timeout > 0 {
			lookupCtx, cancel = context.WithTimeout(ctx, options.Timeout)
		}
		updated, err := s.Refresh(lookupCtx, shipment.TrackingNumber)
		cancel()
		if updated != nil {
			shipment = *updated
		}
		results = append(results, Result{
			Shipment: shipment,
			Err:      err,
		})
	}

	return results, ctx.Err()
}
