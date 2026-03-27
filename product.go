package shop

// Money represents a monetary value in minor units (cents for USD, pence for
// GBP, etc.). Never a float.
type Money struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

// Product is the full representation of a product.
type Product struct {
	ID           string         `json:"id"`
	Title        string         `json:"title"`
	Brand        string         `json:"brand,omitempty"`
	Description  string         `json:"description,omitempty"`
	URL          string         `json:"url"`
	Images       []Image        `json:"images"`
	Price        *Money         `json:"price,omitempty"`
	ListPrice    *Money         `json:"listPrice,omitempty"`
	Rating       *Rating        `json:"rating,omitempty"`
	Availability Availability   `json:"availability"`
	Seller       *Seller        `json:"seller,omitempty"`
	Categories   []string       `json:"categories,omitempty"`
	Features     []string       `json:"features,omitempty"`
	Specs        []Spec         `json:"specs,omitempty"`
	VariantInfo  *VariantInfo   `json:"variantInfo,omitempty"`
	Attributes   map[string]any `json:"attributes,omitempty"`
}

// ProductSummary is the condensed representation returned in search results.
type ProductSummary struct {
	ID           string         `json:"id"`
	Title        string         `json:"title"`
	Brand        string         `json:"brand,omitempty"`
	URL          string         `json:"url"`
	ImageURL     string         `json:"imageUrl,omitempty"`
	Price        *Money         `json:"price,omitempty"`
	ListPrice    *Money         `json:"listPrice,omitempty"`
	Rating       *Rating        `json:"rating,omitempty"`
	Availability Availability   `json:"availability"`
	Sponsored    bool           `json:"sponsored,omitempty"`
	Badge        string         `json:"badge,omitempty"`
	Attributes   map[string]any `json:"attributes,omitempty"`
}

// Image is a product image with optional dimensions.
type Image struct {
	URL    string `json:"url"`
	Alt    string `json:"alt,omitempty"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
}

// Rating is the aggregate customer rating for a product.
type Rating struct {
	Average float64        `json:"average"`
	Count   int            `json:"count"`
	Stars   *StarBreakdown `json:"stars,omitempty"`
}

// StarBreakdown shows the distribution of ratings by star level.
// Values are percentages (0-100), not counts.
type StarBreakdown struct {
	Five  float64 `json:"five"`
	Four  float64 `json:"four"`
	Three float64 `json:"three"`
	Two   float64 `json:"two"`
	One   float64 `json:"one"`
}

// Availability represents whether a product can be purchased.
type Availability struct {
	Status  AvailabilityStatus `json:"status"`
	Message string             `json:"message,omitempty"`
}

// AvailabilityStatus is the purchase-readiness of a product.
type AvailabilityStatus string

const (
	AvailabilityInStock     AvailabilityStatus = "in_stock"
	AvailabilityLowStock    AvailabilityStatus = "low_stock"
	AvailabilityOutOfStock  AvailabilityStatus = "out_of_stock"
	AvailabilityPreorder    AvailabilityStatus = "preorder"
	AvailabilityUnavailable AvailabilityStatus = "unavailable"
)

// Seller represents the merchant selling a product.
type Seller struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name"`
	URL  string `json:"url,omitempty"`
}

// Spec is a single key-value specification (e.g., "Weight" → "2.5 lbs").
type Spec struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// VariantInfo describes which variant this product is within its family.
// Use shop variants to get the full dimension/combination tree.
type VariantInfo struct {
	ParentID string            `json:"parentId,omitempty"`
	Selected map[string]string `json:"selected,omitempty"` // e.g. {"Color": "Black"}
}

// VariantDimension is a single axis of variation (e.g., "Color", "Size").
type VariantDimension struct {
	Name    string          `json:"name"`
	Options []VariantOption `json:"options"`
}

// VariantOption is a single value within a dimension.
type VariantOption struct {
	Value     string `json:"value"`
	ProductID string `json:"productId,omitempty"`
	Available bool   `json:"available"`
	ImageURL  string `json:"imageUrl,omitempty"`
}

// VariantsResult is the full variant tree returned by Store.Variants().
type VariantsResult struct {
	ParentID     string             `json:"parentId"`
	Dimensions   []VariantDimension `json:"dimensions"`
	Combinations []VariantCombo     `json:"combinations"`
	Truncated    bool               `json:"truncated,omitempty"`
}

// VariantCombo maps a specific set of dimension values to a product ID.
type VariantCombo struct {
	Values    map[string]string `json:"values"`
	ProductID string            `json:"productId"`
	Price     *Money            `json:"price,omitempty"`
	Available bool              `json:"available"`
}
