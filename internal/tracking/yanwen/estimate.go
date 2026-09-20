package yanwen

import (
	"encoding/json"
	"time"

	"github.com/saucesteals/shop/internal/tracking"
)

func parseEstimate(body []byte) string {
	var result struct {
		Success bool `json:"success"`
		Data    struct {
			Now       int64  `json:"now"`
			Remaining int64  `json:"remaining"`
			TimeZone  string `json:"timeZone"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &result) != nil || !result.Success || result.Data.Now <= 0 || result.Data.Remaining <= 0 {
		return ""
	}
	zone, err := time.LoadLocation(result.Data.TimeZone)
	if err != nil || result.Data.TimeZone == "" {
		return ""
	}
	now := time.UnixMilli(result.Data.Now).In(zone)
	// Match the carrier UI: only display during its 07:00–17:59 local window.
	if now.Hour() <= 6 || now.Hour() >= 18 {
		return ""
	}
	// The carrier publishes remaining milliseconds and displays a two-hour window.
	startMillis := result.Data.Now + result.Data.Remaining
	if startMillis < result.Data.Now {
		return ""
	}
	start := time.UnixMilli(startMillis).In(zone)

	return tracking.DeliveryWindow(start, start.Add(2*time.Hour))
}
