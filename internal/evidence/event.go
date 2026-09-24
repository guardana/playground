// Package evidence reads the trail the enforcement plane writes, as the frozen
// v1 wire contract carries it over JSON: one event per line, field names in
// lower camel case, enums as their declared names, 64-bit integers as strings,
// timestamps in RFC 3339.
//
// It is a reader, not a client. The lab never imports the enforcement plane's
// Go module: what this repository tests is a pinned image, and a type shared
// through the file system would make the lab agree with a working copy instead
// of with the contract. The cost is that this mirror has to be kept in step
// with api/proto by hand, and a field added upstream shows up here as a refused
// line rather than as silence.
//
// The chain rule below is written from the order the contract documents, for
// the same reason. Calling the producer's own validator would ask the system
// under test whether it agrees with itself.
package evidence

import "time"

// Kind is an event kind as the wire carries it. It is a string rather than an
// enumeration of the eleven declared kinds because an undeclared one is not an
// error to this reader: it means the producer holds a taxonomy the lab does
// not, which ValidateChain reports as indeterminate.
type Kind string

// The kinds this version can place. Anything else is indeterminate.
const (
	KindActionProposed    Kind = "EVENT_KIND_ACTION_PROPOSED"
	KindPolicyDecided     Kind = "EVENT_KIND_POLICY_DECIDED"
	KindApprovalRequested Kind = "EVENT_KIND_APPROVAL_REQUESTED"
	KindApprovalDecided   Kind = "EVENT_KIND_APPROVAL_DECIDED"
	KindActionStarted     Kind = "EVENT_KIND_ACTION_STARTED"
	KindActionCompleted   Kind = "EVENT_KIND_ACTION_COMPLETED"
	KindActionFailed      Kind = "EVENT_KIND_ACTION_FAILED"
	KindActionBlocked     Kind = "EVENT_KIND_ACTION_BLOCKED"
	KindApprovalExpired   Kind = "EVENT_KIND_APPROVAL_EXPIRED"
	KindFindingRaised     Kind = "EVENT_KIND_FINDING_RAISED"
	KindPolicyReloaded    Kind = "EVENT_KIND_POLICY_RELOADED"
	KindUnspecified       Kind = "EVENT_KIND_UNSPECIFIED"
	verdictPrefix              = "VERDICT_"
	enforcementModePrefix      = "ENFORCEMENT_MODE_"
)

// Event is one record in the trail. Only one payload field is ever set; which
// one is fixed by the kind.
type Event struct {
	EventID         string    `json:"eventId,omitempty"`
	Kind            Kind      `json:"kind,omitempty"`
	RequestID       string    `json:"requestId,omitempty"`
	RunID           string    `json:"runId,omitempty"`
	ProjectID       string    `json:"projectId,omitempty"`
	TenantID        string    `json:"tenantId,omitempty"`
	OccurredAt      time.Time `json:"occurredAt,omitempty"`
	SchemaVersion   string    `json:"schemaVersion,omitempty"`
	EnforcementMode string    `json:"enforcementMode,omitempty"`
	ExecutionID     string    `json:"executionId,omitempty"`

	Proposed *ActionEnvelope  `json:"proposed,omitempty"`
	Decision *Decision        `json:"decision,omitempty"`
	Approval *Approval        `json:"approval,omitempty"`
	Result   *ActionResult    `json:"result,omitempty"`
	Finding  *Finding         `json:"finding,omitempty"`
	Policy   *PolicyBundleRef `json:"policy,omitempty"`

	PrevEventID string `json:"prevEventId,omitempty"`
}

// Obligation is a condition attached to an ALLOW_WITH_OBLIGATIONS verdict.
// Advisory is false by default, which means the receiver has to understand it.
type Obligation struct {
	Type     string            `json:"type,omitempty"`
	Params   map[string]string `json:"params,omitempty"`
	Advisory bool              `json:"advisory,omitempty"`
}

// Decision is the verdict a policy decision point returned, and what it was
// decided under.
type Decision struct {
	SchemaVersion      string       `json:"schemaVersion,omitempty"`
	DecisionID         string       `json:"decisionId,omitempty"`
	RequestID          string       `json:"requestId,omitempty"`
	ActionDigest       string       `json:"actionDigest,omitempty"`
	PolicyBundleDigest string       `json:"policyBundleDigest,omitempty"`
	PolicyRuleIDs      []string     `json:"policyRuleIds,omitempty"`
	Verdict            string       `json:"verdict,omitempty"`
	ReasonCodes        []string     `json:"reasonCodes,omitempty"`
	Obligations        []Obligation `json:"obligations,omitempty"`
	ExpiresAt          *time.Time   `json:"expiresAt,omitempty"`
	DecisionLatencyUs  Int64        `json:"decisionLatencyUs,omitempty"`
	PdpType            string       `json:"pdpType,omitempty"`
	PdpInstance        string       `json:"pdpInstance,omitempty"`
	PdpVersion         string       `json:"pdpVersion,omitempty"`
	EnforcementMode    string       `json:"enforcementMode,omitempty"`
	PolicyFreshness    string       `json:"policyFreshness,omitempty"`
	PolicyLoadedAt     *time.Time   `json:"policyLoadedAt,omitempty"`
}

