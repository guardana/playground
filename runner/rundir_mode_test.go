package main

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func modeOf(t *testing.T, path string) fs.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	return info.Mode() & (fs.ModePerm | fs.ModeSticky)
}

// Only the runner writes into the run directory and its journals directory; the
// services, as uid 65532, write into the subdirectories below them, where the
// sticky bit keeps another local user from removing or replacing a record.
func TestTheRunDirectoryIsTheRunnersAndItsSharedDirectoriesAreSticky(t *testing.T) {
	old := syscall.Umask(0o077)
	defer syscall.Umask(old)
	reports := filepath.Join(t.TempDir(), "reports")
	runDir := filepath.Join(reports, "flow-01-20260909T120000Z-a1b2c3d4")
	if err := makeRunDir(reports, runDir); err != nil {
		t.Fatalf("makeRunDir: %v", err)
	}
	if err := makeShared(filepath.Join(runDir, "collector")); err != nil {
		t.Fatalf("makeShared: %v", err)
	}
	want := map[string]fs.FileMode{
		reports:                            0o755,
		runDir:                             0o755,
		filepath.Join(runDir, "journals"):  0o755,
		filepath.Join(runDir, "agent"):     fs.ModeSticky | 0o777,
		filepath.Join(runDir, "collector"): fs.ModeSticky | 0o777,
	}
	for _, writer := range journalWriterNames() {
		want[filepath.Join(runDir, "journals", writer)] = fs.ModeSticky | 0o777
	}
	for path, mode := range want {
		if got := modeOf(t, path); got != mode {
			t.Errorf("%s is %v, want %v", filepath.Base(path), got, mode)
		}
	}
}

// journalWriterNames is every service that writes a journal, spelled out
// rather than read from the runner, which builds the directories from it.
func journalWriterNames() []string {
	return []string{
		"victim-crm", "victim-db", "victim-fs", "victim-shell", "victim-mail", "victim-web", "victim-pay",
		"pdp-double", "approver",
	}
}

// Every directory an enforcer run hands to a service is sticky, and the run
// directory above them is not writable by any of them.
func TestAnEnforcerRunsDirectoriesHaveTheirModes(t *testing.T) {
	subject, compose, scenario := enforcerLab(t)
	if _, err := subject.execute(context.Background(), scenario); err != nil {
		t.Fatalf("execute: %v", err)
	}
	runDir := compose.env["LAB_RUN_HOST_DIR"]
	if got := modeOf(t, runDir); got != 0o755 {
		t.Errorf("the run directory is %v, want -rwxr-xr-x", got)
	}
	if got := modeOf(t, filepath.Join(runDir, "journals")); got != 0o755 {
		t.Errorf("journals is %v, want -rwxr-xr-x", got)
	}
	shared := []string{"agent", "collector", "pki"}
	for _, writer := range journalWriterNames() {
		shared = append(shared, filepath.Join("journals", writer))
	}
	for _, dir := range shared {
		if got := modeOf(t, filepath.Join(runDir, dir)); got != fs.ModeSticky|0o777 {
			t.Errorf("%s is %v, want trwxrwxrwx", dir, got)
		}
	}
}

// A reports directory that was already there keeps the mode its owner gave it.
func TestAnExistingReportsDirectoryKeepsItsMode(t *testing.T) {
	reports := filepath.Join(t.TempDir(), "reports")
	if err := os.Mkdir(reports, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := makeRunDir(reports, filepath.Join(reports, "run")); err != nil {
		t.Fatalf("makeRunDir: %v", err)
	}
	if got := modeOf(t, reports); got != 0o700 {
		t.Errorf("an existing reports directory is %v after a run, want it left at -rwx------", got)
	}
}

func TestTheRunnerRefusesToRunAsTheServicesUID(t *testing.T) {
	err := refuseServiceUID(65532)
	if err == nil {
		t.Fatal("a runner with the services' uid was let run")
	}
	if !strings.Contains(err.Error(), "65532") {
		t.Errorf("the refusal is %v, want it to name the uid", err)
	}
	for _, uid := range []int{0, 501, 1000, 65533} {
		if err := refuseServiceUID(uid); err != nil {
			t.Errorf("uid %d was refused: %v", uid, err)
		}
	}
}
