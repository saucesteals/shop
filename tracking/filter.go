package tracking

import (
	"strings"
	"time"
)

// Filter controls which saved shipments are displayed or refreshed.
// The zero value includes unresolved shipments and deliveries dated today.
// DeliveredSince uses its calendar date, ignoring the time of day.
type Filter struct {
	All            bool
	DeliveredSince time.Time
}

// Validate rejects conflicting filters.
func (s Filter) Validate() error {
	if s.All && !s.DeliveredSince.IsZero() {
		return &Error{Kind: ErrInvalidInput, Message: "all and delivered-since filters cannot be combined"}
	}

	return nil
}

// Includes uses the latest saved scan, never retrieval time, to select a shipment.
// Unknown statuses or dates remain eligible. Offset-bearing dates use now's local
// timezone; carrier dates without offsets retain their reported calendar date.
func (s Filter) Includes(entry Shipment, now time.Time) bool {
	if s.All || entry.Tracking == nil || len(entry.Tracking.Events) == 0 {
		return true
	}
	latest := entry.Tracking.Events[0]
	status := strings.ToLower(strings.TrimSpace(latest.Description))
	// Conservative terminal wording shared by the supported providers. Do not
	// mistake "out for delivery", "not delivered", or handoffs for completion.
	delivered := status == "delivered" || strings.HasPrefix(status, "delivered,") || strings.HasPrefix(status, "delivered.") || strings.HasPrefix(status, "delivered:")
	if !delivered {
		return true
	}
	var day string
	if at, err := time.Parse(time.RFC3339Nano, latest.Date+"T"+latest.Time); err == nil {
		day = at.In(now.Location()).Format(time.DateOnly)
	} else {
		for _, layout := range []string{time.DateOnly, "Mon, Jan 2, 2006", "01/02/2006", "1/2/2006", "January 2, 2006", "Jan 2, 2006"} {
			if at, err := time.Parse(layout, strings.TrimSpace(latest.Date)); err == nil {
				day = at.Format(time.DateOnly)
				break
			}
		}
	}
	if day == "" {
		return true
	}
	cutoff := s.DeliveredSince.Format(time.DateOnly)
	if s.DeliveredSince.IsZero() {
		cutoff = now.Format(time.DateOnly)
	}

	return day >= cutoff
}

// Select returns matching entries in their original order without altering state.
func (s Filter) Select(entries []Shipment, now time.Time) []Shipment {
	selected := make([]Shipment, 0, len(entries))
	for _, entry := range entries {
		if s.Includes(entry, now) {
			selected = append(selected, entry)
		}
	}

	return selected
}
