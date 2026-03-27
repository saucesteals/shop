package amazon

import (
	"context"
	"fmt"
	"maps"
	"net/url"
	"slices"
	"strings"

	"github.com/saucesteals/shop"
)

// Product fetches full details for an ASIN from the TVSS API.
//
// TVSS endpoint: GET /marketplaces/{marketplace}/products/{asin}
func (s *Store) Product(ctx context.Context, productID string) (*shop.Product, error) {
	if err := validateASIN(productID); err != nil {
		return nil, err
	}

	api, err := s.tvssAPI()
	if err != nil {
		return nil, err
	}

	u := api.tvssPath([]string{"products", productID}, nil)

	var tp tvssProduct
	if err := api.doGet(ctx, u, &tp); err != nil {
		return nil, err
	}

	return mapProduct(&tp, s.handle, s.currency), nil
}

// Offers returns the buy-box offer for the product. The TVSS API does not
// expose a multi-seller offers listing — it returns a single merchant per
// product detail call. We return that as a single-offer result.
//
// TVSS endpoint: GET /marketplaces/{marketplace}/products/{asin}
func (s *Store) Offers(ctx context.Context, productID string, _ *shop.OffersQuery) (*shop.OffersResult, error) {
	if err := validateASIN(productID); err != nil {
		return nil, err
	}

	api, err := s.tvssAPI()
	if err != nil {
		return nil, err
	}

	u := api.tvssPath([]string{"products", productID}, nil)

	var tp tvssProduct
	if err := api.doGet(ctx, u, &tp); err != nil {
		return nil, err
	}

	result := &shop.OffersResult{
		Page:    1,
		HasMore: false,
	}

	offer := shop.Offer{
		ID:        tp.OfferID,
		Condition: shop.ConditionNew,
		Price:     toMoney(tp.Price, s.currency),
		Availability: shop.Availability{
			Status: mapAvailabilityStatus(tp.ProductAvailability),
		},
		IsBuyBox: true,
	}

	if tp.MerchantInfo != nil {
		offer.Seller = shop.Seller{
			ID:   tp.MerchantInfo.MerchantID,
			Name: tp.MerchantInfo.MerchantName,
		}
		offer.IsPrime = tp.MerchantInfo.SoldByAmazon
	}

	if tp.ShippingDetails != nil {
		si := &shop.ShippingInfo{
			Description: tp.ShippingDetails.ShippingCost,
		}
		if tp.ShippingDetails.FreeShipping {
			si.Description = "Free Shipping"
		}
		offer.Shipping = si
	}

	result.Offers = append(result.Offers, offer)

	return result, nil
}

// Variants returns the variation tree for a product.
//
// TVSS endpoint: GET /marketplaces/{marketplace}/products/{asin}/variations
func (s *Store) Variants(ctx context.Context, productID string) (*shop.VariantsResult, error) {
	if err := validateASIN(productID); err != nil {
		return nil, err
	}

	api, err := s.tvssAPI()
	if err != nil {
		return nil, err
	}

	params := url.Values{}
	params.Set("page-index", "0")
	params.Set("page-size", "100")
	u := api.tvssPath([]string{"products", productID, "variations"}, params)

	var vr tvssVariationsResponse
	if err := api.doGet(ctx, u, &vr); err != nil {
		return nil, err
	}

	result := &shop.VariantsResult{
		ParentID: productID,
	}

	// Map dimensions.
	for _, d := range vr.Dimensions {
		dim := shop.VariantDimension{
			Name: d.DisplayString,
		}
		for i, val := range d.Values {
			opt := shop.VariantOption{
				Value:     val,
				Available: true,
			}
			if i < len(d.SwatchImageURLs) {
				opt.ImageURL = d.SwatchImageURLs[i]
			}
			dim.Options = append(dim.Options, opt)
		}
		result.Dimensions = append(result.Dimensions, dim)
	}

	currency := s.currency

	// Map combinations.
	for _, v := range vr.Variations {
		combo := shop.VariantCombo{
			ProductID: v.ASIN,
			Available: true,
			Values:    make(map[string]string),
		}
		if v.BuyingPrice != "" {
			p := toMoney(v.BuyingPrice, currency)
			combo.Price = &p
		}
		// Map variation indices to dimension values.
		for i, idx := range v.VariationIndices {
			if i < len(vr.Dimensions) && idx < len(vr.Dimensions[i].Values) {
				combo.Values[vr.Dimensions[i].DisplayString] = vr.Dimensions[i].Values[idx]
			}
		}
		result.Combinations = append(result.Combinations, combo)
	}

	return result, nil
}

