package fedex

import (
	"encoding/json"
	"strings"

	"github.com/saucesteals/shop"
	"github.com/saucesteals/shop/internal/tracking"
)

type response struct {
	Output struct {
		Packages []struct {
			Number    string `json:"trackingNbr"`
			ErrorCode string `json:"trackErrCD"`
			Events    []struct {
				Date     string `json:"date"`
				Time     string `json:"time"`
				Offset   string `json:"gmtOffset"`
				Status   string `json:"status"`
				Details  string `json:"scanDetails"`
				Location string `json:"scanLocation"`
			} `json:"scanEventList"`
		} `json:"packages"`
	} `json:"output"`
}

func parse(body []byte, number string) ([]shop.TrackingEvent, error) {
	var result response
	if json.Unmarshal(body, &result) != nil {
		return nil, tracking.UpstreamError("invalid_response")
	}
	if len(result.Output.Packages) != 1 {
		return nil, tracking.UpstreamError("shipment_unavailable")
	}
	shipment := result.Output.Packages[0]
	if shipment.Number != number {
		return nil, tracking.UpstreamError("shipment_mismatch")
	}
	if shipment.ErrorCode != "" {
		return nil, tracking.UpstreamError("shipment_unavailable")
	}
	if len(shipment.Events) == 0 {
		return nil, tracking.UpstreamError("history_unavailable")
	}
	events := make([]shop.TrackingEvent, 0, len(shipment.Events))
	for _, scan := range shipment.Events {
		description := strings.TrimSpace(scan.Status)
		if details := strings.TrimSpace(scan.Details); details != "" && details != description {
			if description != "" {
				description += ": "
			}
			description += details
		}
		if scan.Date == "" || description == "" {
			return nil, tracking.UpstreamError("invalid_response")
		}
		scanTime := scan.Time
		if scanTime != "" {
			scanTime += scan.Offset
		}
		events = append(events, shop.TrackingEvent{
			Date:        scan.Date,
			Time:        scanTime,
			Description: description,
			Location:    scan.Location,
		})
	}

	return events, nil
}
