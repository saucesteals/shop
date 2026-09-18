package shop

import "time"

// Shipment is a saved package: local attribution and its last successful lookup.
// Tracking is nil until the first successful refresh.
type Shipment struct {
	TrackingNumber string            `json:"trackingNumber"`
	Label          string            `json:"label,omitempty"`
	Merchant       string            `json:"merchant,omitempty"`
	OrderID        string            `json:"orderId,omitempty"`
	Note           string            `json:"note,omitempty"`
	AddedAt        time.Time         `json:"addedAt"`
	Tracking       *TrackingSnapshot `json:"tracking,omitempty"`
}

// TrackingSnapshot is the scan history returned by one successful lookup.
// FetchedAt is the retrieval time, not the time the carrier last updated its data.
type TrackingSnapshot struct {
	TrackingNumber string    `json:"trackingNumber"`
	Source         string    `json:"source"`
	URL            string    `json:"url"`
	FetchedAt      time.Time `json:"fetchedAt"`
	// Freshness describes upstream freshness; "unknown" makes no live-data claim.
	Freshness string `json:"freshness"`
	// Events are ordered newest first by the source.
	Events []TrackingEvent `json:"events"`
}

// TrackingEvent is one carrier scan. Date and Time retain the source's local
// text because a timezone is not supplied; they must not be interpreted as UTC.
type TrackingEvent struct {
	Date        string `json:"date"`
	Time        string `json:"time,omitempty"`
	Description string `json:"description"`
	Location    string `json:"location,omitempty"`
}
