package facebook

import (
	"context"
	"encoding/json"
	"math"
	"strings"

	"github.com/saucesteals/shop"
)

type coordinate struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type optionalCoordinate struct {
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
}

func (c coordinate) valid() bool {
	return c.Latitude >= -90 && c.Latitude <= 90 && c.Longitude >= -180 && c.Longitude <= 180
}

func (c optionalCoordinate) coordinate() (coordinate, bool) {
	if c.Latitude == nil || c.Longitude == nil {
		return coordinate{}, false
	}
	point := coordinate{
		Latitude:  *c.Latitude,
		Longitude: *c.Longitude,
	}

	return point, point.valid()
}

type marketplacePlace struct {
	City        string
	State       string
	CityPageID  string
	DisplayName string
}

type geocodeVariables struct {
	Params geocodeParams `json:"params"`
}

type geocodeParams struct {
	Caller              string      `json:"caller"`
	CountryFilter       *string     `json:"country_filter"`
	IntegrationStrategy string      `json:"integration_strategy"`
	PageCategory        []string    `json:"page_category"`
	Query               string      `json:"query"`
	SearchType          string      `json:"search_type"`
	ViewerCoordinates   *coordinate `json:"viewer_coordinates"`
}

func (l *listing) place() marketplacePlace {
	reverse := l.Location.ReverseGeocode

	return marketplacePlace{
		City:        reverse.City,
		State:       reverse.State,
		CityPageID:  reverse.CityPage.ID,
		DisplayName: reverse.CityPage.DisplayName,
	}
}

func (p marketplacePlace) key() string {
	if p.CityPageID != "" {
		return p.CityPageID
	}

	return strings.ToLower(strings.TrimSpace(p.City + "," + p.State))
}

func (p marketplacePlace) query() string {
	if p.State == "" {
		return p.City
	}

	return p.City + ", " + p.State
}

func (s *Store) placeCoordinate(ctx context.Context, docID string, place marketplacePlace) (coordinate, error) {
	key := place.key()
	if key == "" {
		return coordinate{}, shop.Errorf(shop.ErrNotFound, "marketplace listing location is missing")
	}
	s.cacheMu.RLock()
	if point, ok := s.places[key]; ok {
		s.cacheMu.RUnlock()

		return point, nil
	}
	s.cacheMu.RUnlock()

	point, err := s.fetchPlaceCoordinate(ctx, docID, place)
	if err != nil {
		return coordinate{}, err
	}
	s.cacheMu.Lock()
	s.places[key] = point
	s.cacheMu.Unlock()

	return point, nil
}

func (s *Store) fetchPlaceCoordinate(ctx context.Context, docID string, place marketplacePlace) (coordinate, error) {
	if docID == "" {
		return coordinate{}, shop.Errorf(shop.ErrUpstream, "marketplace location operation missing")
	}
	variables, err := json.Marshal(geocodeVariables{
		Params: geocodeParams{
			Caller:              "MARKETPLACE",
			IntegrationStrategy: "STRING_MATCH",
			PageCategory:        []string{"CITY", "SUBCITY", "NEIGHBORHOOD", "POSTAL_CODE"},
			Query:               place.query(),
			SearchType:          "PLACE_TYPEAHEAD",
		},
	})
	if err != nil {
		return coordinate{}, shop.Errorf(shop.ErrInternal, "encode Marketplace location query: %v", err)
	}
	raw, err := s.graphQL(ctx, docID, "MarketplaceSearchAddressDataSourceQuery", variables)
	if err != nil {
		return coordinate{}, err
	}
	var response struct {
		Data struct {
			Search struct {
				Results struct {
					Edges []struct {
						Node struct {
							Address string `json:"single_line_address"`
							Page    struct {
								ID string `json:"id"`
							} `json:"page"`
							Location optionalCoordinate `json:"location"`
						} `json:"node"`
					} `json:"edges"`
				} `json:"street_results"`
			} `json:"city_street_search"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return coordinate{}, shop.Errorf(shop.ErrUpstream, "decode Marketplace location: %v", err)
	}
	edges := response.Data.Search.Results.Edges
	for _, edge := range edges {
		point, ok := edge.Node.Location.coordinate()
		if place.CityPageID != "" && edge.Node.Page.ID == place.CityPageID && ok {
			return point, nil
		}
	}
	wanted := strings.ToLower(strings.TrimSpace(place.DisplayName))
	if wanted == "" {
		wanted = strings.ToLower(strings.TrimSpace(place.query()))
	}
	for _, edge := range edges {
		point, ok := edge.Node.Location.coordinate()
		if strings.EqualFold(strings.TrimSpace(edge.Node.Address), wanted) && ok {
			return point, nil
		}
	}

	return coordinate{}, shop.Errorf(shop.ErrNotFound, "resolve Marketplace location %q", place.query())
}

func distanceKm(a, b coordinate) float64 {
	const earthRadiusKm = 6371.0088
	lat1 := a.Latitude * math.Pi / 180
	lat2 := b.Latitude * math.Pi / 180
	dLat := (b.Latitude - a.Latitude) * math.Pi / 180
	dLng := (b.Longitude - a.Longitude) * math.Pi / 180
	h := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1)*math.Cos(lat2)*math.Sin(dLng/2)*math.Sin(dLng/2)

	return 2 * earthRadiusKm * math.Asin(math.Min(1, math.Sqrt(h)))
}
