package amazon

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/saucesteals/shop"
)

// bearerExpiryFromResponse converts Amazon's expires_in field (which may be
// a JSON number or a quoted string) into a duration. Falls back to 1 hour
// if parsing fails.
func bearerExpiryFromResponse(v json.Number) time.Duration {
	if secs, err := v.Int64(); err == nil {
		return time.Duration(secs) * time.Second
	}
	if secs, err := strconv.ParseInt(string(v), 10, 64); err == nil {
		return time.Duration(secs) * time.Second
	}
	return time.Hour // safe default
}

// API endpoints.
const (
	codePairURL = "https://api.amazon.com/auth/create/codepair"
	registerURL = "https://api.amazon.com/auth/register"
)

// Device registration constants — iOS (Amazon Shopping app).
const (
	defaultDeviceType    = "A3NWHXTQ4EBCZS"
	defaultDeviceDomain  = "Device"
	defaultAppName       = "Amazon Shopping"
	defaultAppVersion    = "24.20.2"
	defaultDeviceModel   = "iPhone"
	defaultOSVersion     = "17.6.1"
	defaultSoftwareVer   = "1"
	serialBytes          = 12
)

// HTTP constants.
const (
	// mobileUA is the standard iOS Safari user agent used for web requests
	// (search, Alexa profile, etc.). Matches the iOS version in defaultOSVersion.
	mobileUA = "Mozilla/5.0 (iPhone; CPU iPhone OS 17_6_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/21G93"

	// tvssUA is the Fire TV ShopTV user agent required by the TVSS API.
	// The TVSS backend validates this format — do not change without testing.
	tvssUA = "AMZN(SetTopBox/Amazon Fire TV Mantis/AKPGW064GI9HE,Android/7.1.2,ShopTV3P/release/2.0)"

	authDomain   = "api.amazon.com"
	cookieDomain = ".amazon.com"
)

// defaultCookieExpiry is the assumed cookie lifetime (~1 year) when the
// register response doesn't include an explicit expiry.
const defaultCookieExpiry = 365 * 24 * time.Hour

// maxErrorBodyLen caps response body length in error messages to avoid
// dumping large HTML error pages.
const maxErrorBodyLen = 512

// --- Request/Response types ---

// device represents an Amazon device registration payload.
type device struct {
	Domain          string `json:"domain"`
	DeviceType      string `json:"device_type"`
	DeviceSerial    string `json:"device_serial"`
	AppName         string `json:"app_name"`
	AppVersion      string `json:"app_version"`
	DeviceModel     string `json:"device_model"`
	OSVersion       string `json:"os_version"`
	SoftwareVersion string `json:"software_version"`
}

// codePairRequest is the request body for the code pair creation endpoint.
type codePairRequest struct {
	CodeData device `json:"code_data"`
}

// codePairResponse is the response from the code pair creation endpoint.
type codePairResponse struct {
	PublicCode  string `json:"public_code"`
	PrivateCode string `json:"private_code"`
	ExpiresIn   int    `json:"expires_in"`
}

// registerPayload is the full request body for the device registration endpoint.
type registerPayload struct {
	AuthData            registerAuthData `json:"auth_data"`
	RegistrationData    device           `json:"registration_data"`
	RequestedTokenType  []string         `json:"requested_token_type"`
	Cookies             registerCookies  `json:"cookies"`
	RequestedExtensions []string         `json:"requested_extensions"`
}

// registerAuthData carries the authentication method and code pair for
// device registration.
type registerAuthData struct {
	UseGlobalAuthentication string          `json:"use_global_authentication"`
	CodePair                registerCodePair `json:"code_pair"`
}

// registerCodePair holds the public/private code pair within the register
// request auth data.
type registerCodePair struct {
	PublicCode  string `json:"public_code"`
	PrivateCode string `json:"private_code"`
}

// registerCookies is the cookies section of the register request. The
// WebsiteCookies field is always an empty array for initial registration.
type registerCookies struct {
	Domain         string `json:"domain"`
	WebsiteCookies []any  `json:"website_cookies"` // always empty; any is required for []
}

