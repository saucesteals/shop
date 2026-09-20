package tracking

import (
	"context"
	"fmt"
)

// Tracker retrieves a shipment's available history.
type Tracker interface {
	Track(context.Context, string) (*Snapshot, error)
}

// Provider declares the carriers for which it supplies tracking history.
type Provider interface {
	Tracker
	Carriers() []Carrier
}

// Registry is an immutable carrier-to-provider mapping.
// Construct it once before concurrent use; providers must support concurrent lookups.
type Registry struct {
	handlers map[Carrier]Tracker
}

// NewRegistry rejects undeclared carriers and duplicate handlers instead of
// letting registration order silently decide which source wins.
func NewRegistry(providers ...Provider) (*Registry, error) {
	r := &Registry{
		handlers: make(map[Carrier]Tracker),
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
			case UPS, USPS, FedEx, GOFO, Yanwen:
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

// Track selects the registered provider or reports an unsupported carrier.
// Provider errors are preserved unless the caller canceled the request. Successful
// snapshots are validated; requests never silently switch sources.
func (r *Registry) Track(ctx context.Context, number string) (*Snapshot, error) {
	number, err := NormalizeNumber(number)
	if err != nil {
		return nil, err
	}
	provider, ok := r.handlers[DetectCarrier(number)]
	if !ok {
		return nil, &Error{Kind: ErrNotSupported, Message: "tracking carrier is not supported"}
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	snapshot, err := provider.Track(ctx, number)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, err
	}
	if err := validateSnapshot(snapshot, number); err != nil {
		return nil, err
	}

	return snapshot, nil
}
