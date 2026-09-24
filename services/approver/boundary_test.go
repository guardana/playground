package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// A listing the approver cannot read is an error, never "nothing waits".
func TestAListingItCannotUseIsAnError(t *testing.T) {
	cases := map[string]func(*fakeLab){
		"unparseable": func(f *fakeLab) { f.setListing("{not a listing}\n") },
		"incomplete":  func(f *fakeLab) { f.config.ListExit = 1; f.save() },
		"hung":        func(f *fakeLab) { f.config.Hang = true; f.save() },
		"no directory": func(f *fakeLab) {
			if err := os.RemoveAll(f.dir); err != nil {
				f.t.Fatal(err)
			}
		},
	}
	for name, breakIt := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, "mixed.txt", approveRefunds, 800*time.Millisecond)
			breakIt(h.lab)
			if err := h.tick(t, epoch()); !errors.Is(err, errListing) {
				t.Errorf("err = %v, want errListing", err)
			}
			if got := h.answers(t); len(got) != 0 {
				t.Errorf("answered %q from a listing it could not use", got)
			}
			expectLines(t, h.lines(t), nil)
		})
	}
}

// The enforcer's command is the only writer of the approvals directory.
func TestTheApproverWritesNothingUnderTheDirectory(t *testing.T) {
	h := newHarness(t, "mixed.txt", approveRefunds, 10*time.Second)
	before := snapshot(t, h.lab.dir)
	if err := h.tick(t, epoch()); err != nil {
		t.Fatal(err)
	}
	if len(h.answers(t)) == 0 {
		t.Fatal("nothing was answered, so this test examined nothing")
	}
	if after := snapshot(t, h.lab.dir); !reflect.DeepEqual(before, after) {
		t.Errorf("the directory changed: %v, then %v", before, after)
	}
}

func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		body, err := os.ReadFile(filepath.Join(dir, e.Name())) // #nosec G304 -- the test's own directory.
		if err != nil {
			t.Fatal(err)
		}
		out[e.Name()] = string(body)
	}
	return out
}
