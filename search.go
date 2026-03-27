package shop

// SearchQuery defines what to search for and how to filter/sort results.
type SearchQuery struct {
	Query     string            `json:"query"`
	Page      int               `json:"page,omitempty"`
	PageSize  int               `json:"pageSize,omitempty"`
	Sort      SearchSort        `json:"sort,omitempty"`
	MinPrice  *int64            `json:"minPrice,omitempty"`
	MaxPrice  *int64            `json:"maxPrice,omitempty"`
	MinRating *float64          `json:"minRating,omitempty"`
	Category  string            `json:"category,omitempty"`
	Filters   map[string]string `json:"filters,omitempty"`
}

// SearchSort controls the ordering of search results.
type SearchSort string

const (
	SortRelevance  SearchSort = "relevance"
	SortPriceLow   SearchSort = "price_low"
	SortPriceHigh  SearchSort = "price_high"
	SortRating     SearchSort = "rating"
	SortNewest     SearchSort = "newest"
	SortBestSeller SearchSort = "best_seller"
)

// SearchResult is a paginated list of product summaries.
type SearchResult struct {
	Products []ProductSummary `json:"products"`
	Count    int              `json:"count"`
	Page     int              `json:"page"`
	HasMore  bool             `json:"hasMore"`
	Filters  []SearchFilter   `json:"filters,omitempty"`
	Warnings []string         `json:"warnings,omitempty"`
}

// SearchFilter describes a filterable facet returned by the store (e.g.,
// brand, department, price range).
type SearchFilter struct {
	Name    string               `json:"name"`
	Key     string               `json:"key"`
	Options []SearchFilterOption `json:"options"`
}

// SearchFilterOption is a single selectable value within a SearchFilter.
type SearchFilterOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Count int    `json:"count,omitempty"`
}

// ReviewsQuery controls review pagination and filtering.
type ReviewsQuery struct {
	Page     int        `json:"page,omitempty"`
	PageSize int        `json:"pageSize,omitempty"`
	Sort     ReviewSort `json:"sort,omitempty"`
	Rating   *int       `json:"rating,omitempty"`
}

// ReviewSort controls the ordering of reviews.
type ReviewSort string

const (
	ReviewSortRecent  ReviewSort = "recent"
	ReviewSortHelpful ReviewSort = "helpful"
	ReviewSortRating  ReviewSort = "rating"
)

// ReviewsResult is a paginated list of reviews with aggregate stats.
type ReviewsResult struct {
	Rating  Rating   `json:"rating"`
	Reviews []Review `json:"reviews"`
	Page    int      `json:"page"`
	HasMore bool     `json:"hasMore"`
}

// Review is a single customer review.
type Review struct {
	ID         string         `json:"id"`
	Author     string         `json:"author"`
	Title      string         `json:"title,omitempty"`
	Body       string         `json:"body"`
	Rating     int            `json:"rating"`
	Date       string         `json:"date"`
	Verified   bool           `json:"verified"`
	Helpful    int            `json:"helpful,omitempty"`
	Images     []Image        `json:"images,omitempty"`
	Attributes map[string]any `json:"attributes,omitempty"`
}

// OffersQuery controls offer listing.
type OffersQuery struct {
	Condition OfferCondition `json:"condition,omitempty"`
	Page      int            `json:"page,omitempty"`
	PageSize  int            `json:"pageSize,omitempty"`
}

// OfferCondition filters offers by item condition.
type OfferCondition string

const (
	ConditionAny         OfferCondition = ""
	ConditionNew         OfferCondition = "new"
	ConditionUsedLikeNew OfferCondition = "used_like_new"
	ConditionUsedGood    OfferCondition = "used_good"
	ConditionUsedFair    OfferCondition = "used_fair"
	ConditionRefurbished OfferCondition = "refurbished"
)

// OffersResult is a paginated list of offers for a product.
type OffersResult struct {
	Offers  []Offer `json:"offers"`
	Page    int     `json:"page"`
	HasMore bool    `json:"hasMore"`
}

// Offer represents a single seller's listing for a product.
type Offer struct {
	ID           string         `json:"id"`
	Seller       Seller         `json:"seller"`
	Condition    OfferCondition `json:"condition"`
	Price        Money          `json:"price"`
	Shipping     *ShippingInfo  `json:"shipping,omitempty"`
	Availability Availability   `json:"availability"`
	IsBuyBox     bool           `json:"isBuyBox,omitempty"`
	IsPrime      bool           `json:"isPrime,omitempty"`
	DeliveryDate string         `json:"deliveryDate,omitempty"`
	Attributes   map[string]any `json:"attributes,omitempty"`
}

// ShippingInfo describes shipping cost and speed for an offer.
type ShippingInfo struct {
	Price       *Money `json:"price,omitempty"`
	Description string `json:"description,omitempty"`
	Speed       string `json:"speed,omitempty"`
}
