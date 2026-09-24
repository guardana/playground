package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fakeEnv, set in a child's environment, turns this test binary into a stand-in
// for the enforcer's command: it prints the listing it is told to, answers with
// the exit status it is told to, and writes every call it receives to a log
// outside the approvals directory.
const fakeEnv = "APPROVER_FAKE_CONTROL"

func TestMain(m *testing.M) {
	if path := os.Getenv(fakeEnv); path != "" {
		os.Exit(fakeControl(path, os.Args[1:]))
	}
	os.Exit(m.Run())
}

// fakeConfig is what one fake command does. AnswerExit is keyed by approval id.
type fakeConfig struct {
	Listing    string         `json:"listing"`
	ListExit   int            `json:"list_exit"`
	AnswerExit map[string]int `json:"answer_exit"`
	Hang       bool           `json:"hang"`
	// HangAnswers hangs approve and reject and lets list through.
	HangAnswers bool   `json:"hang_answers"`
	Calls       string `json:"calls"`
}

func fakeControl(configPath string, args []string) int {
	body, err := os.ReadFile(configPath) // #nosec G304 -- written by the test that started this process.
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 97
	}
	var c fakeConfig
	if err := json.Unmarshal(body, &c); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 97
	}
	if err := appendCall(c.Calls, args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 97
	}
	listing := len(args) >= 2 && args[0] == "approvals" && args[1] == "list"
	if c.Hang || (c.HangAnswers && !listing) {
		time.Sleep(time.Minute)
	}
	if listing {
		listing, err := os.ReadFile(c.Listing)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 97
		}
		_, _ = os.Stdout.Write(listing)
		return c.ListExit
	}
	if len(args) == 0 {
		return 98
	}
	return c.AnswerExit[args[len(args)-1]]
}

func appendCall(path string, args []string) error {
	line, err := json.Marshal(args)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600) // #nosec G304 -- as above.
	if err != nil {
		return err
	}
	_, err = f.Write(append(line, '\n'))
	return errors.Join(err, f.Close())
}

// fakeLab is one approvals directory, the fake command's configuration and
// the calls log, all under one test's temporary directory.
type fakeLab struct {
	t       *testing.T
	root    string
	dir     string
	config  fakeConfig
	cfgPath string
}

func newFakeLab(t *testing.T, listingFixture string) *fakeLab {
	t.Helper()
	root := t.TempDir()
	f := &fakeLab{t: t, root: root, dir: filepath.Join(root, "approvals"), cfgPath: filepath.Join(root, "fake.json")}
	if err := os.Mkdir(f.dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.dir, "WAITING.0-held.rec"), []byte("held\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f.config = fakeConfig{Listing: filepath.Join(root, "listing.txt"), Calls: filepath.Join(root, "calls.jsonl"), AnswerExit: map[string]int{}}
	f.setListing(fixture(t, listingFixture, f.dir))
	f.save()
	return f
}

func (f *fakeLab) setListing(body string) {
	f.t.Helper()
	if err := os.WriteFile(f.config.Listing, []byte(body), 0o600); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fakeLab) save() {
	f.t.Helper()
	body, err := json.Marshal(f.config)
	if err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(f.cfgPath, body, 0o600); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fakeLab) commander(timeout time.Duration) commander {
	f.t.Helper()
	self, err := os.Executable()
	if err != nil {
		f.t.Fatal(err)
	}
	return commander{path: self, env: f.childEnv(), timeout: timeout}
}

// childEnv turns the child into the fake. The race detector otherwise holds
// every child that exits zero for a second before it exits.
func (f *fakeLab) childEnv() []string {
	return []string{fakeEnv + "=" + f.cfgPath, "GORACE=atexit_sleep_ms=0"}
}

// calls is every argument list the fake command received, in order.
func (f *fakeLab) calls() [][]string {
	f.t.Helper()
	body, err := os.ReadFile(f.config.Calls)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		f.t.Fatal(err)
	}
	var out [][]string
	for _, line := range splitLines(body) {
		var args []string
		if err := json.Unmarshal(line, &args); err != nil {
			f.t.Fatal(err)
		}
		out = append(out, args)
	}
	return out
}

func splitLines(body []byte) [][]byte {
	var out [][]byte
	start := 0
	for i, b := range body {
		if b == '\n' {
			out = append(out, body[start:i])
			start = i + 1
		}
	}
	return out
}
