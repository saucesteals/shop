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
func (l Ledger) Add(entry Shipment) (*Shipment, error) {
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
func (l Ledger) List() ([]Shipment, error) {
	keys, err := config.ListStates(l.ConfigDir, "", "shipments")
	if err != nil {
		return nil, ledgerError(err)
	}
	result := make([]Shipment, 0, len(keys))
	for _, key := range keys {
		data, err := config.LoadState(l.ConfigDir, "", "shipments", key)
		if err != nil {
			return nil, ledgerError(err)
		}
		if data == nil {
			continue
		}
		var shipment Shipment
		if err := json.Unmarshal(data, &shipment); err != nil {
			return nil, ledgerError(err)
		}
		result = append(result, shipment)
	}

	return result, nil
}

// Remove deletes the local shipment record, not the carrier shipment.
func (l Ledger) Remove(value string) error {
	number, err := Number(value)
	if err != nil {
		return err
	}
	if err := config.DeleteState(l.ConfigDir, "", "shipments", number); err != nil {
		return ledgerError(err)
	}

	return nil
}

func ledgerError(err error) error {
	return shop.Errorf(shop.ErrConfigError, "shipment ledger: %v", err)
}

// save replaces a shipment record atomically through the shared state layer.
func (l Ledger) save(shipment Shipment) error {
	data, err := json.MarshalIndent(shipment, "", "  ")
	if err != nil {
		return err
	}

	return config.SaveState(l.ConfigDir, "", "shipments", shipment.TrackingNumber, data)
}
