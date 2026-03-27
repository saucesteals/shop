package amazon

import "encoding/json"

// tvss.go defines typed structs mirroring the TVSS API JSON payloads.
// Derived from static analysis of the ShopTV APK's smali bytecode.

// --- Product ---

// tvssProduct is the full product response from GET /products/{asin}.
type tvssProduct struct {
	ASIN                     string                    `json:"asin"`
	Title                    string                    `json:"title"`
	ByLine                   string                    `json:"byLine"`
	Price                    flexString                    `json:"price"`
	ListPrice                flexString                    `json:"listPrice"`
	PricePerUnit             *tvssPricePerUnit         `json:"pricePerUnit,omitempty"`
	ShippingDetails          *tvssShippingDetails      `json:"shippingDetails,omitempty"`
	Badge                    *tvssBadge                `json:"badge,omitempty"`
	CustomerReviewsCount     int                       `json:"customerReviewsCount"`
	AverageOverallRating     float64                   `json:"averageOverallRating"`
	ProductAvailability      *tvssAvailabilityDetails  `json:"productAvailabilityDetails,omitempty"`
	OfferID                  string                    `json:"offerId"`
	ProductGroupID           string                    `json:"productGroupId"`
	VariationParentASIN      string                    `json:"variationParentAsin"`
	VariationPriceRange      flexString                    `json:"variationPriceRange"`
	ProductImageURLs         []string                  `json:"productImageUrls"`
	ProductVideos            []tvssProductVideo        `json:"productVideos"`
	VariationLabels          []tvssVariationLabel      `json:"variationLabels"`
	MerchantInfo             *tvssMerchantInfo         `json:"merchantInfo,omitempty"`
	Details                  map[string]string         `json:"details"`
	Description              map[string][]string       `json:"description"`
	PrimeExclusive           bool                      `json:"primeExclusive"`
	MediaRating              string                    `json:"mediaRating"`
}

type tvssPricePerUnit struct {
	UnitPrice flexString `json:"unitPrice"`
	BaseValue string `json:"baseValue"`
	BaseUnit  string `json:"baseUnit"`
}

type tvssShippingDetails struct {
	FreeShipping bool   `json:"freeShipping"`
	ShippingCost string `json:"shippingCost"`
}

type tvssBadge struct {
	Type     string `json:"type"`
	ImageURL string `json:"imageUrl"`
}

type tvssAvailabilityDetails struct {
	AvailabilityCondition string `json:"availabilityCondition"`
	Status                string `json:"status"`
	PrimaryMessage        string `json:"primaryMessage"`
	SecondaryMessage      string `json:"secondaryMessage"`
}

type tvssProductVideo struct {
	ImageURL string `json:"imageUrl"`
	VideoURL string `json:"videoUrl"`
}

type tvssVariationLabel struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type tvssMerchantInfo struct {
	MerchantID         string `json:"merchantId"`
	MerchantName       string `json:"merchantName"`
	ShipsFromAndSoldBy string `json:"shipsFromAndSoldBy"`
	SoldByAmazon       bool   `json:"soldByAmazon"`
}

// --- Basic Products (bulk lookup + search enrichment) ---

type tvssBasicProductEntity struct {
	BasicProduct *tvssBasicProduct `json:"basicProduct"`
	BasicOffer   *tvssBasicOffer   `json:"basicOffer"`
}

type tvssBasicProduct struct {
	ASIN                 string   `json:"asin"`
	Title                string   `json:"title"`
	ImageURL             string   `json:"imageUrl"`
	ListPrice            flexString   `json:"listPrice"`
	AverageOverallRating float64  `json:"averageOverallRating"`
	CustomerReviewsCount int      `json:"customerReviewsCount"`
	ProductImageURLs     []string `json:"productImageUrls"`
	VariationPriceRange  flexString   `json:"variationPriceRange"`
}

type tvssBasicOffer struct {
	Price flexString     `json:"price"`
	Badge *tvssBadge `json:"badge,omitempty"`
}

// --- Basic Products (bulk lookup) ---

// tvssBasicProductsResponse is the response from GET /basicproducts/{asins}.
type tvssBasicProductsResponse struct {
	Products []tvssBasicProductEntity `json:"products"`
}

// --- Reviews ---