// registerResponse captures the nested fields from a successful register
// endpoint response.
type registerResponse struct {
	Response struct {
		Success struct {
			Tokens struct {
				Bearer struct {
					AccessToken string      `json:"access_token"`
					ExpiresIn   json.Number `json:"expires_in"`
				} `json:"bearer"`
				WebsiteCookies []struct {
					Name  string `json:"Name"`
					Value string `json:"Value"`
				} `json:"website_cookies"`
			} `json:"tokens"`
			Extensions struct {
				CustomerInfo struct {
					CustomerID string `json:"customer_id"`
				} `json:"customer_info"`
			} `json:"extensions"`
		} `json:"success"`
	} `json:"response"`
	RefreshToken string `json:"refresh_token"`
}

// amazonErrorResponse is the error body Amazon returns for non-200 register
// responses. Amazon uses multiple error formats; this struct covers the known
// variants.
type amazonErrorResponse struct {
	Response struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	} `json:"response"`
	// Top-level error fields (alternative format used by some endpoints).
	Error   string `json:"error"`
	Message string `json:"message"`
}

// isAuthorizationPending reports whether this error indicates the user hasn't
// entered the code yet — the expected state during polling. Amazon uses
// different error codes across API versions: 401/Unauthorized for the
// register endpoint, 400/InvalidValue in some flows.
func (e *amazonErrorResponse) isAuthorizationPending() bool {
	code := e.Response.Error.Code

	return code == "InvalidValue" || code == "AuthorizationPending" ||
		code == "Unauthorized" ||
		e.Error == "authorization_pending" || e.Error == "AuthorizationPending"
}

// registrationResult holds the extracted fields from a successful device
// registration.
type registrationResult struct {
	CustomerID        string
	BearerToken       string
	BearerTokenExpiry time.Time
	RefreshToken      string
	Cookies           []authCookie
	CookiesExpiry     time.Time
}

// --- Device generation ---

// generateDevice creates a device with a random serial number.
func generateDevice() (device, error) {
	b := make([]byte, serialBytes)
	if _, err := rand.Read(b); err != nil {
		return device{}, shop.Errorf(shop.ErrInternal, "generate device serial: %v", err)
	}

	return device{
		Domain:          defaultDeviceDomain,
		DeviceType:      defaultDeviceType,
		DeviceSerial:    hex.EncodeToString(b),
		AppName:         defaultAppName,
		AppVersion:      defaultAppVersion,
		DeviceModel:     defaultDeviceModel,
		OSVersion:       defaultOSVersion,
		SoftwareVersion: defaultSoftwareVer,
	}, nil
}

// --- Code pair flow ---

