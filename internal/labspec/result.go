package labspec

import "fmt"

// ResultExpectation is what the closing record of a step's trail, its
// ACTION_COMPLETED or ACTION_FAILED, has to carry: the result status, written
// without the RESULT_STATUS_ prefix the wire carries. The kind alone does not
// say why a call failed; the status tells an aborted call from one an upstream
// failed or that ran out of time.
type ResultExpectation struct {
	Status string `json:"status"`
}

// resultStatuses are the statuses the wire contract declares, unspecified left
// out because no record states it.
func resultStatuses() []string {
	return []string{"SUCCESS", "FAILURE", "TIMEOUT", "CANCELLED", "BLOCKED", "UNKNOWN"}
}

// validateResult holds a stated result to the trail it is read from: the
// trail is stated and closes, ACTION_COMPLETED with a success and
// ACTION_FAILED with anything else, and a blocked trail never closes.
func (d DecisionExpectation) validateResult(field string) error {
	if d.Result == nil {
		return nil
	}
	if err := oneOf(field+".result.status", d.Result.Status, resultStatuses()...); err != nil {
		return err
	}
	closing := "ACTION_FAILED"
	if d.Result.Status == "SUCCESS" {
		closing = "ACTION_COMPLETED"
	}
	switch {
	case d.Blocked != nil:
		return fmt.Errorf("%w: %s states a block and a result; a blocked trail is never closed by a result", ErrInvalid, field)
	case len(d.Trail) == 0:
		return fmt.Errorf("%w: %s.result is read from the closing record, so the step states the trail it ends", ErrInvalid, field)
	case d.Trail[len(d.Trail)-1] != closing:
		return fmt.Errorf("%w: %s.result %s closes a trail with %s, and the trail list ends with %s",
			ErrInvalid, field, d.Result.Status, closing, d.Trail[len(d.Trail)-1])
	}
	return nil
}
