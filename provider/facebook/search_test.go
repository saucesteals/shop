package facebook

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/saucesteals/shop"
)

func TestMarketplaceCityPreservesNumericLocationID(t *testing.T) {
	const id = "108188925868598"
	got, err := marketplaceCity(id)
	if err != nil {
		t.Fatalf("marketplaceCity(%q): %v", id, err)
	}
	if got != id {
		t.Fatalf("marketplaceCity(%q) = %q, want %q", id, got, id)
	}
}

func TestMarketplaceLocationAcceptsSlugsIDsAndMarketplaceURLs(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "slug", in: "NYC", want: "nyc"},
		{name: "New Brunswick location ID", in: "108188925868598", want: "108188925868598"},
		{name: "generic numeric location ID", in: "108056275889020", want: "108056275889020"},
		{name: "Marketplace search URL", in: "https://www.facebook.com/marketplace/108188925868598/search/?query=golf&radius=16", want: "108188925868598"},
		{name: "Marketplace location URL", in: "https://facebook.com/marketplace/sanfrancisco/?radius_in_km=65", want: "sanfrancisco"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := marketplaceLocation(tt.in)
			if err != nil {
				t.Fatalf("marketplaceLocation(%q): %v", tt.in, err)
			}
			if got != tt.want {
				t.Fatalf("marketplaceLocation(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestMarketplaceLocationRejectsFreeformAndUnsafeURLs(t *testing.T) {
	for _, in := range []string{
		"New Brunswick, New Jersey",
		"https://example.com/marketplace/108188925868598/",
		"https://facebook.com:8443/marketplace/108188925868598/",
		"https://user@facebook.com/marketplace/108188925868598/",
		"https://facebook.com/marketplace/search/?query=golf",
		"https://facebook.com/marketplace/category/search/",
		"https://facebook.com/marketplace/item/123/",
		"https://facebook.com/marketplace/108188925868598/create/",
		"nyc/search",
		"",
	} {
		t.Run(in, func(t *testing.T) {
			if _, err := marketplaceLocation(in); err == nil {
				t.Fatalf("marketplaceLocation(%q) unexpectedly succeeded", in)
			}
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSearchUsesNumericLocationIDAndRadius(t *testing.T) {
	var request *http.Request
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		request = r
		body := `<html><script type="application/json">{"data":{"marketplace_search":{"feed_units":{"edges":[]}}}}</script></html>`
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    r,
		}, nil
	})}
	store := &Store{client: client}
	_, err := store.Search(context.Background(), &shop.SearchQuery{
		Query:   "left handed golf set",
		Filters: map[string]string{"location": "108188925868598", "radius": "10mi"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if request == nil {
		t.Fatal("search did not make an HTTP request")
	}
	if got, want := request.URL.Path, "/marketplace/108188925868598/search/"; got != want {
		t.Fatalf("request path = %q, want %q", got, want)
	}
	if got, want := request.URL.Query().Get("radius"), "16"; got != want {
		t.Fatalf("request radius = %q km, want %q km", got, want)
	}
	if got, want := request.URL.Query().Get("query"), "left handed golf set"; got != want {
		t.Fatalf("request query = %q, want %q", got, want)
	}
}

func TestSearchRejectsInvalidLocationBeforeRequest(t *testing.T) {
	requests := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		return nil, io.EOF
	})}
	store := &Store{client: client}
	_, err := store.Search(context.Background(), &shop.SearchQuery{
		Query:   "left handed golf set",
		Filters: map[string]string{"location": "New Brunswick, New Jersey"},
	})
	if err == nil {
		t.Fatal("expected invalid free-form location to fail")
	}
	if requests != 0 {
		t.Fatalf("invalid location made %d HTTP requests, want none", requests)
	}
}

func TestSearchLocationRadiusAcceptsGenericLocationIdentifier(t *testing.T) {
	location, radiusKm, err := searchLocationRadius(map[string]string{
		"location": "https://www.facebook.com/marketplace/108188925868598/",
		"radius":   "10mi",
	})
	if err != nil {
		t.Fatal(err)
	}
	if location != "108188925868598" {
		t.Fatalf("location = %q, want numeric Facebook location ID", location)
	}
	if radiusKm != 16 {
		t.Fatalf("radius = %d km, want 16 km", radiusKm)
	}
}

func TestSearchLocationRadiusRejectsConflictingFilters(t *testing.T) {
	if _, _, err := searchLocationRadius(map[string]string{"city": "nyc", "location": "108188925868598"}); err == nil {
		t.Fatal("expected city and location together to fail")
	}
}
