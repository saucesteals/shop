package facebook

import (
	"context"
	"encoding/json"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/saucesteals/shop"
)

const (
	defaultRadiusKm = 65
	maxRadiusKm     = 500
)

var radiusValue = regexp.MustCompile(`^([0-9]+(?:\.[0-9]+)?)(mi|km)?$`)
var locationSegment = regexp.MustCompile(`^[A-Za-z0-9]{1,40}$`)
var reservedMarketplaceRoutes = map[string]struct{}{
	"category": {}, "create": {}, "item": {}, "profile": {}, "search": {}, "you": {},
}

func marketplaceCity(raw string) (string, error) {
	slug := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}

		return -1
	}, raw)
	if slug == "" || len(slug) > 40 {
		return "", shop.Errorf(shop.ErrInvalidInput, "marketplace city must be a Facebook city slug such as austin or nyc")
	}

	return slug, nil
}

// marketplaceLocation accepts either a canonical Facebook Marketplace location
// segment (a city slug or numeric place ID) or a Marketplace URL containing one.
// Facebook's location picker uses numeric IDs for many cities and neighborhoods.
func marketplaceLocation(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", shop.Errorf(shop.ErrInvalidInput, "marketplace location is required")
	}

	if strings.Contains(value, "://") {
		u, err := url.Parse(value)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.Port() != "" || (u.Hostname() != "facebook.com" && u.Hostname() != "www.facebook.com") {
			return "", shop.Errorf(shop.ErrInvalidInput, "location URL must be a Facebook Marketplace URL")
		}
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) < 2 || len(parts) > 3 || !strings.EqualFold(parts[0], "marketplace") {
			return "", shop.Errorf(shop.ErrInvalidInput, "location URL must include a Marketplace city slug or location ID")
		}
		if _, reserved := reservedMarketplaceRoutes[strings.ToLower(parts[1])]; reserved {
			return "", shop.Errorf(shop.ErrInvalidInput, "location URL must include a Marketplace city slug or location ID")
		}
		if len(parts) == 3 && !strings.EqualFold(parts[2], "search") {
			return "", shop.Errorf(shop.ErrInvalidInput, "location URL must be a Marketplace location or search URL")
		}
		value = parts[1]
	}

	if !locationSegment.MatchString(value) {
		return "", shop.Errorf(shop.ErrInvalidInput, "marketplace location must be a canonical Facebook city slug, numeric location ID, or Marketplace URL")
	}

	return strings.ToLower(value), nil
}

func parseRadiusKm(raw, unit string) (int, error) {
	match := radiusValue.FindStringSubmatch(strings.ReplaceAll(raw, " ", ""))
	if match == nil {
		return 0, shop.Errorf(shop.ErrInvalidInput, "radius must be a number with optional mi or km suffix")
	}
	value, err := strconv.ParseFloat(match[1], 64)
	if err != nil || value <= 0 {
		return 0, shop.Errorf(shop.ErrInvalidInput, "radius must be greater than zero")
	}
	suffix := match[2]
	if suffix == "" {
		suffix = unit
	} else if unit != "" && unit != suffix {
		return 0, shop.Errorf(shop.ErrInvalidInput, "conflicting radius units")
	}
	if suffix == "" {
		suffix = "mi"
	}
	km := value
	switch suffix {
	case "km":
	case "mi":
		km = value * 1.60934
	default:
		return 0, shop.Errorf(shop.ErrInvalidInput, "radius_unit must be mi or km")
	}
	rounded := int(km + 0.5)
	if rounded < 1 {
		rounded = 1
	}
	if rounded > maxRadiusKm {
		return 0, shop.Errorf(shop.ErrInvalidInput, "radius exceeds %d km", maxRadiusKm)
	}

	return rounded, nil
}

func searchLocationRadius(filters map[string]string) (string, int, error) {
	if filters == nil || (strings.TrimSpace(filters["city"]) == "" && strings.TrimSpace(filters["location"]) == "") {
		return "", 0, shop.Errorf(shop.ErrInvalidInput, "marketplace search requires --filter location=<Facebook location ID, city slug, or Marketplace URL>")
	}
	var location string
	var err error
	if strings.TrimSpace(filters["location"]) != "" {
		if strings.TrimSpace(filters["city"]) != "" {
			return "", 0, shop.Errorf(shop.ErrInvalidInput, "use only one of the city and location filters")
		}
		location, err = marketplaceLocation(filters["location"])
	} else {
		location, err = marketplaceCity(filters["city"])
	}
	if err != nil {
		return "", 0, err
	}
	radiusKm := defaultRadiusKm
	if raw := strings.ToLower(strings.TrimSpace(filters["radius"])); raw != "" {
		radiusKm, err = parseRadiusKm(raw, strings.ToLower(strings.TrimSpace(filters["radius_unit"])))
		if err != nil {
			return "", 0, err
		}
	} else if strings.TrimSpace(filters["radius_unit"]) != "" {
		return "", 0, shop.Errorf(shop.ErrInvalidInput, "radius_unit requires radius")
	}

	return location, radiusKm, nil
}

