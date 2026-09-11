package amazon

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/saucesteals/shop"
)

type offerRoundTripper struct {
	pages      map[string]string
	requests   []string
	urls       []string
	cookies    []string
	statusSeq  []int
	statusHits int
}

func (t *offerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	page := req.URL.Query().Get("pageno")
	if page == "" {
		page = "1"
	}
	t.requests = append(t.requests, page)
	t.urls = append(t.urls, req.URL.RawQuery)
	var names []string
	for _, cookie := range req.Cookies() {
		names = append(names, cookie.Name)
	}
	t.cookies = append(t.cookies, strings.Join(names, ","))
	status := http.StatusOK
	if t.statusHits < len(t.statusSeq) {
		status = t.statusSeq[t.statusHits]
		t.statusHits++
	}
	if status != http.StatusOK {
		return &http.Response{
			StatusCode: status,
			Body:       io.NopCloser(strings.NewReader("unavailable")),
			Header:     make(http.Header),
		}, nil
	}
	body, ok := t.pages[page]
	if !ok {
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(strings.NewReader("not found")),
			Header:     make(http.Header),
		}, nil
	}

	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}, nil
}

func TestOffersLogicalPagination(t *testing.T) {
	transport := &offerRoundTripper{
		pages: map[string]string{
			"1": offerPage(1, 10),
			"2": offerPage(11, 13),
		},
	}
	store := testOfferStore(transport)

	result, err := store.Offers(context.Background(), "B0F2BDXW4J", &shop.OffersQuery{
		Page:     2,
		PageSize: 5,
	})
	if err != nil {
		t.Fatalf("Offers() error = %v", err)
	}
	if got, want := offerIDs(result.Offers), "offer-6,offer-7,offer-8,offer-9,offer-10"; got != want {
		t.Fatalf("offer IDs = %q, want %q", got, want)
	}
	if !result.HasMore {
		t.Fatal("HasMore = false, want true")
	}
	if got, want := strings.Join(transport.requests, ","), "1,2"; got != want {
		t.Fatalf("requested pages = %q, want %q", got, want)
	}
}

func TestOffersFinalPartialPage(t *testing.T) {
	transport := &offerRoundTripper{
		pages: map[string]string{
			"1": offerPage(1, 10),
			"2": offerPage(11, 13),
		},
	}
	store := testOfferStore(transport)

	result, err := store.Offers(context.Background(), "B0F2BDXW4J", &shop.OffersQuery{
		Page:     3,
		PageSize: 5,
	})
	if err != nil {
		t.Fatalf("Offers() error = %v", err)
	}
	if got, want := offerIDs(result.Offers), "offer-11,offer-12,offer-13"; got != want {
		t.Fatalf("offer IDs = %q, want %q", got, want)
	}
	if result.HasMore {
		t.Fatal("HasMore = true, want false")
	}
}

func TestOffersRejectsExcessiveOffset(t *testing.T) {
	store := testOfferStore(&offerRoundTripper{})

	_, err := store.Offers(context.Background(), "B0F2BDXW4J", &shop.OffersQuery{
		Page:     11,
		PageSize: 100,
	})
	if err == nil {
		t.Fatal("Offers() error = nil, want invalid input")
	}
}

func TestOffersRejectsOverflowingPage(t *testing.T) {
	store := testOfferStore(&offerRoundTripper{})

	_, err := store.Offers(context.Background(), "B0F2BDXW4J", &shop.OffersQuery{
		Page:     int(^uint(0) >> 1),
		PageSize: 100,
	})
	if err == nil {
		t.Fatal("Offers() error = nil, want invalid input")
	}
}

func TestOffersRejectsOversizedResponse(t *testing.T) {
	transport := &offerRoundTripper{
		pages: map[string]string{
			"1": strings.Repeat("x", aodMaxBodyBytes+1),
		},
	}
	store := testOfferStore(transport)

	_, err := store.Offers(context.Background(), "B0F2BDXW4J", nil)
	if err == nil {
		t.Fatal("Offers() error = nil, want response size error")
	}
}

