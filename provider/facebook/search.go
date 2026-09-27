package facebook

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/saucesteals/shop"
)

const (
	defaultRadiusKm           = 65
	maxRadiusKm               = 500
	searchQueryName           = "CometMarketplaceSearchContentContainerQuery"
	searchContentComponent    = "CometMarketplaceSearchContentContainer.react"
	searchPaginationOperation = "CometMarketplaceSearchContentPaginationQuery_facebookRelayOperation"
	searchPaginationQuery     = "CometMarketplaceSearchContentPaginationQuery"
	locationDialogComponent   = "MarketplaceBuyLocationDialog.react"
	locationSearchOperation   = "MarketplaceSearchAddressDataSourceQuery_facebookRelayOperation"
)

var radiusValue = regexp.MustCompile(`^([0-9]+(?:\.[0-9]+)?)(mi|km)?$`)

type searchPage struct {
	DocID     string
	Variables json.RawMessage
	Center    coordinate
	Feed      searchFeed
}

type searchFeed struct {
	Edges    []searchEdge `json:"edges"`
	PageInfo pageInfo     `json:"page_info"`
}

type searchEdge struct {
	Node struct {
		Listing *listing `json:"listing"`
	} `json:"node"`
}

type pageInfo struct {
	EndCursor   string `json:"end_cursor"`
	HasNextPage bool   `json:"has_next_page"`
}

func marketplaceCity(raw string) (string, error) {
	slug := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) {
			return unicode.ToLower(r)
		}

		return -1
	}, raw)
	if slug == "" || len(slug) > 40 {
		return "", shop.Errorf(shop.ErrInvalidInput, "marketplace city must be a Facebook city slug such as austin or nyc")
	}

	return slug, nil
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

func searchCityRadius(filters map[string]string) (string, int, error) {
	if filters == nil || strings.TrimSpace(filters["city"]) == "" {
		return "", 0, shop.Errorf(shop.ErrInvalidInput, "marketplace search requires --filter city=<city>")
	}
	city, err := marketplaceCity(filters["city"])
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

	return city, radiusKm, nil
}

