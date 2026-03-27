// Package amazon implements the shop.Provider interface for Amazon stores.
package amazon

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/saucesteals/shop"
)

func init() {
	shop.Register(&Provider{})
}

// Verify interface compliance at compile time.
var (
	_ shop.Provider = (*Provider)(nil)
	_ shop.Store    = (*Store)(nil)
	_ shop.Cart     = (*cartImpl)(nil)
)

const providerName = "amazon"

// httpTimeout is the default timeout for all Amazon API requests.
const httpTimeout = 30 * time.Second

// amazonDomain holds metadata for a supported Amazon domain.
type amazonDomain struct {
	Name          string
	Country       string
	Currency      string
	MarketplaceID string
}

// supportedDomains maps Amazon domains to their region metadata.
var supportedDomains = map[string]amazonDomain{
	"amazon.com":    {Name: "Amazon US", Country: "US", Currency: "USD", MarketplaceID: "ATVPDKIKX0DER"},
	"amazon.co.uk":  {Name: "Amazon UK", Country: "GB", Currency: "GBP", MarketplaceID: "A1F83G8C2ARO7P"},
	"amazon.de":     {Name: "Amazon DE", Country: "DE", Currency: "EUR", MarketplaceID: "A1PA6795UKMFR9"},
	"amazon.co.jp":  {Name: "Amazon JP", Country: "JP", Currency: "JPY", MarketplaceID: "A1VC38T7YXB528"},
	"amazon.ca":     {Name: "Amazon CA", Country: "CA", Currency: "CAD", MarketplaceID: "A2EUQ1WTGCTBG2"},
	"amazon.com.au": {Name: "Amazon AU", Country: "AU", Currency: "AUD", MarketplaceID: "A39IBJ37TRP1C6"},
}

// Provider handles Amazon domains (amazon.com, amazon.co.uk, etc.).
type Provider struct{}

// Name returns "amazon".
func (p *Provider) Name() string { return providerName }

// Detect checks if the handle is an Amazon domain and returns the
// appropriate region-specific StoreInfo.
func (p *Provider) Detect(_ context.Context, handle string) (*shop.StoreInfo, error) {
	if handle == "amazon" {
		handle = "amazon.com"
	}

	if info, ok := supportedDomains[handle]; ok {
		return &shop.StoreInfo{
			Name:     info.Name,
			Domain:   handle,
			Provider: providerName,
			Country:  info.Country,
			Currency: info.Currency,
		}, nil
	}

	return nil, nil
}

// DetectCost returns DetectCostFree — Amazon domains are matched by string.
func (p *Provider) DetectCost() shop.DetectCost { return shop.DetectCostFree }

// Store creates an Amazon Store instance. configDir is used for auth file I/O.
func (p *Provider) Store(_ context.Context, handle string, configDir string) (shop.Store, error) {
	info := supportedDomains[handle]

	s := &Store{
		handle:        handle,
		configDir:     configDir,
		client:        &http.Client{Timeout: httpTimeout},
		currency:      info.Currency,
		marketplaceID: info.MarketplaceID,
	}
	s.cart = &cartImpl{store: s}

	return s, nil
}

// Store implements shop.Store for Amazon.
type Store struct {
	handle        string
	configDir     string
	client        *http.Client
	cart          *cartImpl
	currency      string
	marketplaceID string

	mu   sync.Mutex
	tvss *tvssClient
}

// tvssAPI returns the tvssClient, initializing it from auth state on first
// call. Errors if the user is not authenticated. Uses a mutex so transient
// errors are not cached permanently — unlike sync.Once, a failed init can
// be retried on the next call.
func (s *Store) tvssAPI() (*tvssClient, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.tvss != nil {
		return s.tvss, nil
	}

	state, err := s.loadAuth()
	if err != nil {
		return nil, err
	}
	if state == nil || !state.isAuthenticated() {
		return nil, shop.Errorf(shop.ErrAuthRequired, "not authenticated; run 'shop login --store %s' first", s.handle)
	}

	s.tvss = newTVSSClient(s.client, state, s.marketplaceID)

	return s.tvss, nil
}

// Info returns metadata about this Amazon store, using the store's handle
// to look up region-specific information.
func (s *Store) Info() shop.StoreInfo {
	if info, ok := supportedDomains[s.handle]; ok {
		return shop.StoreInfo{
			Name:     info.Name,
			Domain:   s.handle,
			Provider: providerName,
			Country:  info.Country,
			Currency: info.Currency,
		}
	}

	// Fallback for unknown handles.
	return shop.StoreInfo{
		Name:     "Amazon",
		Domain:   s.handle,
		Provider: providerName,
	}
}

// Login, Logout, and WhoAmI are implemented in auth.go.

// Cart returns the cart singleton for this store.
func (s *Store) Cart() shop.Cart { return s.cart }

// Capabilities returns Amazon's supported feature set.
func (s *Store) Capabilities() shop.Capabilities {
	return shop.Capabilities{
		Search:          true,
		Reviews:         true,
		Offers:          true,
		Variants:        true,
		Cart:            true,
		Checkout:        true,
		Addresses:       true,
		PaymentMethods:  true,
		ShippingOptions: true,
	}
}

// productURL builds a public-facing product URL for the store's domain.
func productURL(domain, asin string) string {
	return fmt.Sprintf("https://www.%s/dp/%s", domain, asin)
}
