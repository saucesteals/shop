package amazon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/saucesteals/shop"
	"github.com/saucesteals/shop/internal/config"
)

const (
	// Auth state constants.
	stateNone          = ""
	statePending       = "pending"
	stateAuthenticated = "authenticated"
)

// authState is the persistent auth file format. It can represent either a
// pending code pair challenge or a completed authentication. The Device
// struct is preserved in full across both states so that TVSS API calls and
// token refresh flows have access to all registration fields.
type authState struct {
	State  string `json:"state"`
	Device device `json:"device"`

	// Pending fields — present when State is "pending".
	PrivateCode string `json:"privateCode,omitempty"`
	PublicCode  string `json:"publicCode,omitempty"`
	ExpiresAt   string `json:"expiresAt,omitempty"`

	// Authenticated fields — present when State is "authenticated".
	CustomerID        string       `json:"customerId,omitempty"`
	Cookies           []authCookie `json:"cookies,omitempty"`
	CookiesExpiry     string       `json:"cookiesExpiry,omitempty"`
	RefreshToken      string   `json:"refreshToken,omitempty"`
	BearerToken       string   `json:"bearerToken,omitempty"`
	BearerTokenExpiry string   `json:"bearerTokenExpiry,omitempty"`
	AuthenticatedAt   string   `json:"authenticatedAt,omitempty"`
}

// authCookie is the JSON-serialized form of a single Amazon session cookie.
type authCookie struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// httpCookies converts stored auth cookies to standard []*http.Cookie.
// Amazon returns cookie values containing double quotes, which are invalid
// per RFC 6265 and cause Go's net/http to log warnings. Strip them here.
func (a *authState) httpCookies() []*http.Cookie {
	out := make([]*http.Cookie, len(a.Cookies))
	for i, c := range a.Cookies {
		out[i] = &http.Cookie{
			Name:  c.Name,
			Value: strings.ReplaceAll(c.Value, `"`, ""),
		}
	}

	return out
}

// cookieValue returns the value of the named cookie, or empty string.
func (a *authState) cookieValue(name string) string {
	for _, c := range a.Cookies {
		if c.Name == name {
			return c.Value
		}
	}

	return ""
}

// isPending reports whether the auth state is a pending code pair challenge.
func (a *authState) isPending() bool {
	return a.State == statePending
}

// isAuthenticated reports whether the auth state represents completed auth.
func (a *authState) isAuthenticated() bool {
	return a.State == stateAuthenticated
}

// isExpired reports whether the pending code pair has expired.
func (a *authState) isExpired() bool {
	if a.ExpiresAt == "" {
		return true
	}

	exp, err := time.Parse(time.RFC3339, a.ExpiresAt)
	if err != nil {
		return true
	}

	return time.Now().After(exp)
}

// challengeURL returns the domain-specific authorization URL. The code is
// prefilled via the cbl-code query parameter so the user just clicks the link.
func (s *Store) challengeURL(code string) string {
	if code != "" {
		return fmt.Sprintf("https://www.%s/a/code?cbl-code=%s", s.handle, code)
	}

	return fmt.Sprintf("https://www.%s/a/code", s.handle)
}

// Login implements shop.Store.Login for Amazon using the Fire TV device code
// auth flow.
func (s *Store) Login(ctx context.Context, _ map[string]string) (*shop.LoginResult, error) {
	state, err := s.loadAuth()
	if err != nil {
		return nil, err
	}

	// Already authenticated — idempotent return.
	if state != nil && state.isAuthenticated() {
		return &shop.LoginResult{
			Authenticated: true,
			Account:       accountInfoFromState(state),
		}, nil
	}

	// Pending state exists and not expired — poll once.
	if state != nil && state.isPending() && !state.isExpired() {
		return s.pollRegistration(ctx, state)
	}

	// No state, expired, or otherwise invalid — start fresh.
	return s.startCodePairFlow(ctx)
}

// Logout implements shop.Store.Logout for Amazon.
// ctx unused: local file operation only. Thread it through if token
// revocation is added later.
func (s *Store) Logout(_ context.Context) error {
	if err := config.DeleteAuth(s.configDir, s.handle); err != nil {
		return shop.Errorf(shop.ErrInternal, "delete auth: %v", err)
	}

	return nil
}

// WhoAmI implements shop.Store.WhoAmI for Amazon.
//
// Fetches account identity (name, email) from the Alexa users/me endpoint
// using session cookies, then merges with locally stored auth state.
func (s *Store) WhoAmI(ctx context.Context) (*shop.AccountInfo, error) {
	api, err := s.tvssAPI()
	if err != nil {
		return nil, err
	}

	info := accountInfoFromState(api.state)

	// Enrich with account identity from the Alexa API.
	profile, err := s.fetchAlexaProfile(ctx, api)
	if err == nil && profile != nil {
		info.AccountName = profile.FullName
		info.Email = profile.Email
		if profile.ID != "" {
			info.AccountID = profile.ID
		}
	}

	return info, nil
}

