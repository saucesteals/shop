package facebook

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode"

	"github.com/saucesteals/shop"
)

const (
	locationSearchDocID = "9660140454040174"
	locationSearchName  = "MarketplaceSearchAddressDataSourceQuery"
)

var lsdTokenPattern = regexp.MustCompile(`"LSD",\[\],\{"token":"([^"\\]+)"`)

var usStateNames = map[string]string{
	"AL": "Alabama", "AK": "Alaska", "AZ": "Arizona", "AR": "Arkansas", "CA": "California",
	"CO": "Colorado", "CT": "Connecticut", "DE": "Delaware", "FL": "Florida", "GA": "Georgia",
	"HI": "Hawaii", "ID": "Idaho", "IL": "Illinois", "IN": "Indiana", "IA": "Iowa", "KS": "Kansas",
	"KY": "Kentucky", "LA": "Louisiana", "ME": "Maine", "MD": "Maryland", "MA": "Massachusetts",
	"MI": "Michigan", "MN": "Minnesota", "MS": "Mississippi", "MO": "Missouri", "MT": "Montana",
	"NE": "Nebraska", "NV": "Nevada", "NH": "New Hampshire", "NJ": "New Jersey", "NM": "New Mexico",
	"NY": "New York", "NC": "North Carolina", "ND": "North Dakota", "OH": "Ohio", "OK": "Oklahoma",
	"OR": "Oregon", "PA": "Pennsylvania", "RI": "Rhode Island", "SC": "South Carolina", "SD": "South Dakota",
	"TN": "Tennessee", "TX": "Texas", "UT": "Utah", "VT": "Vermont", "VA": "Virginia", "WA": "Washington",
	"WV": "West Virginia", "WI": "Wisconsin", "WY": "Wyoming", "DC": "District of Columbia",
}

