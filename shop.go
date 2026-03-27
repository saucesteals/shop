// Package shop defines the core interfaces and types for a multi-platform
// shopping CLI. Providers implement these interfaces; the CLI consumes them.
package shop

import "context"

// Provider is a factory that creates Store instances for compatible domains.
// Each provider handles a family of stores (e.g., "amazon" handles
// amazon.com/co.uk/etc.).
type Provider interface {
	// Name returns the provider's unique identifier (e.g., "amazon").
	Name() string

	// Detect probes a handle/domain and returns true if this provider can
	// handle it. Called during auto-discovery for unknown domains.
	// Implementations should be fast and not require auth. Returns a
	// StoreInfo if detected, which gets cached in the registry.
	Detect(ctx context.Context, handle string) (*StoreInfo, error)

	// DetectCost indicates how expensive Detect is, allowing the resolution
	// layer to sort providers optimally (cheapest first).
	DetectCost() DetectCost

	// Store creates a Store instance for the given handle. The handle is
	// the canonical domain for the store. configDir is the path to the
	// CLI's config directory (e.g., ~/.config/shop) for auth file I/O.
	Store(ctx context.Context, handle string, configDir string) (Store, error)
}

// Store is the primary shopping interface. All operations are scoped to a
// single merchant/domain. Implementations handle their own HTTP clients,
// auth token refresh, and platform-specific API details.
type Store interface {
	// Info returns metadata about this store.
	Info() StoreInfo

	// Login authenticates with the store. Handles the full lifecycle:
	//   - No existing state + no creds → starts device code/OAuth flow, returns challenge
	//   - No existing state + creds → authenticates directly, returns authenticated
	//   - Pending challenge exists → polls for completion
	//   - Already authenticated → returns authenticated (idempotent)
	Login(ctx context.Context, creds map[string]string) (*LoginResult, error)

	// Logout revokes credentials and clears stored tokens.
	Logout(ctx context.Context) error

	// WhoAmI returns the current auth state. Read-only — no side effects.
	WhoAmI(ctx context.Context) (*AccountInfo, error)

	// Search performs a product search with the given query and filters.
	Search(ctx context.Context, query *SearchQuery) (*SearchResult, error)

	// Product returns full details for a single product by its opaque ID.
	Product(ctx context.Context, productID string) (*Product, error)

	// Offers returns all available offers (sellers/conditions) for a product.
	Offers(ctx context.Context, productID string, opts *OffersQuery) (*OffersResult, error)

	// Reviews returns customer reviews for a product.
	Reviews(ctx context.Context, productID string, opts *ReviewsQuery) (*ReviewsResult, error)

	// Variants returns the full variant tree for a product.
	Variants(ctx context.Context, productID string) (*VariantsResult, error)

	// Cart returns the cart interface for this store. The cart is a singleton
	// per store instance.
	Cart() Cart

	// Checkout previews the current cart as an order. Returns a full cost
	// breakdown and a checkout ID (hash of cart state). Does not place the order.
	Checkout(ctx context.Context, opts *CheckoutOpts) (*CheckoutResult, error)

	// PlaceOrder commits the checkout. The checkoutID must match the hash
	// from a prior Checkout() call — if the cart changed, this fails with
	// ErrCartChanged.
	PlaceOrder(ctx context.Context, checkoutID string) (*Order, error)

	// Addresses returns saved shipping addresses for the authenticated account.
	Addresses(ctx context.Context) ([]Address, error)

	// PaymentMethods returns saved payment methods for the authenticated account.
	PaymentMethods(ctx context.Context) ([]PaymentMethod, error)

	// Capabilities returns which optional features this store supports.
	Capabilities() Capabilities
}

// Cart manages line items for a single store. Cart state is internal to the
// provider implementation — the CLI only reads it via View().
type Cart interface {
	// Add adds a product to the cart. quantity must be >= 1.
	Add(ctx context.Context, id string, quantity int) (*CartContents, error)

	// Remove removes a product entirely from the cart.
	Remove(ctx context.Context, id string) (*CartContents, error)

	// View returns the current cart snapshot.
	View(ctx context.Context) (*CartContents, error)

	// Clear empties the cart.
	Clear(ctx context.Context) (*CartContents, error)
}

// DetectCost indicates how expensive a provider's Detect call is.
type DetectCost int

const (
	// DetectCostFree is a pure string match, no network.
	DetectCostFree DetectCost = 0
	// DetectCostCheap is a single HTTP request.
	DetectCostCheap DetectCost = 1
	// DetectCostModerate is multiple requests or DNS probes.
	DetectCostModerate DetectCost = 2
)

// Capabilities declares which optional features a store implementation supports.
type Capabilities struct {
	Search          bool `json:"search"`
	Reviews         bool `json:"reviews"`
	Offers          bool `json:"offers"`
	Variants        bool `json:"variants"`
	Cart            bool `json:"cart"`
	Checkout        bool `json:"checkout"`
	Addresses       bool `json:"addresses"`
	PaymentMethods  bool `json:"paymentMethods"`
	ShippingOptions bool `json:"shippingOptions"`
	Coupons         bool `json:"coupons"`
}
