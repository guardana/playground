package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/guardana/playground/internal/journal"
)

// serverName is stamped on every journal line and names the journal file.
const serverName = "approver"

// sighting is what the approver knows of one waiting approval: when a listing
// first showed it, whether it has been dealt with, and the rule whose answer
// was cut short until a listing shows what the record became.
type sighting struct {
	first  time.Time
	done   bool
	unsure *rule
}

// approver answers the approvals one directory holds, through the enforcer's
// command, as a script says. It is driven by one goroutine, so seen needs no
// lock.
type approver struct {
	dir     string
	cmd     commander
	script  *script
	journal *journal.Writer
	runID   string
	logger  *slog.Logger
	seen    map[string]*sighting
}

func newApprover(dir string, cmd commander, s *script, w *journal.Writer, runID string, logger *slog.Logger) *approver {
	return &approver{dir: dir, cmd: cmd, script: s, journal: w, runID: runID, logger: logger, seen: map[string]*sighting{}}
}

// tick lists the directory once and deals with every waiting approval that is
// due. A listing it cannot use is an error and nothing is answered from it; a
// directory no plane holds is errNoPlane, after its approvals are journalled.
func (a *approver) tick(ctx context.Context, now time.Time) error {
	l, err := a.list(ctx)
	if err != nil {
		return err
	}
	if err := a.act(ctx, l, now); err != nil {
		return err
	}
	if !l.planeHolds {
		return errNoPlane
	}
	return nil
}

func (a *approver) list(ctx context.Context) (listing, error) {
	info, err := os.Stat(a.dir) // #nosec G703 -- the directory the operator named, only examined here.
	if err != nil {
		return listing{}, fmt.Errorf("%w: %w", errListing, err)
	}
	if !info.IsDir() {
		return listing{}, fmt.Errorf("%w: %s is not a directory", errListing, a.dir)
	}
	o := a.cmd.run(ctx, "approvals", "list", a.dir)
	if o.err != nil {
		return listing{}, fmt.Errorf("%w: approvals list: %w: %s", errListing, o.err, firstLine(o.stderr))
	}
	if !o.exited || o.code != 0 {
		return listing{}, fmt.Errorf("%w: approvals list exited %s: %s", errListing, o.status(), firstLine(o.stderr))
	}
	if len(o.stderr) > 0 {
		a.logger.Warn("approvals list reported problems", "stderr", string(o.stderr))
	}
	return parseListing(o.stdout, a.dir)
}

// act settles what earlier answers cut short became, then deals with each
// waiting approval in listing order, and answers nothing once ctx is done.
func (a *approver) act(ctx context.Context, l listing, now time.Time) error {
	if err := a.followUp(l); err != nil {
		return err
	}
	for _, e := range l.entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !e.waiting() {
			continue
		}
		s, known := a.seen[e.id]
		if !known {
			s = &sighting{first: now}
			a.seen[e.id] = s
		}
		if s.done {
			continue
		}
		if err := a.deal(ctx, l.planeHolds, e, s, !known, now); err != nil {
			return err
		}
	}
	return nil
}

// deal does what the script says with one waiting approval. Each is dealt
// with once: an answer the command refused is journalled and not retried,
// because the refusal is the enforcer's answer and a retry would bury it.
func (a *approver) deal(ctx context.Context, planeHolds bool, e entry, s *sighting, first bool, now time.Time) error {
	if !planeHolds {
		// The command writes an answer and exits zero with no plane, and the
		// call it was held for will not run whatever the record says.
		s.done = true
		return a.record(toolNoPlane, journal.Served, "approval="+e.id)
	}
	r, number := a.script.ruleFor(e)
	switch {
	case r == nil:
		s.done = true
		detail := "approval=" + e.id + " unmatched"
		if !e.readable {
			detail += " unreadable"
		}
		return a.record(string(answerLeave), journal.Served, detail)
	case r.Answer == answerLeave:
		s.done = true
		return a.record(string(answerLeave), journal.Served, "approval="+e.id+" rule="+strconv.Itoa(number))
	case now.Before(s.first.Add(r.after)):
		if !first {
			return nil
		}
		return a.record(toolWait, journal.Served, "approval="+e.id+" rule="+strconv.Itoa(number)+" delay="+r.Delay)
	}
	s.done = true
	return a.answer(ctx, e.id, r, s)
}

// answer runs approve or reject for one approval and journals what the
// command made of it. A command that did not run to its own exit may have
// written the answer, so its outcome is unknown until a listing shows it.
func (a *approver) answer(ctx context.Context, id string, r *rule, s *sighting) error {
	args := []string{"approvals", string(r.Answer), "--approver-id", r.ApproverID}
	if r.Reason != "" {
		args = append(args, "--reason", r.Reason)
	}
	o := a.cmd.run(ctx, append(args, a.dir, id)...)
	if o.cutShort {
		s.unsure = r
		a.logger.Warn("an answer was cut short", "approval", id, "answer", r.Answer, "error", o.err)
		return a.record(toolUnknown, journal.Served, "approval="+id+" answer="+string(r.Answer))
	}
	status := journal.Served
	if o.err != nil || !o.exited || o.code != 0 {
		status = journal.Refused
		a.logger.Warn("the command refused an answer", "approval", id, "answer", r.Answer,
			"exit", o.status(), "error", o.err, "stderr", string(o.stderr))
	}
	return a.record(string(r.Answer), status, "approval="+id+" exit="+o.status())
}

func (a *approver) record(tool string, status journal.Status, detail string) error {
	return a.journal.Record(journal.Entry{
		OccurredAt: time.Now().UTC(), Tool: tool, RunID: a.runID, Status: status, Detail: detail,
	})
}

func firstLine(b []byte) string {
	for i, c := range b {
		if c == '\n' {
			return string(b[:i])
		}
	}
	return string(b)
}
