package tracking

import (
	"context"
	"fmt"

	"github.com/saucesteals/shop"
)

// Provider declares the carriers for which it supplies tracking history.
type Provider interface {
	Tracker
	Carriers() []Carrier
}

// Registry is an immutable carrier-to-provider mapping with an explicit fallback.
// Construct it once before concurrent use; providers must support concurrent lookups.
type Registry struct {
	handlers map[Carrier]Tracker
	fallback Tracker
}

// NewRegistry rejects undeclared carriers and duplicate handlers instead of
// letting registration order silently decide which source wins.
func NewRegistry(fallback Tracker, providers ...Provider) (*Registry, error) {
	if fallback == nil {
		return nil, fmt.Errorf("tracking fallback is required")
	}
	r := &Registry{
		handlers: make(map[Carrier]Tracker),
		fallback: fallback,
	}
	for _, provider := range providers {
		if provider == nil {
			return nil, fmt.Errorf("tracking provider is required")
		}
		carriers := provider.Carriers()
		if len(carriers) == 0 {
			return nil, fmt.Errorf("tracking provider must declare a carrier")
		}
		for _, carrier := range carriers {
			switch carrier {
			case UPS, USPS:
			default:
				return nil, fmt.Errorf("undeclared tracking carrier %q", carrier)
			}
			if _, exists := r.handlers[carrier]; exists {
				return nil, fmt.Errorf("duplicate tracking handler for %q", carrier)
			}
			r.handlers[carrier] = provider
		}
	}

	return r, nil
}

// Track selects one provider. Fallback handles unknown or unregistered carriers,
// not failed requests: a selected provider's error is returned unchanged.
func (r *Registry) Track(ctx context.Context, number string) (*shop.TrackingSnapshot, error) {
	number, err := Number(number)
	if err != nil {
		return nil, err
	}
	provider, ok := r.handlers[DetectCarrier(number)]
	if !ok {
		provider = r.fallback
	}

	return provider.Track(ctx, number)
}