// Search returns the first relevance page for a Marketplace city and radius.
func (s *Store) Search(ctx context.Context, query *shop.SearchQuery) (*shop.SearchResult, error) {
	if query == nil || strings.TrimSpace(query.Query) == "" {
		return nil, shop.Errorf(shop.ErrInvalidInput, "marketplace search query is required")
	}
	if query.Page < 0 || query.PageSize < 0 {
		return nil, shop.Errorf(shop.ErrInvalidInput, "page and page size must not be negative")
	}
	if query.Page > 1 || (query.Sort != "" && query.Sort != shop.SortRelevance) || query.MinRating != nil || query.Category != "" {
		return nil, shop.Errorf(shop.ErrNotSupported, "marketplace search supports first-page relevance results only")
	}
	for key := range query.Filters {
		switch key {
		case "city", "location", "radius", "radius_unit":
		default:
			return nil, shop.Errorf(shop.ErrNotSupported, "unsupported marketplace filter %q", key)
		}
	}
	location, radiusKm, err := searchLocationRadius(query.Filters)
	if err != nil {
		return nil, err
	}
	params := url.Values{"query": {query.Query}, "radius": {strconv.Itoa(radiusKm)}}
	if query.MinPrice != nil {
		if *query.MinPrice < 0 {
			return nil, shop.Errorf(shop.ErrInvalidInput, "min price must not be negative")
		}
		params.Set("minPrice", strconv.FormatInt(*query.MinPrice/100, 10))
	}
	if query.MaxPrice != nil {
		if *query.MaxPrice < 0 {
			return nil, shop.Errorf(shop.ErrInvalidInput, "max price must not be negative")
		}
		params.Set("maxPrice", strconv.FormatInt(*query.MaxPrice/100, 10))
	}
	body, err := s.document(ctx, "/marketplace/"+location+"/search/?"+params.Encode())
	if err != nil {
		return nil, err
	}
	result := &shop.SearchResult{
		Products: []shop.ProductSummary{},
		Page:     1,
		Warnings: []string{
			"Search listing currencies may be omitted by Facebook; USD is assumed.",
			"Pagination is not supported; only the first page is returned.",
		},
	}
	found := false
	hasEdges := false
	seen := make(map[string]bool)
	err = relayData(body, func(raw json.RawMessage) error {
		var data struct {
			Search *struct {
				Feed *struct {
					Edges []struct {
						Node struct {
							Listing *listing `json:"listing"`
						} `json:"node"`
					} `json:"edges"`
				} `json:"feed_units"`
			} `json:"marketplace_search"`
		}
		if err := json.Unmarshal(raw, &data); err != nil {
			return shop.Errorf(shop.ErrUpstream, "decode marketplace search: %v", err)
		}
		if data.Search == nil || data.Search.Feed == nil {
			return nil
		}
		found = true
		if data.Search.Feed.Edges == nil {
			return shop.Errorf(shop.ErrUpstream, "missing marketplace search edges")
		}
		hasEdges = hasEdges || len(data.Search.Feed.Edges) > 0
		for _, edge := range data.Search.Feed.Edges {
			if edge.Node.Listing == nil {
				continue
			}
			product, err := edge.Node.Listing.product(true)
			if err != nil {
				return err
			}
			if seen[product.ID] {
				continue
			}
			seen[product.ID] = true
			result.Products = append(result.Products, shop.ProductSummary{
				ID:           product.ID,
				Title:        product.Title,
				URL:          product.URL,
				ImageURL:     product.Images[0].URL,
				Price:        product.Price,
				Availability: product.Availability,
				Attributes:   product.Attributes,
			})
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, shop.Errorf(shop.ErrUpstream, "marketplace search Relay payload missing")
	}
	if hasEdges && len(result.Products) == 0 {
		return nil, shop.Errorf(shop.ErrUpstream, "marketplace feed contains no readable listings")
	}
	if query.PageSize > 0 && query.PageSize < len(result.Products) {
		result.Products = result.Products[:query.PageSize]
		result.HasMore = true
		result.Warnings = append(result.Warnings, "page-size truncates this first page locally")
	}
	result.Count = len(result.Products)

	return result, nil
}
