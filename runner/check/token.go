package check

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
	"syscall"

	"github.com/guardana/playground/internal/assertion"
)

// maxScannedBytes bounds one file the token is looked for in; a larger one is
// named as unscanned rather than read into memory.
const maxScannedBytes = 64 << 20

// TokenKeptOut fails a run any of whose files holds the run's bearer token. The
// token reaches a container by name alone, so a copy of it in a journal, a
// report or what a container printed is a leak, whoever wrote it. A file that
// could not be read is not a file shown clean.
type TokenKeptOut struct {
	RunDir string
	Token  string
}

// ID names the check in a report.
func (TokenKeptOut) ID() string { return "token" }

// Run reads every regular file under the run directory once, and every name.
func (k TokenKeptOut) Run(ctx context.Context, _ assertion.Records) ([]assertion.Result, error) {
	result := assertion.Result{
		Check: VerifierPrefix + "token-kept-out", Want: "no file of the run holds the run's token, in its name or its bytes",
		Source: k.RunDir,
	}
	if k.Token == "" {
		result.Outcome, result.Got = assertion.Indeterminate, "the run had no token to look for"
		return []assertion.Result{result}, nil
	}
	holding, unscanned := k.scan(ctx)
	switch {
	case len(holding) > 0:
		result.Outcome, result.Got = assertion.Fail, "the token is in "+strings.Join(holding, ", ")
		result.Detail = result.Got
	case len(unscanned) > 0:
		result.Outcome, result.Got = assertion.Indeterminate, "not read: "+strings.Join(unscanned, "; ")
	default:
		result.Outcome, result.Got = assertion.Pass, "in no file of the run"
	}
	return []assertion.Result{result}, nil
}

// scan walks the run directory through an os.Root, so a link a container left
// there can neither lead the walk nor a read outside the run.
func (k TokenKeptOut) scan(ctx context.Context) (holding, unscanned []string) {
	root, err := os.OpenRoot(k.RunDir)
	if err != nil {
		return nil, []string{err.Error()}
	}
	defer func() { _ = root.Close() }()
	token := []byte(k.Token)
	walked := fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, err error) error {
		switch {
		case ctx.Err() != nil:
			return ctx.Err()
		case err != nil:
			unscanned = append(unscanned, fmt.Sprintf("%s (%v)", name, err))
			return fs.SkipDir
		case strings.Contains(name, k.Token):
			holding = append(holding, "a name under the run directory")
			return nil
		case !entry.Type().IsRegular():
			return nil
		}
		switch found, readable := holds(root, name, token); {
		case !readable:
			unscanned = append(unscanned, name)
		case found:
			holding = append(holding, name)
		}
		return nil
	})
	if walked != nil {
		unscanned = append(unscanned, walked.Error())
	}
	return holding, unscanned
}

// holds reports whether the file holds the token, and whether it could be read
// whole within the bound. It opens without following a link and without
// blocking, so a pipe swapped in for a file a container still writes cannot
// hold the run, and reads only what is a regular file once open.
func holds(root *os.Root, name string, token []byte) (found, readable bool) {
	file, err := root.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return false, false
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxScannedBytes {
		return false, false
	}
	body, err := io.ReadAll(io.LimitReader(file, maxScannedBytes+1))
	if err != nil || len(body) > maxScannedBytes {
		return false, false
	}
	return bytes.Contains(body, token), true
}
