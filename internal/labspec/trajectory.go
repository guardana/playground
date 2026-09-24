package labspec

import (
	"fmt"
	"regexp"
	"strconv"
)

// Trajectory is one scripted run: who it runs as, and the tool calls it makes,
// in order. There is no expectation in this file. What a run should produce is
// stated once, in the scenario, so that two files can never disagree about it.
type Trajectory struct {
	SchemaVersion int       `json:"schema_version"`
	Agent         Agent     `json:"agent"`
	Principal     Principal `json:"principal"`
	Session       Session   `json:"session"`
	Steps         []Step    `json:"steps"`
}

// Agent is the caller's own description of itself. The lab supplies it; nothing
// verifies it, which is the point: an agent's self-description is an input to
// the system under test, not a fact about the run.
type Agent struct {
	ID        string `json:"id"`
	Framework string `json:"framework"`
	ModelRef  string `json:"model_ref"`
}

// Principal is the identity the calls are made on behalf of. TenantID is the
// value a cross-tenant check compares against the resource's owner, so a
// scenario about tenancy changes this and nothing else.
type Principal struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	TenantID string `json:"tenant_id"`
}

// Session holds what the whole run shares. The run identifier is not here: it
// is minted per run by the runner, because a trajectory replayed twice is two
// runs and a file cannot hold the identifier of both.
type Session struct {
	Environment string `json:"environment"`
}

// Step is one tool call. Label names what the step is for in the failure
// message and in a scenario written about it; it carries no meaning to the
// systems under test.
type Step struct {
	Label string `json:"label,omitempty"`
	Call  Call   `json:"call"`
}

// Call is the tool call itself, addressed to a server by the name compose gives
// it. The agent sends it to the gateway, never to the server directly.
type Call struct {
	Server string         `json:"server"`
	Tool   string         `json:"tool"`
	Args   map[string]any `json:"args,omitempty"`
}

// stepReference matches one ${step[n].output} inside an argument string. The
// step number is 1-based; see the package comment for why there is only one
// numbering.
var stepReference = regexp.MustCompile(`\$\{step\[([0-9]+)\]\.output\}`)

func (t Trajectory) validate() error {
	if t.SchemaVersion != SchemaVersion {
		return fmt.Errorf("%w: schema_version is %d, want %d", ErrInvalid, t.SchemaVersion, SchemaVersion)
	}
	if err := first(
		required("agent.id", t.Agent.ID),
		required("agent.framework", t.Agent.Framework),
		required("principal.id", t.Principal.ID),
		required("principal.tenant_id", t.Principal.TenantID),
		required("session.environment", t.Session.Environment),
	); err != nil {
		return err
	}
	if len(t.Steps) == 0 {
		// A trajectory with no calls makes no calls to assert on, and a run
		// over one would report that everything it looked at was fine.
		return fmt.Errorf("%w: steps is empty", ErrInvalid)
	}
	for i, step := range t.Steps {
		if err := step.validate(i + 1); err != nil {
			return err
		}
	}
	return nil
}

func (s Step) validate(number int) error {
	if err := first(
		required(fmt.Sprintf("steps[%d].call.server", number), s.Call.Server),
		required(fmt.Sprintf("steps[%d].call.tool", number), s.Call.Tool),
	); err != nil {
		return err
	}
	for _, reference := range s.References() {
		if reference.Step < 1 || reference.Step >= number {
			return fmt.Errorf(
				"%w: steps[%d] reads step %d, which is not a step that has already run",
				ErrInvalid, number, reference.Step)
		}
	}
	return nil
}

// Reference is one ${step[n].output} found in a step's arguments, with the
// argument key it was found under so a failure names the field.
type Reference struct {
	Step int
	Key  string
}

// References returns every step reference in the step's arguments. The order is
// the order the keys sort in, so two runs over one file report the same first
// problem.
func (s Step) References() []Reference {
	var found []Reference
	for _, key := range sortedKeys(s.Call.Args) {
		text, ok := s.Call.Args[key].(string)
		if !ok {
			continue
		}
		for _, match := range stepReference.FindAllStringSubmatch(text, -1) {
			// The pattern admits digits only, so the only error a well formed
			// match can carry is a number too large to hold, which fails the
			// range check below as 0 does.
			number, _ := strconv.Atoi(match[1])
			found = append(found, Reference{Step: number, Key: key})
		}
	}
	return found
}

// Resolve returns the step's arguments with every ${step[n].output} replaced by
// the output of step n. The loaded step is left alone, so replaying a
// trajectory twice produces the same calls.
//
// An output that is absent or empty is refused. A flow whose payload carries
// nothing is not the flow the scenario says it is testing, and substituting an
// empty string would let such a run reach its assertions and pass them.
func (s Step) Resolve(outputs map[int]string) (map[string]any, error) {
	resolved := make(map[string]any, len(s.Call.Args))
	for key, value := range s.Call.Args {
		text, ok := value.(string)
		if !ok {
			resolved[key] = value
			continue
		}
		var failure error
		text = stepReference.ReplaceAllStringFunc(text, func(match string) string {
			number, _ := strconv.Atoi(stepReference.FindStringSubmatch(match)[1])
			output, present := outputs[number]
			switch {
			case !present:
				failure = fmt.Errorf("%w: %s reads step %d, which produced no output",
					ErrInvalid, key, number)
			case output == "":
				failure = fmt.Errorf("%w: %s reads step %d, whose output is empty",
					ErrInvalid, key, number)
			}
			return output
		})
		if failure != nil {
			return nil, failure
		}
		resolved[key] = text
	}
	return resolved, nil
}
