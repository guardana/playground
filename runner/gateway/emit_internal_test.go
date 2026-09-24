package gateway

import (
	"errors"
	"testing"
)

// The enforcer reads a dotted key as the nested keys it spells, so the writer
// refuses one wherever it sits, whatever let it through.
func TestEmitRefusesAKeyHoldingADot(t *testing.T) {
	for name, value := range map[string]map[string]any{
		"at the top":       {"overrides.0.effect": "READ"},
		"nested":           {"pdp": map[string]any{"identifier.x": "y"}},
		"inside a list":    {"upstreams": []any{map[string]any{"tenant_id.x": "y"}}},
		"a dot on its own": {".": "x"},
	} {
		if _, err := emit(value); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s was written: %v", name, err)
		}
	}
}