func TestParseAODOffer(t *testing.T) {
	body := offerPage(1, 1)
	offers, count, err := parseAODOffers([]byte(body), "USD", "ATVPDKIKX0DER")
	if err != nil {
		t.Fatalf("parseAODOffers() error = %v", err)
	}
	if count != 1 || len(offers) != 1 {
		t.Fatalf("parsed count = %d, offers = %d, want 1", count, len(offers))
	}
	offer := offers[0]
	if offer.ID != "offer-1" {
		t.Fatalf("offer ID = %q, want %q", offer.ID, "offer-1")
	}
	if offer.Seller.Name != "Amazon" || offer.Seller.ID != "ATVPDKIKX0DER" {
		t.Fatalf("seller = %#v, want Amazon marketplace seller", offer.Seller)
	}
	if offer.Shipping == nil || offer.Shipping.From != "Amazon" {
		t.Fatalf("shipping = %#v, want Amazon", offer.Shipping)
	}
	if offer.Price.Amount != 1001 {
		t.Fatalf("price = %d, want 1001", offer.Price.Amount)
	}
}

func TestAODURLUsesStableQuery(t *testing.T) {
	got := aodURL("amazon.com", "B0F2BDXW4J", 1)
	if !strings.Contains(got, "isAod=1") || !strings.Contains(got, "experience=aod") || !strings.Contains(got, "asin=B0F2BDXW4J") {
		t.Fatalf("aodURL() = %q", got)
	}
	page2 := aodURL("amazon.com", "B0F2BDXW4J", 2)
	if !strings.Contains(page2, "pageno=2") || !strings.Contains(page2, "isonlyrenderofferlist=true") {
		t.Fatalf("paged aodURL() = %q", page2)
	}
}

func TestOffersRetriesUnavailableThenSucceeds(t *testing.T) {
	transport := &offerRoundTripper{
		pages:     map[string]string{"1": offerPage(1, 1)},
		statusSeq: []int{http.StatusServiceUnavailable, http.StatusOK},
	}
	store := testOfferStore(transport)
	result, err := store.Offers(context.Background(), "B0F2BDXW4J", &shop.OffersQuery{PageSize: 1})
	if err != nil {
		t.Fatalf("Offers() error = %v", err)
	}
	if len(result.Offers) != 1 {
		t.Fatalf("len(offers) = %d, want 1", len(result.Offers))
	}
	if len(transport.urls) != 2 {
		t.Fatalf("attempts = %d, want 2", len(transport.urls))
	}
	if !strings.Contains(transport.urls[0], "isAod=1") || !strings.Contains(transport.urls[0], "experience=aod") {
		t.Fatalf("query = %q", transport.urls[0])
	}
}

func TestOffersSendsSessionCookiesWhenAuthenticated(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "auth"), 0o700); err != nil {
		t.Fatal(err)
	}
	auth := []byte(`{"state":"authenticated","device":{},"cookies":[{"name":"at-main","value":"token"},{"name":"ubid-main","value":"ubid"}]}`)
	if err := os.WriteFile(filepath.Join(dir, "auth", "amazon.com.json"), auth, 0o600); err != nil {
		t.Fatal(err)
	}
	transport := &offerRoundTripper{pages: map[string]string{"1": offerPage(1, 1)}}
	store := testOfferStore(transport)
	store.configDir = dir
	if _, err := store.Offers(context.Background(), "B0F2BDXW4J", &shop.OffersQuery{PageSize: 1}); err != nil {
		t.Fatalf("Offers() error = %v", err)
	}
	if got, want := transport.cookies[0], "at-main,ubid-main"; got != want {
		t.Fatalf("cookies = %q, want %q", got, want)
	}
}

func testOfferStore(transport http.RoundTripper) *Store {
	return &Store{
		handle: "amazon.com",
		client: &amazonClient{http: &http.Client{
			Transport: transport,
		}},
		currency:      "USD",
		marketplaceID: "ATVPDKIKX0DER",
	}
}

func offerPage(first, last int) string {
	var body strings.Builder
	body.WriteString("<html><body>")
	for i := first; i <= last; i++ {
		fmt.Fprintf(&body, `<div class="aod-other-offer-block">
<div id="aod-offer-heading">New</div>
<div id="aod-offer-soldBy">Sold by Amazon.com</div>
<div id="aod-offer-shipsFrom">Ships from Amazon</div>
<span>$10.%02d</span>
<span data-action="aw-aod-cart-api" data-aw-aod-cart-api="{&quot;oid&quot;:&quot;offer-%d&quot;}"></span>
</div>`, i, i)
	}
	body.WriteString("</body></html>")

	return body.String()
}

func offerIDs(offers []shop.Offer) string {
	ids := make([]string, len(offers))
	for i, offer := range offers {
		ids[i] = offer.ID
	}

	return strings.Join(ids, ",")
}
