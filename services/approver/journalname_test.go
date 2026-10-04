package main

import (
	"testing"

	"github.com/guardana/playground/internal/labspec"
)

// The runner reads this double's journal from journals/<name>/<name>.jsonl
// under the name labspec gives it; a file of another name is never graded.
func TestTheJournalIsNamedAsTheRunnerReadsIt(t *testing.T) {
	if serverName != labspec.ApproverJournal {
		t.Errorf("the journal is %s.jsonl, and the runner reads %s.jsonl", serverName, labspec.ApproverJournal)
	}
}
