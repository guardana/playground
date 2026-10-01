package gateway

import (
	"errors"
	"testing"
)

// Each of these characters either starts a YAML indicator or ends a plain key
// early, so a key carrying one would be read as something other than its name.
func TestEmitRefusesAKeyCarryingAYAMLIndicator(t *testing.T) {
	tests := []struct {
		name string
		char string
	}{
		{"colon", ":"}, {"hash", "#"}, {"double quote", `"`}, {"single quote", "'"},
		{"ampersand", "&"}, {"asterisk", "*"}, {"open brace", "{"}, {"close brace", "}"},
		{"open bracket", "["}, {"close bracket", "]"}, {"pipe", "|"}, {"greater than", ">"},
		{"bang", "!"}, {"percent", "%"}, {"space", " "}, {"tab", "\t"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			key := "tenant" + test.char + "id"
			if out, err := emit(map[string]any{key: "x"}); !errors.Is(err, ErrInvalid) {
				t.Errorf("key %q was written as %q (err %v)", key, out, err)
			}
		})
	}
}

func TestEmitWritesAPlainKey(t *testing.T) {
	out, err := emit(map[string]any{"tenant_id": "x"})
	if err != nil {
		t.Fatalf("a plain key was refused: %v", err)
	}
	if string(out) != "tenant_id: \"x\"\n" {
		t.Errorf("a plain key was written as %q", out)
	}
}
