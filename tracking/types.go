// Package tracking provides shipment data, carrier routing, and local shipment storage.
package tracking

import (
	"time"

	"github.com/saucesteals/shop/internal/fault"
)

// Shipment is a saved package: local attribution and its last successful lookup.
// Tracking is nil until the first successful refresh.
type Shipment struct {
	TrackingNumber string    `json:"trackingNumber"`
	Label          string    `json:"label,omitempty"`
	Merchant       string    `json:"merchant,omitempty"`
	OrderID        string    `json:"orderId,omitempty"`
	Note           string    `json:"note,omitempty"`
	AddedAt        time.Time `json:"addedAt"`
	Tracking       *Snapshot `json:"tracking,omitempty"`
}

// Snapshot is the scan history returned by one successful lookup.
// FetchedAt is the retrieval time, not the time the carrier last updated its data.
type Snapshot struct {
	// ExpectedDelivery is the carrier-provided estimate, not a guarantee.
	ExpectedDelivery string    `json:"expectedDelivery,omitempty"`
	TrackingNumber   string    `json:"trackingNumber"`
	Source           string    `json:"source"`
	URL              string    `json:"url"`
	FetchedAt        time.Time `json:"fetchedAt"`
	// Freshness describes upstream freshness; "unknown" makes no live-data claim.
	Freshness string `json:"freshness"`
	// Events are ordered newest first.
	Events []Event `json:"events"`
}

// Event is one carrier scan. Date and Time retain the source's timezone
// context, when supplied; values without an offset must not be interpreted as UTC.
type Event struct {
	Date        string `json:"date"`
	Time        string `json:"time,omitempty"`
	Description string `json:"description"`
	Location    string `json:"location,omitempty"`
}

// Latest returns a copy of the newest scan, or nil if no history is available.
// A nil snapshot is valid, as saved shipments may not have been refreshed yet.
func (s *Snapshot) Latest() *Event {
	if s == nil || len(s.Events) == 0 {
		return nil
	}
	event := s.Events[0]

	return &event
}

func validateSnapshot(snapshot *Snapshot, number string) error {
	if snapshot == nil || len(snapshot.Events) == 0 || snapshot.TrackingNumber != number {
		return fault.Errorf(fault.ErrUpstream, "tracking source returned an invalid snapshot")
	}

	return nil
}
