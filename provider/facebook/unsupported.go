package facebook

import (
	"context"

	"github.com/saucesteals/shop"
)

func (s *Store) Login(_ context.Context, _ map[string]string) (*shop.LoginResult, error) {
	return nil, shop.NotImplemented("facebook", "login")
}

func (s *Store) Logout(context.Context) error {
	return shop.NotImplemented("facebook", "logout")
}

func (s *Store) Offers(_ context.Context, _ string, _ *shop.OffersQuery) (*shop.OffersResult, error) {
	return nil, shop.NotImplemented("facebook", "offers")
}

func (s *Store) Reviews(_ context.Context, _ string, _ *shop.ReviewsQuery) (*shop.ReviewsResult, error) {
	return nil, shop.NotImplemented("facebook", "reviews")
}

func (s *Store) Variants(_ context.Context, _ string) (*shop.VariantsResult, error) {
	return nil, shop.NotImplemented("facebook", "variants")
}

func (s *Store) Checkout(_ context.Context, _ *shop.CheckoutOpts) (*shop.CheckoutResult, error) {
	return nil, shop.NotImplemented("facebook", "checkout")
}

func (s *Store) PlaceOrder(_ context.Context, _ string) (*shop.Order, error) {
	return nil, shop.NotImplemented("facebook", "order placement")
}

func (s *Store) Addresses(context.Context) ([]shop.Address, error) {
	return nil, shop.NotImplemented("facebook", "addresses")
}

func (s *Store) PaymentMethods(context.Context) ([]shop.PaymentMethod, error) {
	return nil, shop.NotImplemented("facebook", "payment methods")
}

func (s *Store) Cart() shop.Cart { return &unsupportedCart{} }

type unsupportedCart struct{}

func (c *unsupportedCart) Add(_ context.Context, _ string, _ int, _ *shop.CartAddOptions) (*shop.CartContents, error) {
	return nil, shop.NotImplemented("facebook", "cart")
}

func (c *unsupportedCart) Remove(_ context.Context, _ string) (*shop.CartContents, error) {
	return nil, shop.NotImplemented("facebook", "cart")
}

func (c *unsupportedCart) View(context.Context) (*shop.CartContents, error) {
	return nil, shop.NotImplemented("facebook", "cart")
}

func (c *unsupportedCart) Clear(context.Context) (*shop.CartContents, error) {
	return nil, shop.NotImplemented("facebook", "cart")
}
