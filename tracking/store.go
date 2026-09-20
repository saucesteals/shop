package tracking

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/saucesteals/shop/internal/config"
)

// Service persists shipment attribution and the last successful tracking snapshot.
// Track performs an unsaved lookup; Refresh and RefreshAll persist successful results.
// Writes are atomic, but read-modify-write operations are not cross-process transactions.
type Service struct {
	configDir string
	tracker   Tracker
}

// New uses state/shipments beneath a Shop configuration directory.
// It does not create files until the first write. Service owns no open resources.
func New(configDir string, tracker Tracker) *Service {
	return &Service{
		configDir: configDir,
		tracker:   tracker,
	}
}

// Add saves a new shipment and sets AddedAt. Existing records are never overwritten.
// The returned error preserves os.ErrExist for errors.Is checks on duplicates.
func (s *Service) Add(ctx context.Context, shipment Shipment) (*Shipment, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	number, err := NormalizeNumber(shipment.TrackingNumber)
	if err != nil {
		return nil, err
	}
	shipment.TrackingNumber = number
	shipment.AddedAt = time.Now().UTC()
	data, err := encodeShipment(shipment)
	if err != nil {
		return nil, err
	}
	if err := config.CreateState(s.configDir, "", "shipments", number, data); err != nil {
		return nil, fmt.Errorf("add shipment: %w", err)
	}

	return &shipment, nil
}

// Get reads one saved shipment without scanning the collection or contacting a carrier.
// A missing record returns an error wrapping os.ErrNotExist.
func (s *Service) Get(ctx context.Context, number string) (*Shipment, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	number, err := NormalizeNumber(number)
	if err != nil {
		return nil, err
	}
	data, err := config.LoadState(s.configDir, "", "shipments", number)
	if err != nil {
		return nil, fmt.Errorf("get shipment: %w", err)
	}
	if data == nil {
		return nil, fmt.Errorf("get shipment: %w", os.ErrNotExist)
	}
	var shipment Shipment
	if err := json.Unmarshal(data, &shipment); err != nil {
		return nil, fmt.Errorf("decode shipment: %w", err)
	}
	if shipment.TrackingNumber != number || (shipment.Tracking != nil && shipment.Tracking.TrackingNumber != number) {
		return nil, fmt.Errorf("saved shipment identity does not match its key")
	}

	return &shipment, nil
}

// List reads matching records offline in tracking-number order. A zero Filter
// selects active shipments and deliveries dated today; All includes every record.
func (s *Service) List(ctx context.Context, filter Filter) ([]Shipment, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := filter.Validate(); err != nil {
		return nil, err
	}
	keys, err := config.ListStates(s.configDir, "", "shipments")
	if err != nil {
		return nil, fmt.Errorf("list shipments: %w", err)
	}
	shipments := make([]Shipment, 0, len(keys))
	now := time.Now()
	for _, key := range keys {
		shipment, err := s.Get(ctx, key)
		if errors.Is(err, os.ErrNotExist) {
			continue // A concurrent removal is not a failed listing.
		}
		if err != nil {
			return nil, err
		}
		if filter.Includes(*shipment, now) {
			shipments = append(shipments, *shipment)
		}
	}

	return shipments, nil
}

// Update replaces an existing record, preserving its original AddedAt.
// Read with Get, modify the attribution or snapshot, then pass the record here.
// Updating a missing record never intentionally creates it; concurrent writes are last-writer-wins.
func (s *Service) Update(ctx context.Context, shipment Shipment) error {
	current, err := s.Get(ctx, shipment.TrackingNumber)
	if err != nil {
		return err
	}
	shipment.TrackingNumber = current.TrackingNumber
	shipment.AddedAt = current.AddedAt
	data, err := encodeShipment(shipment)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := config.SaveState(s.configDir, "", "shipments", shipment.TrackingNumber, data); err != nil {
		return fmt.Errorf("update shipment: %w", err)
	}

	return nil
}

// Remove deletes a local record, not the carrier shipment. Missing records are ignored.
func (s *Service) Remove(ctx context.Context, number string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	number, err := NormalizeNumber(number)
	if err != nil {
		return err
	}
	if err := config.DeleteState(s.configDir, "", "shipments", number); err != nil {
		return fmt.Errorf("remove shipment: %w", err)
	}

	return nil
}

func encodeShipment(shipment Shipment) ([]byte, error) {
	if shipment.Tracking != nil {
		if err := validateSnapshot(shipment.Tracking, shipment.TrackingNumber); err != nil {
			return nil, err
		}
	}
	data, err := json.MarshalIndent(shipment, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode shipment: %w", err)
	}

	return data, nil
}

// Track retrieves a snapshot without saving or changing local shipment state.
func (s *Service) Track(ctx context.Context, number string) (*Snapshot, error) {
	if s.tracker == nil {
		return nil, fmt.Errorf("tracking client is required")
	}
	number, err := NormalizeNumber(number)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	snapshot, err := s.tracker.Track(ctx, number)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, err
	}
	if err := validateSnapshot(snapshot, number); err != nil {
		return nil, err
	}
	return snapshot, nil
}