// tvssReviewsResponse is the response from GET /products/{asin}/customer-reviews.
type tvssReviewsResponse struct {
	OneStarCount       int                    `json:"oneStarCount"`
	TwoStarCount       int                    `json:"twoStarCount"`
	ThreeStarCount     int                    `json:"threeStarCount"`
	FourStarCount      int                    `json:"fourStarCount"`
	FiveStarCount      int                    `json:"fiveStarCount"`
	ProductStarRatings *tvssProductStarRatings `json:"productStarRatings"`
	Reviews            []tvssReview           `json:"reviews"`
	PageNext           string                 `json:"pageNext"`
	UsingPageNext      bool                   `json:"usingPageNext"`
}

type tvssProductStarRatings struct {
	Count                int        `json:"count"`
	OverallAverageRating jsonNumber `json:"overallAverageRating"`
	FiveStarPercent      jsonNumber `json:"fiveStarPercent"`
	FourStarPercent      jsonNumber `json:"fourStarPercent"`
	ThreeStarPercent     jsonNumber `json:"threeStarPercent"`
	TwoStarPercent       jsonNumber `json:"twoStarPercent"`
	OneStarPercent       jsonNumber `json:"oneStarPercent"`
}

type tvssReview struct {
	AuthorName         string             `json:"authorName"`
	Title              string             `json:"title"`
	Text               string             `json:"text"`
	OverallRating      *float64           `json:"overallRating"`
	SubmissionDate     string             `json:"submissionDate"` // epoch millis or date string
	IsVerifiedPurchase *bool              `json:"isVerifiedPurchase"`
	IsVine             *bool              `json:"isVine"`
	ImageURLs          []tvssReviewImage  `json:"imageUrls"`
	OriginDescription  string             `json:"originDescription"`
}

type tvssReviewImage struct {
	SmallImageURL  string `json:"smallImageURL"`
	MediumImageURL string `json:"mediumImageURL"`
	LargeImageURL  string `json:"largeImageURL"`
}

// --- Variations ---

// tvssVariationsResponse is the response from GET /products/{asin}/variations.
type tvssVariationsResponse struct {
	Dimensions []tvssVariationDimension `json:"dimensions"`
	Variations []tvssVariation          `json:"variations"`
}

type tvssVariationDimension struct {
	DimensionKey   string   `json:"dimensionKey"`
	DisplayString  string   `json:"displayString"`
	Values         []string `json:"values"`
	SwatchImageURLs []string `json:"swatchImageUrls"`
}

type tvssVariation struct {
	ASIN             string     `json:"asin"`
	BuyingPrice      flexString     `json:"buyingPrice"`
	Badge            *tvssBadge `json:"badge,omitempty"`
	VariationIndices []int      `json:"variationIndices"`
}

// --- Cart ---

// tvssAddToCartRequest is the PUT body for /cart/items.
type tvssAddToCartRequest struct {
	Items []tvssAddToCartItem `json:"items"`
}

type tvssAddToCartItem struct {
	ASIN     string `json:"asin"`
	OfferID  string `json:"offerId,omitempty"`
	Quantity int    `json:"quantity"`
}

// tvssAddToCartResponse is the response from PUT /cart/items.
type tvssAddToCartResponse struct {
	Messages              []tvssCartMessage `json:"messages"`
	ShoppingCartItemCount int               `json:"shoppingCartItemCount"`
}

type tvssCartMessage struct {
	ASIN    string `json:"asin"`
	ItemID  string `json:"itemId"`
	Message string `json:"message"`
}

// tvssCartResponse is the response from GET /cart/items.
type tvssCartResponse struct {
	Items    []tvssCartItem    `json:"items"`
	Messages []tvssCartMessage `json:"messages"`
	Subtotal flexString            `json:"subtotal"`
}

type tvssCartItem struct {
	ASIN             string     `json:"asin"`
	ItemID           string     `json:"itemId"`
	OfferID          string     `json:"offerId"`
	Title            string     `json:"title"`
	ByLine           string     `json:"byLine"`
	Price            flexString     `json:"price"`
	ImageURL         string     `json:"imageUrl"`
	Quantity         int        `json:"quantity"`
	QuantityEditable bool       `json:"quantityEditable"`
	SavedItem        bool       `json:"savedItem"`
	NotAvailableOnTV bool       `json:"notAvailableOnTV"`
	Badge            *tvssBadge `json:"badge,omitempty"`
}

