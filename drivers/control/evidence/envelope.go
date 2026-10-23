package evidence

import "time"

// ActionEnvelope is the action a caller proposed, as the trail records it. The
// enums are strings because that is what protojson writes and because a number
// this reader has no name for is a fact about the producer's taxonomy, not a
// parse error.
type ActionEnvelope struct {
	SchemaVersion string       `json:"schemaVersion,omitempty"`
	RequestID     string       `json:"requestId,omitempty"`
	TraceID       string       `json:"traceId,omitempty"`
	SpanID        string       `json:"spanId,omitempty"`
	OccurredAt    *time.Time   `json:"occurredAt,omitempty"`
	ProjectID     string       `json:"projectId,omitempty"`
	TenantID      string       `json:"tenantId,omitempty"`
	Environment   string       `json:"environment,omitempty"`
	Principal     *Principal   `json:"principal,omitempty"`
	Agent         *Agent       `json:"agent,omitempty"`
	Delegation    []Delegation `json:"delegation,omitempty"`
	Action        *Action      `json:"action,omitempty"`
	Resource      *Resource    `json:"resource,omitempty"`
	Destination   *Destination `json:"destination,omitempty"`
	Data          *DataLabels  `json:"data,omitempty"`
	Arguments     *Arguments   `json:"arguments,omitempty"`
	// The protojson key is "context"; the message is RunContext.
	Context *RunContext `json:"context,omitempty"`
}

// Principal is the identity the call is made on behalf of. TenantID here is one
// half of the cross-tenant comparison; the other half is Resource.TenantID, and
// the envelope's own tenant is neither.
type Principal struct {
	ID            string            `json:"id,omitempty"`
	Type          string            `json:"type,omitempty"`
	AuthnStrength string            `json:"authnStrength,omitempty"`
	TenantID      string            `json:"tenantId,omitempty"`
	Attributes    map[string]string `json:"attributes,omitempty"`
}

// Agent is the caller's description of itself, which nothing verifies.
type Agent struct {
	ID         string `json:"id,omitempty"`
	InstanceID string `json:"instanceId,omitempty"`
	Framework  string `json:"framework,omitempty"`
	Version    string `json:"version,omitempty"`
	ModelRef   string `json:"modelRef,omitempty"`
}

// Delegation is one hop of delegated authority. The list is ordered from the
// root outwards, and reading it backwards turns the "child exceeds parent"
// check into a no-op, so the order is part of the record rather than a
// convention.
type Delegation struct {
	From      string     `json:"from,omitempty"`
	To        string     `json:"to,omitempty"`
	Scopes    []string   `json:"scopes,omitempty"`
	Reason    string     `json:"reason,omitempty"`
	IssuedAt  *time.Time `json:"issuedAt,omitempty"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
}

// Action names what is being done. Effect is what policy keys on; the tool name
// is not.
type Action struct {
	Kind     string `json:"kind,omitempty"`
	Name     string `json:"name,omitempty"`
	Protocol string `json:"protocol,omitempty"`
	Effect   string `json:"effect,omitempty"`
	Provider string `json:"provider,omitempty"`
}

// Resource is what the call names. TenantID is the owner, which is what the
// cross-tenant check compares against the principal's.
type Resource struct {
	Type        string            `json:"type,omitempty"`
	ID          string            `json:"id,omitempty"`
	TenantID    string            `json:"tenantId,omitempty"`
	Environment string            `json:"environment,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
}

// Destination is where the call sends data. There is no "external" boolean:
// an unspecified trust zone counts as untrusted.
type Destination struct {
	TrustZone string `json:"trustZone,omitempty"`
	Host      string `json:"host,omitempty"`
}

// DataLabels is what the producer asserted about the data. ContainsSecrets is a
// positive assertion only: false means nobody asserted secrets are present,
// never that their absence was established.
type DataLabels struct {
	Sensitivities   []string `json:"sensitivities,omitempty"`
	Sources         []string `json:"sources,omitempty"`
	ContainsSecrets bool     `json:"containsSecrets,omitempty"`
}

// Arguments never travel whole. RedactedPreview is the field a scenario asserts
// content capture against: text here where the scenario expects none means the
// privacy default did not hold.
type Arguments struct {
	CanonicalHash    string `json:"canonicalHash,omitempty"`
	RedactedPreview  string `json:"redactedPreview,omitempty"`
	SchemaRef        string `json:"schemaRef,omitempty"`
	RedactionProfile string `json:"redactionProfile,omitempty"`
}

// RunContext is what changes between two attempts at one action, which is why
// none of it is in the canonical action digest.
type RunContext struct {
	SessionID string           `json:"sessionId,omitempty"`
	RunID     string           `json:"runId,omitempty"`
	StepID    string           `json:"stepId,omitempty"`
	Risk      string           `json:"risk,omitempty"`
	Budgets   map[string]Int64 `json:"budgets,omitempty"`
	Tags      []string         `json:"tags,omitempty"`
}
