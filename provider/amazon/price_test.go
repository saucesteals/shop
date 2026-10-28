package amazon

import "testing"

func TestParsePriceCents(t *testing.T) {
	tests := []struct {
		in       string
		currency string
		want     int64
	}{
		{"€2,243.99", "EUR", 224399},
		{"€1,421.00", "EUR", 142100},
		{"€29.99", "EUR", 2999},
		{"RRP: €2,899.00", "EUR", 289900},
		{"29,99 €", "EUR", 2999},
		{"1.234,56 €", "EUR", 123456},
		{"€1.234", "EUR", 123400},
		{"8.99", "EUR", 899},
		{"2243.99", "EUR", 224399},
		{"$1,299.99", "USD", 129999},
		{"£29.9", "GBP", 2990},
		{"¥2,999", "JPY", 2999},
		{"", "EUR", 0},
		{"N/A", "EUR", 0},
	}
	for _, tt := range tests {
		if got := parsePriceCents(tt.in, tt.currency); got != tt.want {
			t.Errorf("parsePriceCents(%q, %q) = %d, want %d", tt.in, tt.currency, got, tt.want)
		}
	}
}