// generateCodePair creates a code pair for the given device by calling the
// Amazon auth code pair endpoint. Validates that the response contains all
// required fields before returning.
func generateCodePair(ctx context.Context, client *http.Client, d device) (*codePairResponse, error) {
	body, err := json.Marshal(codePairRequest{CodeData: d})
	if err != nil {
		return nil, shop.Errorf(shop.ErrInternal, "marshal code pair request: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, codePairURL, bytes.NewReader(body))
	if err != nil {
		return nil, shop.Errorf(shop.ErrInternal, "build code pair request: %v", err)
	}
	setAuthHeaders(req)

	resp, err := client.Do(req)
	if err != nil {
		return nil, shop.Errorf(shop.ErrNetwork, "code pair request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, shop.Errorf(shop.ErrNetwork, "read code pair response: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, shop.Errorf(shop.ErrAuthFailed, "code pair request failed (%d): %s", resp.StatusCode, truncateBody(data))
	}

	var cp codePairResponse
	if err := json.Unmarshal(data, &cp); err != nil {
		return nil, shop.Errorf(shop.ErrAuthFailed, "parse code pair response: %v", err)
	}

	if cp.PublicCode == "" || cp.PrivateCode == "" || cp.ExpiresIn <= 0 {
		return nil, shop.Errorf(shop.ErrAuthFailed, "code pair response missing required fields")
	}

	return &cp, nil
}

// --- Device registration ---

// registerDevice attempts to register the device after the user has entered
// the code. Returns nil, nil if the user has not yet completed auth (the
// authorization_pending case). Returns a *shop.Error for actual failures
// (rate limits, server errors, malformed requests).
func registerDevice(ctx context.Context, client *http.Client, d device, publicCode, privateCode string) (*registrationResult, error) {
	payload := buildRegisterPayload(d, publicCode, privateCode)

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, shop.Errorf(shop.ErrInternal, "marshal register request: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, registerURL, bytes.NewReader(body))
	if err != nil {
		return nil, shop.Errorf(shop.ErrInternal, "build register request: %v", err)
	}
	setAuthHeaders(req)

	resp, err := client.Do(req)
	if err != nil {
		return nil, shop.Errorf(shop.ErrNetwork, "register request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, shop.Errorf(shop.ErrNetwork, "read register response: %v", err)
	}

	// Handle non-200 responses. Only authorization_pending means keep polling.
	if resp.StatusCode != http.StatusOK {
		return handleRegisterError(resp.StatusCode, data)
	}

	var rr registerResponse
	if err := json.Unmarshal(data, &rr); err != nil {
		return nil, shop.Errorf(shop.ErrAuthFailed, "parse register response: %v", err)
	}

	success := rr.Response.Success
	if success.Tokens.Bearer.AccessToken == "" {
		// 200 but no bearer token — treat as auth not yet complete.
		return nil, nil
	}

	now := time.Now()
	cookies := make([]authCookie, 0, len(success.Tokens.WebsiteCookies))
	for _, c := range success.Tokens.WebsiteCookies {
		cookies = append(cookies, authCookie{Name: c.Name, Value: c.Value})
	}

	return &registrationResult{
		CustomerID:        success.Extensions.CustomerInfo.CustomerID,
		BearerToken:       success.Tokens.Bearer.AccessToken,
		BearerTokenExpiry: now.Add(bearerExpiryFromResponse(success.Tokens.Bearer.ExpiresIn)),
		RefreshToken:      rr.RefreshToken,
		Cookies:           cookies,
		CookiesExpiry:     now.Add(defaultCookieExpiry),
	}, nil
}

// handleRegisterError inspects a non-200 register response and returns the
// appropriate error. Returns nil, nil only for the authorization_pending case
// (user hasn't entered the code yet).
func handleRegisterError(statusCode int, body []byte) (*registrationResult, error) {
	switch {
	case statusCode == http.StatusBadRequest || statusCode == http.StatusUnauthorized:
		// 400/401 — check if this is the expected "user hasn't entered code yet" case.
		// Amazon returns 401 Unauthorized during the device code flow when the
		// user hasn't entered the code, and 400 InvalidValue in some API versions.
		var ae amazonErrorResponse
		if err := json.Unmarshal(body, &ae); err == nil && ae.isAuthorizationPending() {
			return nil, nil
		}

		return nil, shop.Errorf(shop.ErrAuthFailed, "register device rejected (%d): %s", statusCode, truncateBody(body))

	case statusCode == http.StatusForbidden:
		// 403 — code already consumed or invalidated.
		return nil, shop.Errorf(shop.ErrAuthFailed, "register device forbidden: code may be consumed or invalidated")

	case statusCode == http.StatusTooManyRequests:
		return nil, shop.Errorf(shop.ErrRateLimited, "register device rate limited")

	case statusCode >= http.StatusInternalServerError:
		return nil, shop.Errorf(shop.ErrStoreError, "register device server error (%d)", statusCode)

	default:
		return nil, shop.Errorf(shop.ErrStoreError, "register device unexpected status (%d): %s", statusCode, truncateBody(body))
	}
}

// --- Helpers ---

// buildRegisterPayload constructs the typed register request body.
func buildRegisterPayload(d device, publicCode, privateCode string) registerPayload {
	return registerPayload{
		AuthData: registerAuthData{
			UseGlobalAuthentication: "true",
			CodePair: registerCodePair{
				PublicCode:  publicCode,
				PrivateCode: privateCode,
			},
		},
		RegistrationData: d,
		RequestedTokenType: []string{
			"bearer",
			"mac_dms",
			"store_authentication_cookie",
			"website_cookies",
		},
		Cookies: registerCookies{
			Domain:         cookieDomain,
			WebsiteCookies: []any{},
		},
		RequestedExtensions: []string{
			"device_info",
			"customer_info",
		},
	}
}

// setAuthHeaders applies the standard Amazon auth headers to a request.
func setAuthHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", mobileUA)
	req.Header.Set("x-amzn-identity-auth-domain", authDomain)
}

// truncateBody caps the response body length for inclusion in error messages
// to prevent large HTML error pages from polluting logs.
func truncateBody(data []byte) string {
	if len(data) <= maxErrorBodyLen {
		return string(data)
	}

	return string(data[:maxErrorBodyLen]) + "...(truncated)"
}
