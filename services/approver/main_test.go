package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func env(values map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) {
		v, ok := values[name]
		return v, ok
	}
}

func labEnv() map[string]string {
	return map[string]string{"LAB_RUN_ID": "run-7", "LAB_REPORTS_DIR": "/reports"}
}

func TestParseSettingsReadsFlagsAndTheLabEnvironment(t *testing.T) {
	s, err := parseSettings([]string{"-dir", "/run/approvals", "-script", "/lab/approver.yaml", "-interval", "750ms"}, env(labEnv()), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if s.control != "/enforcer/control" || s.interval != 750*time.Millisecond || s.execTimeout != 10*time.Second {
		t.Errorf("settings = %+v", s)
	}
	if got := s.journalPath(); got != "/reports/run-7/journals/approver.jsonl" {
		t.Errorf("journal path = %q", got)
	}
}

func TestParseSettingsRefusesAnIncompleteSetup(t *testing.T) {
	full := []string{"-dir", "/run/approvals", "-script", "/lab/a.yaml"}
	cases := map[string]struct {
		args []string
		env  map[string]string
	}{
		"no directory":              {[]string{"-script", "/lab/a.yaml"}, labEnv()},
		"no script":                 {[]string{"-dir", "/run/approvals"}, labEnv()},
		"no control":                {append([]string{"-control", " "}, full...), labEnv()},
		"a zero interval":           {append([]string{"-interval", "0s"}, full...), labEnv()},
		"a zero exec timeout":       {append([]string{"-exec-timeout", "0s"}, full...), labEnv()},
		"a stray argument":          {append(full, "extra"), labEnv()},
		"an unknown flag":           {append([]string{"-upstream", "x"}, full...), labEnv()},
		"no run id":                 {full, map[string]string{"LAB_REPORTS_DIR": "/reports"}},
		"no reports directory":      {full, map[string]string{"LAB_RUN_ID": "run-7"}},
		"a run id with a separator": {full, map[string]string{"LAB_RUN_ID": "run/7", "LAB_REPORTS_DIR": "/reports"}},
		"a run id climbing out":     {full, map[string]string{"LAB_RUN_ID": "..", "LAB_REPORTS_DIR": "/reports"}},
		"a run id with a backslash": {full, map[string]string{"LAB_RUN_ID": `run\7`, "LAB_REPORTS_DIR": "/reports"}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := parseSettings(c.args, env(c.env), io.Discard); !errors.Is(err, errInvalidConfig) {
				t.Errorf("err = %v, want errInvalidConfig", err)
			}
		})
	}
}

func TestHelpPrintsTheFlagsAndIsNotAFailure(t *testing.T) {
	var out bytes.Buffer
	_, err := parseSettings([]string{"-help"}, env(labEnv()), &out)
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("err = %v, want flag.ErrHelp", err)
	}
	for _, name := range []string{"-control", "-dir", "-script", "-interval", "-exec-timeout", "-listen"} {
		if !strings.Contains(out.String(), name) {
			t.Errorf("help does not name %s:\n%s", name, out.String())
		}
	}
}

// The health check is what compose waits on before a scenario starts, so it
// answers only on a listing that went through.
func TestHealthFollowsTheLastListing(t *testing.T) {
	h := newHarness(t, "mixed.txt", approveRefunds, 10*time.Second)
	var hl health
	status := func() int {
		rec := httptest.NewRecorder()
		hl.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
		return rec.Code
	}
	if got := status(); got != http.StatusServiceUnavailable {
		t.Errorf("before any listing: %d, want 503", got)
	}
	hl.set(h.tick(t, epoch()))
	if got := status(); got != http.StatusOK {
		t.Errorf("after a listing: %d, want 200", got)
	}
	h.lab.setListing("{not a listing}\n")
	hl.set(h.tick(t, epoch()))
	if got := status(); got != http.StatusServiceUnavailable {
		t.Errorf("after a listing it could not read: %d, want 503", got)
	}
}

// run end to end: the journal lands where the runner reads it, and the loop
// stops when its context does.
func TestRunJournalsUnderTheRunAndStops(t *testing.T) {
	lab := newFakeLab(t, "mixed.txt")
	got := runFor(t, lab, 3*time.Second)
	expectLines(t, got, [][3]string{
		{"leave", "served", "approval=PURGE unmatched"},
		{"approve", "served", "approval=WAITING exit=0"},
	})
}

// An answer the shutdown cut short is settled from one listing after the loop
// has stopped.
func TestRunSettlesAnAnswerCutShortAtShutdown(t *testing.T) {
	lab := newFakeLab(t, "mixed.txt")
	lab.config.HangAnswers = true
	lab.save()
	expectLines(t, runFor(t, lab, 2*time.Second), [][3]string{
		{"leave", "served", "approval=PURGE unmatched"},
		{"unknown", "served", "approval=WAITING answer=approve"},
		{"approve", "refused", "approval=WAITING record=APPROVAL_STATE_PENDING/pending"},
	})
}

// runFor runs the service against lab until the deadline and returns its
// journal.
func runFor(t *testing.T, lab *fakeLab, deadline time.Duration) [][3]string {
	t.Helper()
	return runIn(t, lab, deadline, t.TempDir())
}

// runIn runs the service with its reports under reports and returns its
// journal.
func runIn(t *testing.T, lab *fakeLab, deadline time.Duration, reports string) [][3]string {
	t.Helper()
	scriptPath := filepath.Join(t.TempDir(), "approver.yaml")
	if err := os.WriteFile(scriptPath, []byte(approveRefunds), 0o600); err != nil {
		t.Fatal(err)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, kv := range lab.childEnv() {
		name, value, _ := strings.Cut(kv, "=")
		t.Setenv(name, value)
	}
	args := []string{"-control", self, "-dir", lab.dir, "-script", scriptPath, "-listen", "127.0.0.1:0", "-interval", "200ms", "-exec-timeout", "30s"}
	ctx, cancel := context.WithTimeout(context.Background(), deadline)
	defer cancel()
	err = run(ctx, args, env(map[string]string{"LAB_RUN_ID": "run-7", "LAB_REPORTS_DIR": reports}), io.Discard, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{lab: lab, journal: filepath.Join(reports, "run-7", "journals", "approver.jsonl")}
	return h.lines(t)
}