// tvssModifyCartRequest is the PATCH body for /cart/items.
type tvssModifyCartRequest struct {
	Items []tvssModifyCartItem `json:"items"`
}

type tvssModifyCartItem struct {
	ID          string `json:"id"`
	Quantity    int    `json:"quantity"`
	IsSavedItem bool   `json:"isSavedItem"`
}

// --- Checkout ---

// tvssPurchaseInitiateRequest is the POST body for /checkout/purchase/initiate.
type tvssPurchaseInitiateRequest struct {
	Items []tvssPurchaseInitiateItem `json:"items"`
}

type tvssPurchaseInitiateItem struct {
	ASIN     string `json:"asin"`
	OfferID  string `json:"offerId,omitempty"`
	DealID   string `json:"dealId,omitempty"`
	Quantity int    `json:"quantity"`
}

// tvssPurchaseResponse is the response from POST /checkout/purchase/initiate
// and also the base response for checkout state queries.
type tvssPurchaseResponse struct {
	PurchaseID           string              `json:"purchaseId"`
	Destinations         *tvssDestinations   `json:"destinations,omitempty"`
	PaymentMethods       *tvssPaymentMethods `json:"paymentMethods,omitempty"`
	LineItems            *tvssLineItems      `json:"lineItems,omitempty"`
	PurchaseTotals       *tvssPurchaseTotals `json:"purchaseTotals,omitempty"`
	DeliveryGroups       *tvssDeliveryGroups `json:"deliveryGroups,omitempty"`
	PurchaseRestrictions []tvssPurchaseRestriction `json:"purchaseRestrictions,omitempty"`
	PurchaseState        []string            `json:"purchaseState,omitempty"`
}

type tvssDestinations struct {
	Destinations     []tvssDestination `json:"destinations"`
	DestinationList  []tvssDestinationDetail `json:"destinationList"`
}

func (d *tvssDestinations) UnmarshalJSON(data []byte) error {
	type alias tvssDestinations
	return json.Unmarshal(unwrapEntity(data), (*alias)(d))
}

type tvssDestination struct {
	ID            string `json:"id"`
	AddressID     string `json:"addressId"`
	Name          string `json:"name"`
	City          string `json:"city"`
	DisplayString string `json:"displayString"`
}

// tvssDestinationDetail is the full address from the destinationList array.
type tvssDestinationDetail struct {
	Address *tvssDetailAddress `json:"address,omitempty"`
}

func (d *tvssDestinationDetail) UnmarshalJSON(data []byte) error {
	type alias tvssDestinationDetail
	return json.Unmarshal(unwrapEntity(data), (*alias)(d))
}

type tvssDetailAddress struct {
	Name    string              `json:"name"`
	Display *tvssAddressDisplay `json:"display,omitempty"`
}

type tvssAddressDisplay struct {
	SingleLine string   `json:"singleLine"`
	MultiLine  []string `json:"multiLine"`
	Compact    string   `json:"compact"`
}

type tvssPaymentMethods struct {
	CreditOrDebitCard *tvssCreditOrDebitCard `json:"creditOrDebitCard,omitempty"`
	GiftCardAndPromo  *tvssGiftCard         `json:"giftCardAndPromo,omitempty"`
	BankAccount       *tvssBankAccount      `json:"bankAccount,omitempty"`
}

func (p *tvssPaymentMethods) UnmarshalJSON(data []byte) error {
	type alias tvssPaymentMethods
	return json.Unmarshal(unwrapEntity(data), (*alias)(p))
}

type tvssCreditOrDebitCard struct {
	ID         string                 `json:"id"`
	EndingIn   string                 `json:"endingIn"`
	Issuer     flexString             `json:"issuer"`
	Expiry     *tvssCardExpiry        `json:"expiry,omitempty"`
	NameOnCard string                 `json:"nameOnCard"`
}

type tvssCardExpiry struct {
	Month int `json:"month"`
	Year  int `json:"year"`
}

type tvssGiftCard struct {
	Balance *tvssCurrency `json:"balance,omitempty"`
}

type tvssBankAccount struct {
	EndingIn string `json:"endingIn"`
}

type tvssLineItems struct {
	LineItems []tvssLineItem `json:"lineItems"`
}

func (l *tvssLineItems) UnmarshalJSON(data []byte) error {
	type alias tvssLineItems
	return json.Unmarshal(unwrapEntity(data), (*alias)(l))
}

