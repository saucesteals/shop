package fedex

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/saucesteals/shop"
	"github.com/saucesteals/shop/internal/tracking"
)

type numberInfo struct {
	Number string `json:"trackingNumber"`
}

type response struct {
	Output struct {
		Shipments []struct {
			Number  string `json:"trackingNumber"`
			Results []struct {
				NumberInfo numberInfo `json:"trackingNumberInfo"`
				Error      struct {
					Code string `json:"code"`
				} `json:"error"`
				Events []struct {
					Date        time.Time `json:"date"`
					Description string    `json:"eventDescription"`
					Exception   string    `json:"exceptionDescription"`
					Location    struct {
						City    string `json:"city"`
						State   string `json:"stateOrProvinceCode"`
						Country string `json:"countryCode"`
					} `json:"scanLocation"`
				} `json:"scanEvents"`
			} `json:"trackResults"`
		} `json:"completeTrackResults"`
	} `json:"output"`
}

func parse(body []byte, number string) ([]shop.TrackingEvent, error) {
	var result response
	if json.Unmarshal(body, &result) != nil {
		return nil, tracking.UpstreamError("invalid_response")
	}
	if len(result.Output.Shipments) != 1 {
		return nil, tracking.UpstreamError("shipment_unavailable")
	}
	shipment := result.Output.Shipments[0]
	if shipment.Number != number {
		return nil, tracking.UpstreamError("shipment_mismatch")
	}
	// Reused numbers may have multiple shipments; never select one arbitrarily.
	if len(shipment.Results) != 1 {
		return nil, tracking.UpstreamError("shipment_unavailable")
	}
	detail := shipment.Results[0]
	if detail.NumberInfo.Number != number {
		return nil, tracking.UpstreamError("shipment_mismatch")
	}
	if detail.Error.Code != "" {
		return nil, tracking.UpstreamError("shipment_unavailable")
	}
	if len(detail.Events) == 0 {
		return nil, tracking.UpstreamError("history_unavailable")
	}
	sort.SliceStable(detail.Events, func(i, j int) bool {
		return detail.Events[i].Date.After(detail.Events[j].Date)
	})

	events := make([]shop.TrackingEvent, 0, len(detail.Events))
	for _, scan := range detail.Events {
		when := scan.Date
		description := strings.TrimSpace(scan.Description)
		if when.IsZero() || description == "" {
			return nil, tracking.UpstreamError("invalid_response")
		}
		if exception := strings.TrimSpace(scan.Exception); exception != "" && exception != description {
			description += ": " + exception
		}
		var location []string
		for _, part := range []string{scan.Location.City, scan.Location.State, scan.Location.Country} {
			if part = strings.TrimSpace(part); part != "" {
				location = append(location, part)
			}
		}
		events = append(events, shop.TrackingEvent{
			Date:        when.Format("2006-01-02"),
			Time:        when.Format("15:04:05Z07:00"),
			Description: description,
			Location:    strings.Join(location, ", "),
		})
	}

	return events, nil
}
