package facebook

import (
	"context"
	"encoding/json"
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

func TestSearchRejectsUnsafeLocationURLBeforeRequest(t *testing.T) {
	requests := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		return nil, io.EOF
	})}
	store := &Store{client: client}
	_, err := store.Search(context.Background(), &shop.SearchQuery{
		Query:   "left handed golf set",
		Filters: map[string]string{"location": "https://example.com/marketplace/108188925868598/"},
	})
	if err == nil {
		t.Fatal("expected unsafe URL to fail")
	}
	if requests != 0 {
		t.Fatalf("invalid location made %d HTTP requests, want none", requests)
	}
}

func TestSearchResolvesSmallTownWithPickerAndUsesItsID(t *testing.T) {
	var calls int
	var searchPath string
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		var body string
		switch r.URL.Path {
		case "/marketplace/":
			body = `<script>"LSD",[],{"token":"test-lsd"}</script>`
		case "/api/graphql/":
			if r.Method != http.MethodPost {
				t.Fatalf("GraphQL method = %s, want POST", r.Method)
			}
			if got := r.Header.Get("X-FB-LSD"); got != "test-lsd" {
				t.Fatalf("X-FB-LSD = %q, want bootstrap token", got)
			}
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			var variables struct {
				Params struct {
					Query string `json:"query"`
				} `json:"params"`
			}
			if err := json.Unmarshal([]byte(r.Form.Get("variables")), &variables); err != nil {
				t.Fatal(err)
			}
			if variables.Params.Query != "Wayne, New Jersey" {
				t.Fatalf("picker query = %q, want expanded Wayne, New Jersey", variables.Params.Query)
			}
			body = `{"data":{"city_street_search":{"street_results":{"edges":[{"node":{"single_line_address":"Wayne, New Jersey","subtitle":"City","page":{"id":"103723296332578"}}}]}}}}`
		case "/marketplace/103723296332578/search/":
			searchPath = r.URL.Path
			body = `<script type="application/json">{"data":{"marketplace_search":{"feed_units":{"edges":[]}}}}</script>`
		default:
			t.Fatalf("unexpected request path %q", r.URL.Path)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}
	store := &Store{client: client}
	_, err := store.Search(context.Background(), &shop.SearchQuery{
		Query:   "bicycle",
		Filters: map[string]string{"location": "Wayne, NJ", "radius": "20mi"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if searchPath != "/marketplace/103723296332578/search/" {
		t.Fatalf("search request path = %q, want to use resolved Wayne ID", searchPath)
	}
	if calls != 3 {
		t.Fatalf("made %d HTTP requests, want bootstrap + picker + search", calls)
	}
}

func TestResolveMarketplaceLocationRejectsAmbiguousTownName(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body := `<script>"LSD",[],{"token":"test-lsd"}</script>`
		if r.URL.Path == "/api/graphql/" {
			body = `{"data":{"city_street_search":{"street_results":{"edges":[{"node":{"single_line_address":"Wayne, New Jersey","page":{"id":"103723296332578"}}},{"node":{"single_line_address":"Wayne, Pennsylvania","page":{"id":"104"}}}]}}}}`
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}
	_, err := (&Store{client: client}).resolveMarketplaceLocation(context.Background(), "Wayne")
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("error = %v, want ambiguous location error", err)
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
