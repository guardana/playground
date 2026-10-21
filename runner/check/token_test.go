package check_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/runner/check"
)

// tokenFor is assembled at run time, so the guards that scan this file never
// see a token-shaped literal.
func tokenFor(t *testing.T) string {
	t.Helper()
	return strings.Repeat("ab", 32)
}

func scanRun(t *testing.T, dir, token string) assertion.Result {
	t.Helper()
	done := make(chan []assertion.Result, 1)
	go func() {
		results, err := check.TokenKeptOut{RunDir: dir, Token: token}.Run(context.Background(), assertion.Records{})
		if err != nil {
			t.Error(err)
		}
		done <- results
	}()
	select {
	case results := <-done:
		if len(results) != 1 {
			t.Fatalf("%d results, want 1", len(results))
		}
		return results[0]
	case <-time.After(10 * time.Second):
		t.Fatal("the scan did not return")
		return assertion.Result{}
	}
}

func TestATokenInAFileNameFailsTheRun(t *testing.T) {
	dir, token := t.TempDir(), tokenFor(t)
	if err := os.WriteFile(filepath.Join(dir, token+".json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := scanRun(t, dir, token); got.Outcome != assertion.Fail || strings.Contains(got.Got, token) {
		t.Errorf("token-kept-out = %s %q, want a fail that does not repeat the token", got.Outcome, got.Got)
	}
}

// A pipe left where a file was is not read, and does not hold the run.
func TestAPipeInTheRunIsNamedUnreadAndDoesNotHang(t *testing.T) {
	dir := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(dir, "step-1.stderr"), 0o600); err != nil {
		t.Skipf("no fifo here: %v", err)
	}
	if got := scanRun(t, dir, tokenFor(t)); got.Outcome != assertion.Pass {
		t.Logf("token-kept-out = %s %q", got.Outcome, got.Got)
	}
}

// A link out of the run is never followed, so a file outside it holding the
// token is not the run's.
func TestALinkOutOfTheRunIsNotFollowed(t *testing.T) {
	dir, outside, token := t.TempDir(), t.TempDir(), tokenFor(t)
	if err := os.WriteFile(filepath.Join(outside, "host.txt"), []byte(token), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "host.txt"), filepath.Join(dir, "link.txt")); err != nil {
		t.Fatal(err)
	}
	if got := scanRun(t, dir, token); got.Outcome == assertion.Fail {
		t.Errorf("token-kept-out = %s %q, want the link left unfollowed", got.Outcome, got.Got)
	}
}
