package amazon

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// unwrapEntity extracts the "entity" field from a TVSS envelope if present.
// TVSS wraps many responses and nested objects in {"resource":..,"type":..,"entity":{...}}.
// If the data doesn't contain an entity field, it's returned as-is.
func unwrapEntity(data []byte) []byte {
	var envelope struct {
		Entity json.RawMessage `json:"entity"`
	}
	if json.Unmarshal(data, &envelope) == nil && len(envelope.Entity) > 0 {
		return envelope.Entity
	}

	return data
}

// jsonNumber handles JSON values that may be either a number or a string
// representation of a number. Amazon's TVSS API is inconsistent — some
// numeric fields are quoted strings, others are raw numbers.
type jsonNumber float64

func (n *jsonNumber) UnmarshalJSON(data []byte) error {
	// Try as number first.
	var f float64
	if err := json.Unmarshal(data, &f); err == nil {
		*n = jsonNumber(f)

		return nil
	}

	// Try as quoted string.
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return err
		}
		*n = jsonNumber(f)

		return nil
	}

	return fmt.Errorf("jsonNumber: cannot unmarshal %s", string(data))
}

// Float64 returns the value as a float64.
func (n jsonNumber) Float64() float64 { return float64(n) }

// flexString handles TVSS fields that can be either a plain string or an
// object with a displayString field: "Visa" or {"displayString":"Visa"}.
type flexString string

func (f *flexString) UnmarshalJSON(data []byte) error {
	// Try plain string first.
	var s string
	if json.Unmarshal(data, &s) == nil {
		*f = flexString(s)
		return nil
	}

	// Try object with displayString.
	var obj struct {
		DisplayString string `json:"displayString"`
	}
	if json.Unmarshal(data, &obj) == nil {
		*f = flexString(obj.DisplayString)
		return nil
	}

	// Fall back to raw string representation.
	*f = flexString(string(data))
	return nil
}

func (f flexString) String() string { return string(f) }