// mapProduct converts a TVSS product response to the shop.Product type.
// domain and currency are used for URL generation and price parsing.
func mapProduct(tp *tvssProduct, domain, currency string) *shop.Product {
	p := &shop.Product{
		ID:    tp.ASIN,
		Title: tp.Title,
		Brand: tp.ByLine,
		URL:   productURL(domain, tp.ASIN),
		Availability: shop.Availability{
			Status: mapAvailabilityStatus(tp.ProductAvailability),
		},
	}

	if tp.Price != "" {
		pr := toMoney(tp.Price, currency)
		p.Price = &pr
	}
	if tp.ListPrice != "" {
		lp := toMoney(tp.ListPrice, currency)
		p.ListPrice = &lp
	}

	if tp.CustomerReviewsCount > 0 || tp.AverageOverallRating > 0 {
		p.Rating = &shop.Rating{
			Average: tp.AverageOverallRating,
			Count:   tp.CustomerReviewsCount,
		}
	}

	for _, u := range tp.ProductImageURLs {
		p.Images = append(p.Images, shop.Image{URL: u})
	}

	if tp.MerchantInfo != nil {
		p.Seller = &shop.Seller{
			ID:   tp.MerchantInfo.MerchantID,
			Name: tp.MerchantInfo.MerchantName,
		}
	}

	// Map details to specs — sorted keys for deterministic output.
	detailKeys := slices.Sorted(maps.Keys(tp.Details))
	for _, k := range detailKeys {
		p.Specs = append(p.Specs, shop.Spec{Name: k, Value: tp.Details[k]})
	}

	// Map description to features — sorted keys for deterministic output.
	descKeys := slices.Sorted(maps.Keys(tp.Description))
	for _, k := range descKeys {
		p.Features = append(p.Features, tp.Description[k]...)
	}

	// Variant info from labels.
	if tp.VariationParentASIN != "" || len(tp.VariationLabels) > 0 {
		vi := &shop.VariantInfo{
			ParentID: tp.VariationParentASIN,
			Selected: make(map[string]string),
		}
		for _, vl := range tp.VariationLabels {
			vi.Selected[vl.Label] = vl.Value
		}
		p.VariantInfo = vi
	}

	// Store provider-specific fields in attributes.
	attrs := map[string]any{}
	if tp.OfferID != "" {
		attrs["offerId"] = tp.OfferID
	}
	if tp.ProductGroupID != "" {
		attrs["productGroupId"] = tp.ProductGroupID
	}
	if tp.PrimeExclusive {
		attrs["primeExclusive"] = true
	}
	if tp.VariationPriceRange != "" {
		attrs["variationPriceRange"] = tp.VariationPriceRange
	}
	if len(attrs) > 0 {
		p.Attributes = attrs
	}

	// Build description from description map — sorted for deterministic output.
	var descParts []string
	for _, section := range descKeys {
		lines := tp.Description[section]
		descParts = append(descParts, fmt.Sprintf("%s: %s", section, strings.Join(lines, " ")))
	}
	if len(descParts) > 0 {
		p.Description = strings.Join(descParts, "\n")
	}

	return p
}

// mapAvailabilityStatus converts TVSS availability to shop status.
func mapAvailabilityStatus(a *tvssAvailabilityDetails) shop.AvailabilityStatus {
	if a == nil {
		return shop.AvailabilityInStock
	}

	switch strings.ToUpper(a.Status) {
	case "IN_STOCK", "IN STOCK":
		return shop.AvailabilityInStock
	case "LOW_STOCK":
		return shop.AvailabilityLowStock
	case "OUT_OF_STOCK", "OUT OF STOCK":
		return shop.AvailabilityOutOfStock
	default:
		return shop.AvailabilityInStock
	}
}
