package facebook

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"github.com/saucesteals/shop"
)

const (
	providerName = "facebook"
	baseURL      = "https://www.facebook.com"
	httpTimeout  = 30 * time.Second
)

func init() { shop.Register(&Provider{}) }

var (
	_ shop.Provider = (*Provider)(nil)
	_ shop.Store    = (*Store)(nil)
	_ shop.Cart     = (*unsupportedCart)(nil)
)

// Provider handles Facebook Marketplace.
type Provider struct{}

// Name returns "facebook".
func (p *Provider) Name() string { return providerName }

// DetectCost returns DetectCostFree — Facebook handles are matched by string.
func (p *Provider) DetectCost() shop.DetectCost { return shop.DetectCostFree }

// Detect recognizes the facebook handle and facebook.com URLs.
func (p *Provider) Detect(_ context.Context, handle string) (*shop.StoreInfo, error) {
	if !facebookHandle(handle) {
		return nil, nil
	}
	info := storeInfo()

	return &info, nil
}

func facebookHandle(handle string) bool {
	handle = strings.ToLower(strings.TrimSpace(handle))
	if handle == "facebook" {
		return true
	}
	if !strings.Contains(handle, "://") {
		handle = "https://" + handle
	}
	u, err := url.Parse(handle)

	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.User == nil && u.Port() == "" && (u.Hostname() == "facebook.com" || u.Hostname() == "www.facebook.com")
}

// Store creates an anonymous Marketplace store.
func (p *Provider) Store(_ context.Context, handle, _ string) (shop.Store, error) {
	if !facebookHandle(handle) {
		return nil, shop.Errorf(shop.ErrStoreNotFound, "unsupported Facebook handle %q", handle)
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	return &Store{client: &http.Client{
		Transport:     http.DefaultTransport.(*http.Transport).Clone(),
		Timeout:       httpTimeout,
		Jar:           jar,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}, nil
}

// Store implements public Marketplace document lookups.
type Store struct{ client *http.Client }

func storeInfo() shop.StoreInfo {
	return shop.StoreInfo{
		Name:     "Facebook Marketplace",
		Domain:   "facebook.com",
		Provider: providerName,
		Country:  "US",
		Currency: "USD",
	}
}

// Info describes the Marketplace store.
func (s *Store) Info() shop.StoreInfo { return storeInfo() }

// Capabilities declares search support.
func (s *Store) Capabilities() shop.Capabilities { return shop.Capabilities{Search: true} }

// WhoAmI reports an anonymous session.
func (s *Store) WhoAmI(context.Context) (*shop.AccountInfo, error) {
	return &shop.AccountInfo{Authenticated: false}, nil
}
