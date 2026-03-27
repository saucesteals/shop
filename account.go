package shop

// Address is a shipping/billing address.
type Address struct {
	ID         string `json:"id"`
	Label      string `json:"label,omitempty"`
	Name       string `json:"name"`
	Line1      string `json:"line1"`
	Line2      string `json:"line2,omitempty"`
	City       string `json:"city"`
	State      string `json:"state,omitempty"`
	PostalCode string `json:"postalCode"`
	Country    string `json:"country"`
	Phone      string `json:"phone,omitempty"`
	IsDefault  bool   `json:"isDefault"`
}

// PaymentMethod is a saved payment instrument. Sensitive details are masked.
type PaymentMethod struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Label     string `json:"label"`
	Last4     string `json:"last4,omitempty"`
	ExpMonth  int    `json:"expMonth,omitempty"`
	ExpYear   int    `json:"expYear,omitempty"`
	IsDefault bool   `json:"isDefault"`
}

// ShippingOption is a selectable shipping speed/method during checkout.
type ShippingOption struct {
	ID            string `json:"id"`
	Label         string `json:"label"`
	Price         Money  `json:"price"`
	EstimatedDays string `json:"estimatedDays,omitempty"`
	EstimatedDate string `json:"estimatedDate,omitempty"`
	IsDefault     bool   `json:"isDefault"`
}

// LoginResult is returned by Store.Login().
type LoginResult struct {
	Authenticated bool         `json:"authenticated"`
	Account       *AccountInfo `json:"account,omitempty"`
	Challenge     *Challenge   `json:"challenge,omitempty"`
}

// Challenge describes an external action the consumer must complete to
// finish authentication.
type Challenge struct {
	URL       string `json:"url"`
	Code      string `json:"code,omitempty"`
	ExpiresAt string `json:"expiresAt,omitempty"`
	Message   string `json:"message,omitempty"`
}

// AccountInfo is returned by Store.WhoAmI() and LoginResult.
type AccountInfo struct {
	Authenticated bool   `json:"authenticated"`
	AccountID     string `json:"accountId,omitempty"`
	AccountName   string `json:"accountName,omitempty"`
	Email         string `json:"email,omitempty"`
	ExpiresAt     string `json:"expiresAt,omitempty"`
}

// StoreInfo is metadata about a store.
type StoreInfo struct {
	Name     string `json:"name"`
	Domain   string `json:"domain"`
	Provider string `json:"provider"`
	Country  string `json:"country,omitempty"`
	Currency string `json:"currency,omitempty"`
	LogoURL  string `json:"logoUrl,omitempty"`
}

// RegistryEntry is a single known store in the persistent registry.
type RegistryEntry struct {
	Alias          string         `json:"alias"`
	Domain         string         `json:"domain"`
	Provider       string         `json:"provider"`
	Name           string         `json:"name"`
	Country        string         `json:"country,omitempty"`
	Currency       string         `json:"currency,omitempty"`
	BuiltIn        bool           `json:"builtIn"`
	DetectedAt     string         `json:"detectedAt,omitempty"`
	DetectedBy     string         `json:"detectedBy,omitempty"`
	ProviderConfig map[string]any `json:"providerConfig,omitempty"`
}
