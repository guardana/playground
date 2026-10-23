// Package record is the neutral record of one tool run: what the tool said it
// ran, what it found, what it could not settle and what it could not run. A
// grader reads this record and never one tool's own report format.
package record

// List is one list a tool run reports. Reported is true only when the tool's
// output named the list: an empty list it named says "none", and a list it left
// out says nothing.
type List[T any] struct {
	Items    []T
	Reported bool
}

// Named is a list the tool's output named, with the items it gave.
func Named[T any](items []T) List[T] {
	return List[T]{Items: items, Reported: true}
}

// Finding is one result a rule reported.
type Finding struct {
	Rule     string
	Severity string
	// Target is what the finding is about, exactly as the tool named it.
	Target  string
	Summary string
}

// Gap is coverage the run needed and did not get: its kind and what it
// concerned.
type Gap struct {
	Kind string
	Name string
}

// Run is one tool run as its output recorded it.
type Run struct {
	// Target is what the run was about, as the tool named it.
	Target string
	// Verdict is what the tool decided about its own run; nil when it recorded
	// none.
	Verdict  *string
	RulesRun List[string]
	Findings List[Finding]
	// Waived holds findings a waiver accepted; Unverified holds results the
	// rule could not settle.
	Waived     List[Finding]
	Unverified List[Finding]
	// Errors names the source of each check that could not run.
	Errors   List[string]
	Coverage List[Gap]
	// Scope is the files the run listed.
	Scope List[string]
}

// Approval is what a tool approved for a later run to compare against: the
// server it approved and how many of that server's tools.
type Approval struct {
	Server string
	Tools  int
}
