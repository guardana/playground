package impact

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// Commit is one commit of the log, with the paths it touched.
type Commit struct {
	Hash    string
	Subject string
	Files   []string
}

// Staleness is what the log says about one page: the commit that last
// touched it and every later commit touching a path under its covers.
type Staleness struct {
	Page        string
	LastCommit  string
	Since       []Commit
	Uncommitted bool
}

// logArgs asks for each commit as one NUL-prefixed header line, so a header
// is never read as a path, then the paths it touched. A merge lists what it
// brought against its first parent, a rename lists its old and its new path,
// and no signature is printed.
var logArgs = []string{"log", "--format=%x00%h %s", "--name-only", "--no-renames", "--diff-merges=first-parent", "--no-show-signature"}

// Stale reads the whole log and judges every page against it. A shallow
// clone holds part of the log, so every count would be wrong: not measured.
func Stale(ctx context.Context, run Runner, pages []Page) ([]Staleness, error) {
	ps, err := compilePages(pages)
	if err != nil {
		return nil, err
	}
	out, err := run(ctx, "rev-parse", "--is-shallow-repository")
	if err != nil {
		return nil, notMeasured("git rev-parse", err)
	}
	if answer := strings.TrimSpace(string(out)); answer != "false" {
		return nil, fmt.Errorf("%w: git says the clone is shallow or cannot say: %q", ErrNotMeasured, answer)
	}
	out, err = run(ctx, logArgs...)
	if err != nil {
		return nil, notMeasured("git log", err)
	}
	commits, err := ParseLog(out)
	if err != nil {
		return nil, err
	}
	rows := make([]Staleness, 0, len(ps))
	for _, p := range ps {
		last := slices.IndexFunc(commits, func(c Commit) bool { return slices.Contains(c.Files, p.path) })
		if last < 0 {
			rows = append(rows, Staleness{Page: p.path, Uncommitted: true})
			continue
		}
		row := Staleness{Page: p.path, LastCommit: commits[last].Hash}
		for _, c := range commits[:last] {
			if len(matching(p.covers, c.Files)) > 0 {
				row.Since = append(row.Since, c)
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// FormatStale spells the rows with the count behind them.
func FormatStale(rows []Staleness) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d pages examined\n", len(rows))
	for _, r := range rows {
		if r.Uncommitted {
			fmt.Fprintf(&b, "  %s: not committed yet\n", r.Page)
			continue
		}
		fmt.Fprintf(&b, "  %s: last commit %s, %d commits since touching its covers\n", r.Page, r.LastCommit, len(r.Since))
		for _, c := range r.Since {
			fmt.Fprintf(&b, "    %s %s\n", c.Hash, c.Subject)
		}
	}
	return b.String()
}

// ParseLog reads the output of logArgs: a header line per commit, then the
// paths it touched, blank lines between.
func ParseLog(out []byte) ([]Commit, error) {
	if strings.Contains(string(out), "\r") {
		return nil, fmt.Errorf("%w: git's output holds a carriage return", ErrInvalid)
	}
	var commits []Commit
	for i, line := range strings.Split(strings.TrimSuffix(string(out), "\n"), "\n") {
		switch {
		case line == "":
			continue
		case strings.HasPrefix(line, "\x00"):
			hash, subject, _ := strings.Cut(line[1:], " ")
			if hash == "" || slices.ContainsFunc(commits, func(c Commit) bool { return c.Hash == hash }) {
				return nil, fmt.Errorf("%w: line %d: a commit header with no hash or one seen before", ErrInvalid, i+1)
			}
			commits = append(commits, Commit{Hash: hash, Subject: subject})
		case len(commits) == 0:
			return nil, fmt.Errorf("%w: line %d: a path before any commit header", ErrInvalid, i+1)
		default:
			if err := checkGitPath(line); err != nil {
				return nil, fmt.Errorf("%w: line %d: %w", ErrInvalid, i+1, err)
			}
			last := &commits[len(commits)-1]
			last.Files = append(last.Files, line)
		}
	}
	if len(commits) == 0 {
		return nil, errors.Join(ErrInvalid, errors.New("the log holds no commit"))
	}
	return commits, nil
}