// startCodePairFlow generates a new device, creates a code pair, persists the
// pending state, and returns a challenge.
func (s *Store) startCodePairFlow(ctx context.Context) (*shop.LoginResult, error) {
	d, err := generateDevice()
	if err != nil {
		return nil, err // already a *shop.Error
	}

	cp, err := generateCodePair(ctx, s.client, d)
	if err != nil {
		return nil, err
	}

	expiresAt := time.Now().Add(time.Duration(cp.ExpiresIn) * time.Second)

	state := &authState{
		State:       statePending,
		Device:      d,
		PrivateCode: cp.PrivateCode,
		PublicCode:  cp.PublicCode,
		ExpiresAt:   expiresAt.Format(time.RFC3339),
	}

	if err := s.saveAuth(state); err != nil {
		return nil, err
	}

	url := s.challengeURL(cp.PublicCode)

	return &shop.LoginResult{
		Authenticated: false,
		Challenge: &shop.Challenge{
			URL:       url,
			Code:      cp.PublicCode,
			ExpiresAt: expiresAt.Format(time.RFC3339),
			Message:   "Open " + url + " to authorize",
		},
	}, nil
}

// pollRegistration attempts a single register call for the pending state.
// On success it persists the authenticated state. On not-yet-complete it
// returns the same challenge.
func (s *Store) pollRegistration(ctx context.Context, state *authState) (*shop.LoginResult, error) {
	d := state.Device

	result, err := registerDevice(ctx, s.client, d, state.PublicCode, state.PrivateCode)
	if err != nil {
		return nil, err
	}

	// User hasn't completed auth yet — return the existing challenge.
	if result == nil {
		url := s.challengeURL(state.PublicCode)

		return &shop.LoginResult{
			Authenticated: false,
			Challenge: &shop.Challenge{
				URL:       url,
				Code:      state.PublicCode,
				ExpiresAt: state.ExpiresAt,
				Message:   "Open " + url + " to authorize",
			},
		}, nil
	}

	// Registration succeeded — save authenticated state with full device.
	authenticated := &authState{
		State:             stateAuthenticated,
		Device:            d,
		CustomerID:        result.CustomerID,
		Cookies:           result.Cookies,
		CookiesExpiry:     result.CookiesExpiry.Format(time.RFC3339),
		RefreshToken:      result.RefreshToken,
		BearerToken:       result.BearerToken,
		BearerTokenExpiry: result.BearerTokenExpiry.Format(time.RFC3339),
		AuthenticatedAt:   time.Now().Format(time.RFC3339),
	}

	if err := s.saveAuth(authenticated); err != nil {
		return nil, err
	}

	return &shop.LoginResult{
		Authenticated: true,
		Account:       accountInfoFromState(authenticated),
	}, nil
}

// loadAuth reads and unmarshals the auth state from disk. Returns nil, nil if
// no auth file exists.
func (s *Store) loadAuth() (*authState, error) {
	raw, err := config.LoadAuth(s.configDir, s.handle)
	if err != nil {
		return nil, shop.Errorf(shop.ErrConfigError, "load auth: %v", err)
	}
	if raw == nil {
		return nil, nil
	}

	var state authState
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, shop.Errorf(shop.ErrConfigError, "parse auth state: %v", err)
	}

	return &state, nil
}

// saveAuth marshals and persists the auth state to disk.
func (s *Store) saveAuth(state *authState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return shop.Errorf(shop.ErrInternal, "marshal auth state: %v", err)
	}

	if err := config.SaveAuth(s.configDir, s.handle, data); err != nil {
		return shop.Errorf(shop.ErrConfigError, "save auth: %v", err)
	}

	return nil
}

// alexaProfile is the subset of fields we care about from the
// Alexa /api/users/me endpoint.
type alexaProfile struct {
	ID       string `json:"id"`
	FullName string `json:"fullName"`
	Email    string `json:"email"`
}

// fetchAlexaProfile calls the domain-specific Alexa API with session cookies
// to retrieve account identity. Returns nil on any failure — callers should
// treat this as best-effort enrichment.
func (s *Store) fetchAlexaProfile(ctx context.Context, api *tvssClient) (*alexaProfile, error) {
	alexaURL := fmt.Sprintf("https://alexa.%s/api/users/me", s.handle)
	req, err := api.newRequest(ctx, http.MethodGet, alexaURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", mobileUA)
	req.Header.Set("Accept", "application/json")

	resp, err := api.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("alexa users/me: %d", resp.StatusCode)
	}

	var profile alexaProfile
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return nil, err
	}

	return &profile, nil
}

// accountInfoFromState converts authenticated auth state into the common
// AccountInfo type. ExpiresAt is set to the cookies expiry — the longest-lived
// credential — since bearer tokens refresh transparently and the cookies
// represent the actual auth lifetime. AccountName is omitted because the
// Amazon register response does not include a display name.
func accountInfoFromState(state *authState) *shop.AccountInfo {
	return &shop.AccountInfo{
		Authenticated: true,
		AccountID:     state.CustomerID,
		ExpiresAt:     state.CookiesExpiry,
	}
}
