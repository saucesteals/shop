package facebook

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/saucesteals/shop"
)

var listingID = regexp.MustCompile(`^[0-9]+$`)

type listing struct {
	ID           string `json:"id"`
	Title        string `json:"marketplace_listing_title"`
	CreationTime int64  `json:"creation_time"`
	Price        struct {
		Amount   string `json:"amount_with_offset_in_currency"`
		Currency string `json:"currency"`
	} `json:"listing_price"`
	PrimaryPhoto photo   `json:"primary_listing_photo"`
	Photos       []photo `json:"listing_photos"`
	Description  struct {
		Text string `json:"text"`
	} `json:"redacted_description"`
	LocationText struct {
		Text string `json:"text"`
	} `json:"location_text"`
	Location struct {
		ReverseGeocode struct {
			City     string `json:"city"`
			State    string `json:"state"`
			CityPage struct {
				ID          string `json:"id"`
				DisplayName string `json:"display_name"`
			} `json:"city_page"`
		} `json:"reverse_geocode"`
	} `json:"location"`
	Live    *bool `json:"is_live"`
	Pending *bool `json:"is_pending"`
	Sold    *bool `json:"is_sold"`
	Hidden  *bool `json:"is_hidden"`
}

type photo struct {
	Image struct {
		URI    string `json:"uri"`
		Width  int    `json:"width"`
		Height int    `json:"height"`
	} `json:"image"`
}

func productID(value string) (string, error) {
	if listingID.MatchString(value) {
		return value, nil
	}
	u, err := url.Parse(value)
	if err == nil && facebookHandle(value) {
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) == 3 && parts[0] == "marketplace" && parts[1] == "item" && listingID.MatchString(parts[2]) {
			return parts[2], nil
		}
	}

	return "", shop.Errorf(shop.ErrInvalidInput, "expected a numeric Marketplace listing ID or item URL")
}

func (l *listing) product(assumeUSD bool) (*shop.Product, error) {
	amount, err := strconv.ParseInt(l.Price.Amount, 10, 64)
	if err != nil || amount < 0 || !listingID.MatchString(l.ID) || strings.TrimSpace(l.Title) == "" {
		return nil, shop.Errorf(shop.ErrUpstream, "incomplete marketplace listing %q", l.ID)
	}
	currency := l.Price.Currency
	if currency == "" && assumeUSD {
		currency = "USD"
	}
	if len(currency) != 3 || strings.Trim(currency, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") != "" {
		return nil, shop.Errorf(shop.ErrUpstream, "missing or invalid currency for marketplace listing %s", l.ID)
	}
	location := l.LocationText.Text
	if location == "" {
		parts := []string{}
		if l.Location.ReverseGeocode.City != "" {
			parts = append(parts, l.Location.ReverseGeocode.City)
		}
		if l.Location.ReverseGeocode.State != "" {
			parts = append(parts, l.Location.ReverseGeocode.State)
		}
		location = strings.Join(parts, ", ")
	}
	var attributes map[string]any
	if location != "" {
		attributes = map[string]any{"location": location}
	}
	if l.CreationTime > 0 {
		if attributes == nil {
			attributes = make(map[string]any)
		}
		attributes["creationTime"] = l.CreationTime
	}
	availability := shop.Availability{Status: shop.AvailabilityUnavailable}
	switch {
	case l.Sold != nil && *l.Sold:
		availability.Status = shop.AvailabilityOutOfStock
		availability.Message = "sold"
	case l.Hidden != nil && *l.Hidden:
		availability.Message = "hidden"
	case l.Pending != nil && *l.Pending:
		availability.Message = "pending"
	case l.Live != nil && *l.Live:
		availability.Status = shop.AvailabilityInStock
	default:
		availability.Message = "availability not confirmed"
	}
	images := []shop.Image{}
	seen := make(map[string]bool)
	for _, p := range append([]photo{l.PrimaryPhoto}, l.Photos...) {
		if p.Image.URI == "" || seen[p.Image.URI] {
			continue
		}
		seen[p.Image.URI] = true
		images = append(images, shop.Image{
			URL:    p.Image.URI,
			Width:  p.Image.Width,
			Height: p.Image.Height,
		})
	}
	if len(images) == 0 {
		return nil, shop.Errorf(shop.ErrUpstream, "missing image for marketplace listing %s", l.ID)
	}

	return &shop.Product{
		ID:          l.ID,
		Title:       l.Title,
		Description: l.Description.Text,
		URL:         baseURL + "/marketplace/item/" + l.ID + "/",
		Images:      images,
		Price: &shop.Money{
			Amount:   amount,
			Currency: currency,
		},
		Availability: availability,
		Attributes:   attributes,
	}, nil
}
