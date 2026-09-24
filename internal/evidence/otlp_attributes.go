package evidence

import (
	"fmt"
	"slices"
	"strings"
)

// expectedAttribute is one attribute the enforcer writes beside an event. An
// empty value means the attribute must be absent.
type expectedAttribute struct {
	name, value string
}

// expectedAttributes lists what the event's attributes must say. The two enums
// are written even at their zero, by the zero's name, which the body leaves
// out.
func expectedAttributes(event Event) []expectedAttribute {
	return []expectedAttribute{
		{"event_id", event.EventID},
		{"request_id", event.RequestID},
		{"run_id", event.RunID},
		{"project_id", event.ProjectID},
		{"tenant_id", event.TenantID},
		{"kind", orZero(string(event.Kind), string(KindUnspecified))},
		{"enforcement_mode", orZero(event.EnforcementMode, enforcementModePrefix+"UNSPECIFIED")},
	}
}

func orZero(value, zero string) string {
	if value == "" {
		return zero
	}
	return value
}

// checkAttributes compares the attributes under namespace with the event. Keys
// outside the namespace are someone else's, a processor's for instance, and
// are not read; inside it, an attribute this reader does not know is refused,
// as an unknown field in the event is.
func checkAttributes(event Event, attrs []keyValue, namespace string) error {
	got, err := namespaced(attrs, namespace+".")
	if err != nil {
		return err
	}
	for _, want := range expectedAttributes(event) {
		value, present := got[want.name]
		delete(got, want.name)
		switch {
		case want.value == "" && present:
			return fmt.Errorf("%w: %s is %q, the event has none", ErrAttributeMismatch, want.name, value)
		case want.value != "" && !present:
			return fmt.Errorf("%w: %s is absent, the event has %q", ErrAttributeMismatch, want.name, want.value)
		case present && value != want.value:
			return fmt.Errorf("%w: %s is %q, the event has %q", ErrAttributeMismatch, want.name, value, want.value)
		}
	}
	if len(got) > 0 {
		names := make([]string, 0, len(got))
		for name := range got {
			names = append(names, name)
		}
		slices.Sort(names)
		return fmt.Errorf("%w: unknown attribute %s%s", ErrAttributeMismatch, namespace+".", names[0])
	}
	return nil
}

func namespaced(attrs []keyValue, prefix string) (map[string]string, error) {
	got := make(map[string]string)
	for _, attr := range attrs {
		name, inside := strings.CutPrefix(attr.Key, prefix)
		if !inside {
			continue
		}
		if _, repeated := got[name]; repeated {
			return nil, fmt.Errorf("%w: %s appears twice", ErrAttributeMismatch, attr.Key)
		}
		value, err := stringValue(attr.Value)
		if err != nil {
			return nil, fmt.Errorf("attribute %s: %w", attr.Key, err)
		}
		got[name] = value
	}
	return got, nil
}
