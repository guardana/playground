package gateway

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// emit writes value in the subset of YAML the enforcer reads: nested block
// mappings, sequences indented under their key, keys plain and free of dots
// (the enforcer reads a dotted key as the nested keys it spells), every scalar
// double-quoted with only the escapes it knows. No flow collection, anchor or
// tag can appear, whatever the scenario's own file held.
func emit(value any) ([]byte, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var generic any
	if err := json.Unmarshal(encoded, &generic); err != nil {
		return nil, err
	}
	root, ok := generic.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: the configuration is not a mapping", ErrInvalid)
	}
	var out strings.Builder
	if err := emitMapping(&out, root, 0); err != nil {
		return nil, err
	}
	return []byte(out.String()), nil
}

func emitMapping(out *strings.Builder, mapping map[string]any, indent int) error {
	keys := make([]string, 0, len(mapping))
	for key := range mapping {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if strings.ContainsAny(key, ".:#\"'&*{}[]|>!% \t") || key == "" {
			return fmt.Errorf("%w: key %q", ErrInvalid, key)
		}
		if err := emitEntry(out, strings.Repeat(" ", indent)+key+":", mapping[key], indent); err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
	}
	return nil
}

func emitEntry(out *strings.Builder, head string, value any, indent int) error {
	switch typed := value.(type) {
	case map[string]any:
		out.WriteString(head + "\n")
		return emitMapping(out, typed, indent+2)
	case []any:
		out.WriteString(head + "\n")
		return emitSequence(out, typed, indent+2)
	default:
		text, err := quoted(typed)
		if err != nil {
			return err
		}
		out.WriteString(head + " " + text + "\n")
		return nil
	}
}

func emitSequence(out *strings.Builder, items []any, indent int) error {
	for _, item := range items {
		switch typed := item.(type) {
		case map[string]any:
			out.WriteString(strings.Repeat(" ", indent) + "-\n")
			if err := emitMapping(out, typed, indent+2); err != nil {
				return err
			}
		case []any:
			return fmt.Errorf("%w: a sequence inside a sequence", ErrInvalid)
		default:
			text, err := quoted(typed)
			if err != nil {
				return err
			}
			out.WriteString(strings.Repeat(" ", indent) + "- " + text + "\n")
		}
	}
	return nil
}

func quoted(value any) (string, error) {
	var text string
	switch typed := value.(type) {
	case string:
		text = typed
	case bool, float64:
		text = fmt.Sprint(typed)
	case nil:
		return "", fmt.Errorf("%w: an empty value", ErrInvalid)
	default:
		return "", fmt.Errorf("%w: a value of type %T", ErrInvalid, value)
	}
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\t", `\t`)
	return `"` + replacer.Replace(text) + `"`, nil
}
