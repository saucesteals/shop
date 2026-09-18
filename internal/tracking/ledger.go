package tracking

import (
	"encoding/json"
	"errors"
	"os"
	"time"

	"github.com/saucesteals/shop"
	"github.com/saucesteals/shop/internal/config"
)

// Ledger stores one private record per tracking number.
// Adding an existing number fails rather than silently overwriting its metadata.
type Ledger struct {
	ConfigDir string
}

// Add atomically publishes a record, without overwriting concurrent additions.
func (l Ledger) Add(entry shop.Shipment) (*shop.Shipment, error) {
	number, err := Number(entry.TrackingNumber)
	if err != nil {
		return nil, err
	}
	entry.TrackingNumber = number
	entry.AddedAt = time.Now().UTC()
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := config.CreateState(l.ConfigDir, "", "shipments", number, data); err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, shop.Errorf(shop.ErrInvalidInput, "shipment already saved")
		}
		return nil, ledgerError(err)
	}

	return &entry, nil
}

// List returns saved shipments in tracking-number order without network requests.
func (l Ledger) List() ([]shop.Shipment, error) {
	keys, err := config.ListStates(l.ConfigDir, "", "shipments")
	if err != nil {
		return nil, ledgerError(err)
	}
	result := make([]shop.Shipment, 0, len(keys))
	for _, key := range keys {
		data, err := config.LoadState(l.ConfigDir, "", "shipments", key)
		if err != nil {
			return nil, ledgerError(err)
		}
		if data == nil {
			continue
		}
		var record struct {
			shop.Shipment
			History *shop.TrackingSnapshot `json:"history,omitempty"`
		}
		if err := json.Unmarshal(data, &record); err != nil {
			return nil, ledgerError(err)
		}
		shipment := record.Shipment
		if shipment.Tracking == nil && record.History != nil {
			shipment.Tracking = record.History
			if err := l.save(shipment); err != nil {
				return nil, ledgerError(err)
			}
		}
		if err := l.migrateHistory(&shipment); err != nil {
			return nil, ledgerError(err)
		}
		result = append(result, shipment)
	}

	return result, nil
}

// Remove deletes the local shipment and history, not the carrier shipment.
func (l Ledger) Remove(value string) error {
	number, err := Number(value)
	if err != nil {
		return err
	}
	if err := config.DeleteState(l.ConfigDir, "", "shipment-history", number); err != nil {
		return ledgerError(err)
	}
	if err := config.DeleteState(l.ConfigDir, "", "shipments", number); err != nil && !errors.Is(err, os.ErrNotExist) {
		return ledgerError(err)
	}

	return nil
}

func ledgerError(err error) error {
	return shop.Errorf(shop.ErrConfigError, "shipment ledger: %v", err)
}

// save replaces a shipment record atomically through the shared state layer.
func (l Ledger) save(shipment shop.Shipment) error {
	data, err := json.MarshalIndent(shipment, "", "  ")
	if err != nil {
		return err
	}

	return config.SaveState(l.ConfigDir, "", "shipments", shipment.TrackingNumber, data)
}

// migrateHistory folds legacy split snapshots into their shipment before cleanup.
// An existing tracking snapshot wins, making interrupted cleanup safe to retry.
func (l Ledger) migrateHistory(shipment *shop.Shipment) error {
	data, err := config.LoadState(l.ConfigDir, "", "shipment-history", shipment.TrackingNumber)
	if err != nil || data == nil {
		return err
	}
	if shipment.Tracking == nil {
		var history shop.TrackingSnapshot
		if err := json.Unmarshal(data, &history); err != nil {
			return err
		}
		if history.TrackingNumber != shipment.TrackingNumber || len(history.Events) == 0 {
			return errors.New("invalid saved shipment history")
		}
		shipment.Tracking = &history
		if err := l.save(*shipment); err != nil {
			return err
		}
	}

	return config.DeleteState(l.ConfigDir, "", "shipment-history", shipment.TrackingNumber)
}