// Search returns one native Marketplace relevance page for a city and radius.
func (s *Store) Search(ctx context.Context, query *shop.SearchQuery) (*shop.SearchResult, error) {
	if query == nil || strings.TrimSpace(query.Query) == "" {
		return nil, shop.Errorf(shop.ErrInvalidInput, "marketplace search query is required")
	}
	if query.Page < 0 || query.PageSize < 0 {
		return nil, shop.Errorf(shop.ErrInvalidInput, "page and page size must not be negative")
	}
	if query.PageSize > 100 {
		return nil, shop.Errorf(shop.ErrInvalidInput, "marketplace page size must be between 1 and 100")
	}
	if (query.Sort != "" && query.Sort != shop.SortRelevance) || query.MinRating != nil || query.Category != "" {
		return nil, shop.Errorf(shop.ErrNotSupported, "marketplace search supports relevance results without rating or category filters")
	}
	for key := range query.Filters {
		switch key {
		case "city", "radius", "radius_unit":
		default:
			return nil, shop.Errorf(shop.ErrNotSupported, "unsupported marketplace filter %q", key)
		}
	}
	city, radiusKm, err := searchCityRadius(query.Filters)
	if err != nil {
		return nil, err
	}
	page := query.Page
	if page < 1 {
		page = 1
	}
	if page > 100 {
		return nil, shop.Errorf(shop.ErrInvalidInput, "marketplace page must be between 1 and 100")
	}
	params := url.Values{
		"query":  {query.Query},
		"radius": {strconv.Itoa(radiusKm)},
	}
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
	if query.MinPrice != nil && query.MaxPrice != nil && *query.MinPrice > *query.MaxPrice {
		return nil, shop.Errorf(shop.ErrInvalidInput, "min price must not exceed max price")
	}
	body, err := s.document(ctx, "/marketplace/"+city+"/search/?"+params.Encode())
	if err != nil {
		return nil, err
	}
	search, err := parseSearchPage(body)
	if err != nil {
		return nil, err
	}
	feed := search.Feed
	paginationDocID := ""
	if page > 1 {
		paginationDocID, err = s.operationID(ctx, body, searchContentComponent, searchPaginationOperation)
		if err != nil {
			return nil, err
		}
	}
	seenBefore := make(map[string]bool)
	seenCursors := make(map[string]bool)
	for current := 1; current < page; current++ {
		if !feed.PageInfo.HasNextPage || feed.PageInfo.EndCursor == "" {
			return &shop.SearchResult{
				Products: []shop.ProductSummary{},
				Page:     page,
				Warnings: distanceWarnings(),
			}, nil
		}
		if seenCursors[feed.PageInfo.EndCursor] {
			return nil, shop.Errorf(shop.ErrUpstream, "marketplace repeated its pagination cursor")
		}
		seenCursors[feed.PageInfo.EndCursor] = true
		for _, edge := range feed.Edges {
			if edge.Node.Listing != nil {
				seenBefore[edge.Node.Listing.ID] = true
			}
		}
		variables, err := searchVariables(search.Variables, feed.PageInfo.EndCursor)
		if err != nil {
			return nil, err
		}
		raw, err := s.graphQL(ctx, paginationDocID, searchPaginationQuery, variables)
		if err != nil {
			return nil, err
		}
		feed, err = parseSearchResponse(raw)
		if err != nil {
			return nil, err
		}
	}
	locationDocID := ""
	if len(feed.Edges) > 0 {
		locationDocID, err = s.operationID(ctx, body, locationDialogComponent, locationSearchOperation)
		if err != nil {
			return nil, err
		}
	}
	products, outside, unverifiable, malformed, err := s.productsInRadius(ctx, locationDocID, feed.Edges, seenBefore, search.Center, radiusKm)
	if err != nil {
		return nil, err
	}
	result := &shop.SearchResult{
		Products: products,
		Page:     page,
		HasMore:  feed.PageInfo.HasNextPage,
		Warnings: distanceWarnings(),
	}
	if outside > 0 {
		result.Warnings = append(result.Warnings, countWarning(outside, "listing outside the requested city-center radius was removed", "listings outside the requested city-center radius were removed"))
	}
	if unverifiable > 0 {
		result.Warnings = append(result.Warnings, countWarning(unverifiable, "listing without a verifiable Marketplace city was removed", "listings without a verifiable Marketplace city were removed"))
	}
	if malformed > 0 {
		result.Warnings = append(result.Warnings, countWarning(malformed, "incomplete Marketplace listing was skipped", "incomplete Marketplace listings were skipped"))
	}
	if query.PageSize > 0 && query.PageSize < len(result.Products) {
		result.Products = result.Products[:query.PageSize]
		result.Warnings = append(result.Warnings, "page-size caps this native Marketplace page locally; hasMore still reflects Facebook's cursor")
	}
	result.Count = len(result.Products)

	return result, nil
}

func countWarning(count int, singular, plural string) string {
	message := plural
	if count == 1 {
		message = singular
	}

	return fmt.Sprintf("%d %s.", count, message)
}

func distanceWarnings() []string {
	return []string{
		"Search listing currencies may be omitted by Facebook; USD is assumed.",
		"Radius filtering is approximate because search results expose Marketplace city centers, not exact seller coordinates.",
	}
}

func parseSearchPage(body []byte) (searchPage, error) {
	var page searchPage
	err := jsonObjects(body, func(raw json.RawMessage) error {
		if page.DocID != "" {
			return nil
		}
		var preloader struct {
			QueryName string          `json:"queryName"`
			QueryID   string          `json:"queryID"`
			Variables json.RawMessage `json:"variables"`
		}
		if err := json.Unmarshal(raw, &preloader); err != nil {
			return shop.Errorf(shop.ErrUpstream, "decode Marketplace preloader: %v", err)
		}
		if preloader.QueryName == searchQueryName && preloader.QueryID != "" && len(preloader.Variables) > 0 {
			page.DocID = preloader.QueryID
			page.Variables = append(json.RawMessage(nil), preloader.Variables...)
		}

		return nil
	})
	if err != nil {
		return searchPage{}, err
	}
	if page.DocID == "" || len(page.Variables) == 0 {
		return searchPage{}, shop.Errorf(shop.ErrUpstream, "marketplace search preloader missing")
	}
	var variables struct {
		BuyLocation *optionalCoordinate `json:"buyLocation"`
	}
	if err := json.Unmarshal(page.Variables, &variables); err != nil {
		return searchPage{}, shop.Errorf(shop.ErrUpstream, "decode Marketplace search variables: %v", err)
	}
	if variables.BuyLocation == nil {
		return searchPage{}, shop.Errorf(shop.ErrUpstream, "marketplace search location missing")
	}
	center, ok := variables.BuyLocation.coordinate()
	if !ok {
		return searchPage{}, shop.Errorf(shop.ErrUpstream, "marketplace search location is invalid")
	}
	page.Center = center
	found := false
	err = relayData(body, func(raw json.RawMessage) error {
		if found {
			return nil
		}
		feed, ok, err := parseSearchData(raw)
		if err != nil {
			return err
		}
		if ok {
			page.Feed = feed
			found = true
		}

		return nil
	})
	if err != nil {
		return searchPage{}, err
	}
	if !found {
		return searchPage{}, shop.Errorf(shop.ErrUpstream, "marketplace search Relay payload missing")
	}

	return page, nil
}

