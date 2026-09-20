package providers

import "time"

// deliveryWindow formats carrier timestamps without converting their timezone.
// Each endpoint retains its date and numeric offset, including overnight windows.
func deliveryWindow(start, end time.Time) string {
	const layout = "Mon, Jan 2, 2006 3:04 PM -07:00"

	return start.Format(layout) + " – " + end.Format(layout)
}
