package tracking

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/saucesteals/shop"
)

// Ledger stores one private immutable attribution record per tracking number.
// Adding an existing number fails rather than silently overwriting its metadata.
type Ledger struct {
	Dir string
}

// Add atomically publishes a record, without overwriting concurrent additions.
func (l Ledger) Add(entry shop.Shipment) (*shop.Shipment, error) {
	number, err := Number(entry.TrackingNumber)
	if err != nil {
		return nil, err
	}
	entry.TrackingNumber = number
	entry.AddedAt = time.Now().UTC().Format(time.RFC3339)
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(l.Dir, 0o700); err != nil {
		return nil, ledgerError(err)
	}
	file, err := os.CreateTemp(l.Dir, ".shipment-*")
	if err != nil {
		return nil, ledgerError(err)
	}
	defer func() { _ = os.Remove(file.Name()) }()
	_, writeErr := file.Write(append(data, '\n'))
	if writeErr == nil {
		writeErr = file.Sync()
	}
	closeErr := file.Close()
	if writeErr != nil {
		return nil, ledgerError(writeErr)
	}
	if closeErr != nil {
		return nil, ledgerError(closeErr)
	}
	if err := os.Link(file.Name(), l.path(number)); err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, shop.Errorf(shop.ErrInvalidInput, "shipment already saved")
		}
		return nil, ledgerError(err)
	}

	return &entry, nil
}

// List returns saved shipments in tracking-number order without network requests.
func (l Ledger) List() ([]shop.Shipment, error) {
	entries, err := os.ReadDir(l.Dir)
	if errors.Is(err, os.ErrNotExist) {
		return []shop.Shipment{}, nil
	}
	if err != nil {
		return nil, ledgerError(err)
	}
	result := make([]shop.Shipment, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(l.Dir, entry.Name()))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, ledgerError(err)
		}
		var shipment shop.Shipment
		if err := json.Unmarshal(data, &shipment); err != nil {
			return nil, ledgerError(err)
		}
		result = append(result, shipment)
	}

	return result, nil
}

// Remove deletes local attribution only; it does not affect the carrier shipment.
func (l Ledger) Remove(value string) error {
	number, err := Number(value)
	if err != nil {
		return err
	}
	if err := os.Remove(l.path(number)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return ledgerError(err)
	}

	return nil
}

func (l Ledger) path(number string) string {
	return filepath.Join(l.Dir, number+".json")
}

func ledgerError(err error) error {
	return shop.Errorf(shop.ErrConfigError, "shipment ledger: %v", err)
}