func normalizePlaceName(raw string) string {
	fields := strings.FieldsFunc(strings.ToLower(strings.TrimSpace(raw)), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	return strings.Join(fields, " ")
}

func expandUSStateAbbreviation(place string) string {
	place = strings.TrimSpace(place)
	commaParts := strings.Split(place, ",")
	last := strings.TrimSpace(commaParts[len(commaParts)-1])
	if len(commaParts) > 1 && len(last) == 2 {
		if name, ok := usStateNames[strings.ToUpper(last)]; ok {
			commaParts[len(commaParts)-1] = name
			return strings.Join(commaParts, ", ")
		}
	}
	spaceParts := strings.Fields(place)
	if len(spaceParts) > 1 && len(spaceParts[len(spaceParts)-1]) == 2 {
		if name, ok := usStateNames[strings.ToUpper(spaceParts[len(spaceParts)-1])]; ok {
			spaceParts[len(spaceParts)-1] = name
			return strings.Join(spaceParts, " ")
		}
	}
	return place
}

// resolveMarketplaceLocation asks Facebook's own place typeahead for a unique
// exact match. It deliberately does not guess from fuzzy or partial results.
func (s *Store) resolveMarketplaceLocation(ctx context.Context, place string) (string, error) {
	place = strings.TrimSpace(place)
	if place == "" {
		return "", shop.Errorf(shop.ErrInvalidInput, "marketplace location is required")
	}
	place = expandUSStateAbbreviation(place)

	page, err := s.document(ctx, "/marketplace/")
	if err != nil {
		return "", err
	}
	matches := lsdTokenPattern.FindSubmatch(page)
	if len(matches) != 2 || len(matches[1]) == 0 {
		return "", shop.Errorf(shop.ErrUpstream, "Facebook location picker bootstrap token was not found")
	}
	lsd := string(matches[1])

	variables, err := json.Marshal(map[string]any{
		"params": map[string]any{
			"caller":               "MARKETPLACE",
			"country_filter":       nil,
			"integration_strategy": "STRING_MATCH",
			"page_category":        []string{"CITY", "SUBCITY", "NEIGHBORHOOD"},
			"query":                place,
			"search_type":          "PLACE_TYPEAHEAD",
			"viewer_coordinates":   nil,
		},
	})
	if err != nil {
		return "", shop.Errorf(shop.ErrUpstream, "encode Facebook location query: %v", err)
	}
	form := url.Values{
		"fb_api_req_friendly_name": {locationSearchName},
		"fb_api_caller_class":      {"RelayModern"},
		"server_timestamps":        {"true"},
		"variables":                {string(variables)},
		"doc_id":                   {locationSearchDocID},
		"lsd":                      {lsd},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/graphql/", strings.NewReader(form.Encode()))
	if err != nil {
		return "", shop.Errorf(shop.ErrNetwork, "build Facebook location lookup: %v", err)
	}
	setBrowserHeaders(req)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", baseURL)
	req.Header.Set("Referer", baseURL+"/marketplace/")
	req.Header.Set("X-FB-LSD", lsd)
	req.Header.Set("X-FB-Friendly-Name", locationSearchName)
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	resp, err := s.client.Do(req)
	if err != nil {
		return "", shop.Errorf(shop.ErrNetwork, "request Facebook location lookup: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusTooManyRequests {
		return "", shop.Errorf(shop.ErrRateLimited, "Facebook location lookup was rate limited")
	}
	if resp.StatusCode != http.StatusOK {
		return "", shop.Errorf(shop.ErrUpstream, "Facebook location lookup returned HTTP %d", resp.StatusCode)
	}
	var payload struct {
		Data struct {
			CityStreetSearch struct {
				StreetResults struct {
					Edges []struct {
						Node struct {
							Name string `json:"single_line_address"`
							Page struct {
								ID string `json:"id"`
							} `json:"page"`
						} `json:"node"`
					} `json:"edges"`
				} `json:"street_results"`
			} `json:"city_street_search"`
		} `json:"data"`
		Errors []json.RawMessage `json:"errors"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxDocumentSize)).Decode(&payload); err != nil {
		return "", shop.Errorf(shop.ErrUpstream, "decode Facebook location lookup: %v", err)
	}
	if len(payload.Errors) > 0 {
		return "", shop.Errorf(shop.ErrUpstream, "Facebook location lookup returned an error")
	}

	want := normalizePlaceName(place)
	var exact []struct{ name, id string }
	queryHasQualifier := strings.Contains(place, ",")
	for _, edge := range payload.Data.CityStreetSearch.StreetResults.Edges {
		node := edge.Node
		addressName := normalizePlaceName(node.Name)
		cityName := normalizePlaceName(strings.SplitN(node.Name, ",", 2)[0])
		matches := addressName == want || (!queryHasQualifier && strings.ReplaceAll(cityName, " ", "") == strings.ReplaceAll(want, " ", ""))
		if node.Page.ID == "" || !matches {
			continue
		}
		duplicate := false
		for _, match := range exact {
			if match.id == node.Page.ID {
				duplicate = true
				break
			}
		}
		if !duplicate {
			exact = append(exact, struct{ name, id string }{node.Name, node.Page.ID})
		}
	}
	switch len(exact) {
	case 0:
		return "", shop.Errorf(shop.ErrNotFound, "Facebook did not return an exact location match for %q; specify a city and state or use its Marketplace location URL", place)
	case 1:
		if !locationSegment.MatchString(exact[0].id) {
			return "", shop.Errorf(shop.ErrUpstream, "Facebook returned an invalid location identifier")
		}
		return exact[0].id, nil
	default:
		names := make([]string, 0, len(exact))
		for _, match := range exact {
			names = append(names, fmt.Sprintf("%s (%s)", match.name, match.id))
		}
		return "", shop.Errorf(shop.ErrInvalidInput, "location %q is ambiguous; use a state or region qualifier: %s", place, strings.Join(names, ", "))
	}
}
