package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/saucesteals/shop/tracking"
)

func (o *textOutput) shipment(shipment tracking.Shipment) {
	o.line(nonempty(shipment.Label, shipment.TrackingNumber))
	o.field("Tracking", shipment.TrackingNumber)
	o.field("Merchant", shipment.Merchant)
	o.field("Order", shipment.OrderID)
	o.field("Note", shipment.Note)
	if shipment.Tracking == nil {
		o.field("Status", "Not refreshed yet")

		return
	}
	o.snapshot(shipment.Tracking, false)
}

func (o *textOutput) snapshot(snapshot *tracking.Snapshot, history bool) {
	o.field("Source", snapshot.Source)
	o.field("Expected delivery", nonempty(snapshot.ExpectedDelivery, "Not provided"))
	if latest := snapshot.Latest(); latest != nil {
		o.event("Latest scan", *latest)
	} else {
		o.field("Latest scan", "Not available")
	}
	if history && len(snapshot.Events) > 1 {
		o.section()
		o.line("Earlier scans (newest first)")
		for _, event := range snapshot.Events[1:] {
			o.event("Scan", event)
		}
	}
	if !snapshot.FetchedAt.IsZero() {
		o.field("Retrieved", snapshot.FetchedAt.Format(time.RFC3339))
	}
	o.field("Carrier freshness", nonempty(snapshot.Freshness, "unknown"))
	o.field("Link", snapshot.URL)
}

func (o *textOutput) event(label string, event tracking.Event) {
	o.field(label, event.Description)
	o.field("When", strings.TrimSpace(event.Date+" "+event.Time))
	o.field("Where", event.Location)
}

func (o *textOutput) refresh(summary refreshSummary) {
	o.line(fmt.Sprintf("%d shipments / %d refreshed / %d failed", summary.Total, summary.Refreshed, summary.Failed))
	for _, item := range summary.Shipments {
		o.section()
		o.line(nonempty(item.Label, item.TrackingNumber))
		o.field("Tracking", item.TrackingNumber)
		o.field("Merchant", item.Merchant)
		o.field("Order", item.OrderID)
		o.field("Note", item.Note)
		if item.Error != nil {
			o.field("Refresh failed", fmt.Sprintf("[%s] %s", item.Error.Code, item.Error.Message))
			if item.Latest != nil {
				o.field("Data", "Previous saved snapshot (not refreshed)")
			}
		}
		o.field("Expected delivery", nonempty(item.ExpectedDelivery, "Not provided"))
		if item.Latest != nil {
			o.event("Latest scan", *item.Latest)
		} else {
			o.field("Latest scan", "Not available")
		}
		if !item.FetchedAt.IsZero() {
			o.field("Retrieved", item.FetchedAt.Format(time.RFC3339))
		}
		o.field("Carrier freshness", nonempty(item.Freshness, "unknown"))
		o.field("Link", item.URL)
	}
}
