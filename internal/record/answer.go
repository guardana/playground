package record

// GateAnswer is what a gate in front of the tools answered one call, read by
// the gate's own driver and handed on so nothing else reads the gate's marks.
type GateAnswer int

const (
	// AnswerBlocked is the gate refusing the call for any reason but an expired
	// approval. It is the zero value, so an answer nobody classified reads as a
	// refusal rather than as a call that went through.
	AnswerBlocked GateAnswer = iota
	// AnswerPassed is the gate letting the call through: whatever the result
	// says, the gate did not make that answer itself.
	AnswerPassed
	// AnswerHeld is the gate holding the call until an approval is given.
	AnswerHeld
	// AnswerApprovalExpired is the gate refusing the call because the approval
	// it sought expired.
	AnswerApprovalExpired
)