type tvssLineItem struct {
	ASIN          string         `json:"asin"`
	OfferID       string         `json:"offerId"`
	Title         string         `json:"title"`
	ByLine        string         `json:"byLine"`
	Price         flexString         `json:"price"`
	ImageURL      string         `json:"imageUrl"`
	Quantity      *int           `json:"quantity,omitempty"`
	LineItemPrice *tvssLineItemPrice `json:"lineItemPrice,omitempty"`
}

type tvssLineItemPrice struct {
	PriceToPay *tvssCurrency `json:"priceToPay,omitempty"`
	Discount   *tvssDiscount `json:"discount,omitempty"`
}

type tvssCurrency struct {
	Amount        string `json:"amount"`
	DisplayString string `json:"displayString"`
	CurrencyCode  string `json:"currencyCode"`
}

type tvssDiscount struct {
	Amount        string `json:"amount"`
	DisplayString string `json:"displayString"`
}

type tvssPurchaseTotals struct {
	PurchaseTotal     *tvssPurchaseTotal    `json:"purchaseTotal,omitempty"`
	Subtotals         []tvssPurchaseSubtotal `json:"subtotals,omitempty"`
}

func (p *tvssPurchaseTotals) UnmarshalJSON(data []byte) error {
	type alias tvssPurchaseTotals
	return json.Unmarshal(unwrapEntity(data), (*alias)(p))
}

type tvssPurchaseTotal struct {
	Amount        *tvssCurrency `json:"amount,omitempty"`
	DisplayString string        `json:"displayString"`
}

type tvssPurchaseSubtotal struct {
	Type          string        `json:"type"`
	Amount        *tvssCurrency `json:"amount,omitempty"`
	DisplayString string        `json:"displayString"`
	GroupIndex    *int          `json:"groupIndex,omitempty"`
}

type tvssDeliveryGroups struct {
	DeliveryGroups []tvssDeliveryGroup `json:"deliveryGroups"`
}

func (d *tvssDeliveryGroups) UnmarshalJSON(data []byte) error {
	type alias tvssDeliveryGroups
	return json.Unmarshal(unwrapEntity(data), (*alias)(d))
}

type tvssDeliveryGroup struct {
	ID      string               `json:"id"`
	Options []tvssDeliveryOption `json:"deliveryOptions"`
}

type tvssDeliveryOption struct {
	ID           string `json:"id"`
	DisplayString string `json:"displayString"`
	DisplayName  string `json:"displayName"`
	DisplayPrice string `json:"displayPrice"`
	Selected     bool   `json:"selected"`
}

type tvssPurchaseRestriction struct {
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

// tvssAvailablePaymentMethods is the response from GET checkout/purchases/{id}/payment-methods/available.
type tvssAvailablePaymentMethods struct {
	CreditOrDebitCards []tvssCreditOrDebitCard `json:"creditOrDebitCards"`
	BankAccounts       []tvssBankAccount       `json:"bankAccounts"`
	GiftCardAndPromo   *tvssGiftCard           `json:"giftCardAndPromo,omitempty"`
}

// purchaseSignRequest is the POST body for the purchase sign endpoint.
// Mirrors the APK's CheckoutPurchaseSignRequest.
type purchaseSignRequest struct {
	CartItems []purchaseSignItem `json:"cartItems"`
}

// purchaseSignItem is a single item in the sign request.
// The APK always sends quantity=0.
type purchaseSignItem struct {
	ID       string `json:"id"`
	ASIN     string `json:"asin"`
	OfferID  string `json:"offerId"`
	Quantity int    `json:"quantity"`
}

// tvssCheckoutOrdersResponse is the response from GET checkout/purchases/{id}/orders.
type tvssCheckoutOrdersResponse struct {
	Orders []tvssCheckoutOrder `json:"orders"`
}

func (r *tvssCheckoutOrdersResponse) UnmarshalJSON(data []byte) error {
	type alias tvssCheckoutOrdersResponse
	return json.Unmarshal(unwrapEntity(data), (*alias)(r))
}

type tvssCheckoutOrder struct {
	ID         string                    `json:"id"`
	ItemGroups []tvssCheckoutOrderItemGroup `json:"itemGroups"`
}

type tvssCheckoutOrderItemGroup struct {
	Items []tvssCheckoutOrderItem `json:"items"`
}

type tvssCheckoutOrderItem struct {
	Quantity int `json:"quantity"`
}


