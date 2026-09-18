package shop

// TrackingResult is a provider's available shipment history, not a live-carrier guarantee.
type TrackingResult struct {
	TrackingNumber string          `json:"trackingNumber"`
	Source         string          `json:"source"`
	URL            string          `json:"url"`
	FetchedAt      string          `json:"fetchedAt"`
	Freshness      string          `json:"freshness"`
	Events         []TrackingEvent `json:"events"`
}

// TrackingEvent preserves source-local dates and times without inventing a timezone.
type TrackingEvent struct {
	Date        string `json:"date"`
	Time        string `json:"time,omitempty"`
	Description string `json:"description"`
	Location    string `json:"location,omitempty"`
}