// ShortVerdict is the verdict without the VERDICT_ prefix, which is how a
// scenario file names it. An empty verdict stays empty rather than becoming a
// word: nothing decided is not a decision.
func (d *Decision) ShortVerdict() string { return trimPrefix(d.GetVerdict(), verdictPrefix) }

// GetVerdict reads through a nil Decision, so a caller that found no decision
// event reads an empty verdict rather than crashing on the record it is
// reporting as absent.
func (d *Decision) GetVerdict() string {
	if d == nil {
		return ""
	}
	return d.Verdict
}

// Approval is a request for a person's answer, bound to one action digest.
type Approval struct {
	SchemaVersion      string     `json:"schemaVersion,omitempty"`
	ApprovalID         string     `json:"approvalId,omitempty"`
	RequestID          string     `json:"requestId,omitempty"`
	ActionDigest       string     `json:"actionDigest,omitempty"`
	PolicyBundleDigest string     `json:"policyBundleDigest,omitempty"`
	State              string     `json:"state,omitempty"`
	ApproverID         string     `json:"approverId,omitempty"`
	Reason             string     `json:"reason,omitempty"`
	RequestedAt        *time.Time `json:"requestedAt,omitempty"`
	DecidedAt          *time.Time `json:"decidedAt,omitempty"`
	ExpiresAt          *time.Time `json:"expiresAt,omitempty"`
	MultiUse           bool       `json:"multiUse,omitempty"`
}

// ActionResult is what running the action produced.
type ActionResult struct {
	SchemaVersion          string     `json:"schemaVersion,omitempty"`
	RequestID              string     `json:"requestId,omitempty"`
	ExecutionID            string     `json:"executionId,omitempty"`
	Status                 string     `json:"status,omitempty"`
	StartedAt              *time.Time `json:"startedAt,omitempty"`
	EndedAt                *time.Time `json:"endedAt,omitempty"`
	ToolProtocolStatus     string     `json:"toolProtocolStatus,omitempty"`
	ResultSchemaValid      bool       `json:"resultSchemaValid,omitempty"`
	ResultHash             string     `json:"resultHash,omitempty"`
	RedactedResultPreview  string     `json:"redactedResultPreview,omitempty"`
	Retryable              bool       `json:"retryable,omitempty"`
	SideEffectConfirmation string     `json:"sideEffectConfirmation,omitempty"`
	ExecutedActionDigest   string     `json:"executedActionDigest,omitempty"`
}

// Finding is what a detector reported after the fact. It annotates a trail that
// is already written and never grants or denies authority. Source says what
// produced it, so a model-derived finding cannot be read as a deterministic
// one.
type Finding struct {
	FindingID          string   `json:"findingId,omitempty"`
	RuleID             string   `json:"ruleId,omitempty"`
	RuleVersion        string   `json:"ruleVersion,omitempty"`
	Severity           string   `json:"severity,omitempty"`
	Verdict            string   `json:"verdict,omitempty"`
	RequestID          string   `json:"requestId,omitempty"`
	RunID              string   `json:"runId,omitempty"`
	EvidenceRefs       []string `json:"evidenceRefs,omitempty"`
	FrameworkMappings  []string `json:"frameworkMappings,omitempty"`
	RecommendedAction  string   `json:"recommendedAction,omitempty"`
	Source             string   `json:"source,omitempty"`
	ConfidencePermille *uint32  `json:"confidencePermille,omitempty"`
}

// PolicyBundleRef is what a decision points at. The content of a bundle never
// enters the trail, only its identity.
type PolicyBundleRef struct {
	BundleID  string     `json:"bundleId,omitempty"`
	Version   string     `json:"version,omitempty"`
	Digest    string     `json:"digest,omitempty"`
	CreatedAt *time.Time `json:"createdAt,omitempty"`
}

func trimPrefix(value, prefix string) string {
	if len(value) >= len(prefix) && value[:len(prefix)] == prefix {
		return value[len(prefix):]
	}
	return value
}
