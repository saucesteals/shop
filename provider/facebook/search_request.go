package facebook

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/saucesteals/shop"
)

// Persisted web operations verified against the shipped client on 2026-11-02.
// These are versioned protocol constants, not per-session state.
const searchDocumentID = "28698615193166509"

func buildSearchPage(query *shop.SearchQuery) (searchPage, error) {
	lat, hasLat := query.Filters["latitude"]
	lon, hasLon := query.Filters["longitude"]
	if !hasLat || !hasLon {
		return searchPage{}, shop.Errorf(shop.ErrInvalidInput, "marketplace search requires latitude and longitude")
	}
	latitude, errLat := strconv.ParseFloat(strings.TrimSpace(lat), 64)
	longitude, errLon := strconv.ParseFloat(strings.TrimSpace(lon), 64)
	center := coordinate{Latitude: latitude, Longitude: longitude}
	if errLat != nil || errLon != nil || !center.valid() {
		return searchPage{}, shop.Errorf(shop.ErrInvalidInput, "invalid marketplace coordinates")
	}
	variables := directSearchVariables{Count: 24, Scale: 1, BuyLocation: center, SavedSearchQuery: query.Query, IncludePopular: true}
	variables.Params.BQF.Callsite = "COMMERCE_MKTPLACE_SEO_USERS"
	variables.Params.BQF.Query = query.Query
	variables.Params.Browse = json.RawMessage(`{"commerce_search_and_rp_available":true,"commerce_search_and_rp_category_id":[],"filter_price_lower_bound":0,"filter_price_upper_bound":214748364700}`)
	variables.Params.Custom = json.RawMessage(`{"surface":"SEARCH","search_vertical":"C2C","contextual_filters":[],"virtual_contextual_filters":[],"serp_landing_settings":{"virtual_category_id":""}}`)
	raw, err := json.Marshal(variables)
	if err != nil {
		return searchPage{}, shop.Errorf(shop.ErrInternal, "encode marketplace search: %v", err)
	}
	return searchPage{DocID: searchDocumentID, Variables: raw, Center: center}, nil
}

type directSearchVariables struct {
	ContextualData  *string `json:"contextual_data"`
	SavedSearchID   *string `json:"savedSearchID"`
	TopicPageParams struct {
		LocationID *string `json:"location_id"`
		URL        *string `json:"url"`
	} `json:"topicPageParams"`
	BuyLocation      coordinate `json:"buyLocation"`
	Count            int        `json:"count"`
	Cursor           *string    `json:"cursor"`
	Scale            int        `json:"scale"`
	SavedSearchQuery string     `json:"savedSearchQuery"`
	IncludePopular   bool       `json:"shouldIncludePopularSearches"`
	DeferNonCritical bool       `json:"shouldDeferNonCritical"`
	SponsoredField   bool       `json:"__relay_internal__pv__GHLShouldChangeMarketplaceSponsoredDataFieldNamerelayprovider"`
	Params           struct {
		BQF struct {
			Callsite string `json:"callsite"`
			Query    string `json:"query"`
		} `json:"bqf"`
		Browse json.RawMessage `json:"browse_request_params"`
		Custom json.RawMessage `json:"custom_request_params"`
	} `json:"params"`
}
