package check

import (
	"fmt"
	"slices"
	"strings"

	"github.com/guardana/playground/internal/labspec"
	"github.com/guardana/playground/internal/record"
)

// gradeVerdict adds the checks on what the verifier decided about its own run
// and why: the gate it recorded, every coverage gap and every rule that could
// not run, each list exact.
func gradeVerdict(want labspec.VerifierExpectation, ran record.Run, add func(string, string, func() (bool, string))) {
	if want.Gate != "" {
		add("gate", "gate "+want.Gate, func() (bool, string) { return gateIs(ran, want.Gate) })
	}
	if want.Shortfall != nil {
		add("shortfall", describeShortfall(*want.Shortfall), func() (bool, string) { return shortfallIs(ran, *want.Shortfall) })
	}
	if want.Errors != nil {
		add("errors", describeList("errors from", *want.Errors), func() (bool, string) { return errorsAre(ran, *want.Errors) })
	}
}

func gateIs(ran record.Run, want string) (bool, string) {
	gate := ran.Verdict
	if gate == nil {
		return false, "the report records no gate"
	}
	return *gate == want, "gate " + *gate
}

// shortfallIs matches the gaps one to one. A named expectation claims its gap
// before a nameless one of the same kind takes any, so the order they are
// written in never decides the match.
func shortfallIs(ran record.Run, want []labspec.ShortfallExpectation) (bool, string) {
	if !ran.Coverage.Reported {
		return false, "the report records no coverage.shortfall"
	}
	got := ran.Coverage.Items
	seen := make([]string, 0, len(got))
	for _, gap := range got {
		seen = append(seen, gap.Kind+" "+gap.Name)
	}
	described := describeList("", seen)
	if len(got) != len(want) {
		return false, described
	}
	ordered := slices.Clone(want)
	slices.SortStableFunc(ordered, func(a, b labspec.ShortfallExpectation) int {
		return strings.Compare(b.Name, a.Name)
	})
	taken := make([]bool, len(got))
	for _, wanted := range ordered {
		index := untaken(got, taken, wanted)
		if index < 0 {
			return false, described
		}
		taken[index] = true
	}
	return true, described
}

// untaken is the first gap the expectation describes that no other has taken,
// or -1.
func untaken(got []record.Gap, taken []bool, wanted labspec.ShortfallExpectation) int {
	for i, gap := range got {
		if !taken[i] && gap.Kind == wanted.Kind && (wanted.Name == "" || gap.Name == wanted.Name) {
			return i
		}
	}
	return -1
}

func errorsAre(ran record.Run, want []string) (bool, string) {
	if !ran.Errors.Reported {
		return false, "the report records no errors list"
	}
	got := slices.Sorted(slices.Values(ran.Errors.Items))
	sorted := slices.Sorted(slices.Values(want))
	return slices.Equal(got, sorted), describeList("errors from", got)
}

func describeShortfall(gaps []labspec.ShortfallExpectation) string {
	named := make([]string, 0, len(gaps))
	for _, gap := range gaps {
		named = append(named, strings.TrimSpace(gap.Kind+" "+gap.Name))
	}
	return describeList("exactly", named)
}

func describeList(lead string, items []string) string {
	if len(items) == 0 {
		return strings.TrimSpace(lead + " none")
	}
	return strings.TrimSpace(fmt.Sprintf("%s %s", lead, strings.Join(items, "; ")))
}