func parseSearchResponse(raw json.RawMessage) (searchFeed, error) {
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return searchFeed{}, shop.Errorf(shop.ErrUpstream, "decode Marketplace search response: %v", err)
	}
	feed, ok, err := parseSearchData(envelope.Data)
	if err != nil {
		return searchFeed{}, err
	}
	if !ok {
		return searchFeed{}, shop.Errorf(shop.ErrUpstream, "marketplace search response missing")
	}

	return feed, nil
}

func parseSearchData(raw json.RawMessage) (searchFeed, bool, error) {
	var data struct {
		Search *struct {
			Feed *searchFeed `json:"feed_units"`
		} `json:"marketplace_search"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return searchFeed{}, false, shop.Errorf(shop.ErrUpstream, "decode Marketplace search: %v", err)
	}
	if data.Search == nil || data.Search.Feed == nil {
		return searchFeed{}, false, nil
	}

	return *data.Search.Feed, true, nil
}

func searchVariables(raw json.RawMessage, cursor string) (json.RawMessage, error) {
	var source map[string]json.RawMessage
	if err := json.Unmarshal(raw, &source); err != nil {
		return nil, shop.Errorf(shop.ErrUpstream, "decode Marketplace pagination variables: %v", err)
	}
	variables := make(map[string]json.RawMessage)
	for _, key := range []string{"count", "params", "scale"} {
		value := source[key]
		if len(value) == 0 {
			return nil, shop.Errorf(shop.ErrUpstream, "marketplace pagination variable %s missing", key)
		}
		variables[key] = value
	}
	for key, value := range source {
		if strings.HasPrefix(key, "__relay_internal__pv__") {
			variables[key] = value
		}
	}
	encodedCursor, err := json.Marshal(cursor)
	if err != nil {
		return nil, shop.Errorf(shop.ErrInternal, "encode Marketplace cursor: %v", err)
	}
	variables["cursor"] = encodedCursor
	encoded, err := json.Marshal(variables)
	if err != nil {
		return nil, shop.Errorf(shop.ErrInternal, "encode Marketplace pagination variables: %v", err)
	}

	return encoded, nil
}

func (s *Store) productsInRadius(ctx context.Context, locationDocID string, edges []searchEdge, excluded map[string]bool, center coordinate, radiusKm int) ([]shop.ProductSummary, int, int, int, error) {
	products := make([]shop.ProductSummary, 0, len(edges))
	seen := make(map[string]bool)
	outside := 0
	unverifiable := 0
	malformed := 0
	for _, edge := range edges {
		if edge.Node.Listing == nil {
			continue
		}
		if excluded[edge.Node.Listing.ID] {
			continue
		}
		product, err := edge.Node.Listing.product(true)
		if err != nil {
			malformed++
			continue
		}
		if seen[product.ID] {
			continue
		}
		point, err := s.placeCoordinate(ctx, locationDocID, edge.Node.Listing.place())
		if err != nil {
			if shop.IsNotFound(err) {
				unverifiable++
				continue
			}

			return nil, 0, 0, 0, err
		}
		distance := distanceKm(center, point)
		if distance > float64(radiusKm) {
			outside++
			continue
		}
		seen[product.ID] = true
		if product.Attributes == nil {
			product.Attributes = make(map[string]any)
		}
		product.Attributes["distanceKm"] = math.Round(distance*10) / 10
		products = append(products, shop.ProductSummary{
			ID:           product.ID,
			Title:        product.Title,
			URL:          product.URL,
			ImageURL:     product.Images[0].URL,
			Price:        product.Price,
			Availability: product.Availability,
			Attributes:   product.Attributes,
		})
	}

	return products, outside, unverifiable, malformed, nil
}
