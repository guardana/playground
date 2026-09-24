package gateway

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// Key is the lab key's public half as the enforcer's keygen printed it.
type Key struct {
	ID     string
	Public string
}

// ParseKey reads keygen's two lines, `key_id: ...` and `public_key: ...`.
func ParseKey(lines []byte) (Key, error) {
	var key Key
	scanner := bufio.NewScanner(bytes.NewReader(lines))
	for scanner.Scan() {
		name, value, ok := strings.Cut(scanner.Text(), ": ")
		switch {
		case !ok:
			return Key{}, fmt.Errorf("%w: key line %q is not `name: value`", ErrInvalid, scanner.Text())
		case name == "key_id":
			key.ID = value
		case name == "public_key":
			key.Public = value
		default:
			return Key{}, fmt.Errorf("%w: key line names %q", ErrInvalid, name)
		}
	}
	if key.ID == "" || key.Public == "" {
		return Key{}, fmt.Errorf("%w: the key lines lack key_id or public_key", ErrInvalid)
	}
	return key, scanner.Err()
}

// BundleID reads the id a policy document names for its bundle; the plane
// serves that bundle and no other.
func BundleID(document []byte) (string, error) {
	var parsed struct {
		Bundle struct {
			ID string `json:"id"`
		} `json:"bundle"`
	}
	if err := json.Unmarshal(document, &parsed); err != nil {
		return "", fmt.Errorf("%w: the policy document: %w", ErrInvalid, err)
	}
	if parsed.Bundle.ID == "" {
		return "", fmt.Errorf("%w: the policy document names no bundle id", ErrInvalid)
	}
	return parsed.Bundle.ID, nil
}

// Class is the lab's classification of one tool: what the tool does, decided
// by reading the victim's code, never its annotations.
type Class struct {
	Upstream     string `json:"upstream"`
	Tool         string `json:"tool"`
	Effect       string `json:"effect"`
	ResourceType string `json:"resource_type"`
	ResourceFrom string `json:"resource_from,omitempty"`
	TrustZone    string `json:"trust_zone,omitempty"`
}

// Fingerprint is what the enforcer's doctor printed for one tool definition.
type Fingerprint struct {
	Upstream    string `json:"upstream"`
	Tool        string `json:"tool"`
	Fingerprint string `json:"fingerprint"`
}

// Overrides joins the classification with the fingerprints, leaving out the
// tools a scenario keeps unclassified on purpose (`upstream/tool`). A class
// with no fingerprint, or two, is refused: it would classify a definition
// nobody printed.
func Overrides(classes []Class, prints []Fingerprint, unclassified []string) ([]Override, error) {
	var overrides []Override
	for _, class := range classes {
		name := class.Upstream + "/" + class.Tool
		if slices.Contains(unclassified, name) {
			continue
		}
		var found []string
		for _, printed := range prints {
			if printed.Upstream == class.Upstream && printed.Tool == class.Tool {
				found = append(found, printed.Fingerprint)
			}
		}
		if len(found) != 1 {
			return nil, fmt.Errorf("%w: %s has %d fingerprints, want one", ErrInvalid, name, len(found))
		}
		overrides = append(overrides, Override{
			Upstream: class.Upstream, Tool: class.Tool, Fingerprint: found[0], Effect: class.Effect,
			ResourceType: class.ResourceType, ResourceFrom: class.ResourceFrom, TrustZone: class.TrustZone,
		})
	}
	for _, name := range unclassified {
		if !slices.ContainsFunc(classes, func(c Class) bool { return c.Upstream+"/"+c.Tool == name }) {
			return nil, fmt.Errorf("%w: %s is kept unclassified and the lab classifies no such tool", ErrInvalid, name)
		}
	}
	return overrides, nil
}
