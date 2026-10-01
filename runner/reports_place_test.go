package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// cloneWithMounts is a clone holding the directories a container mounts or an
// image is built from, and a workspace outside it.
func cloneWithMounts(t *testing.T) (root, outside string) {
	t.Helper()
	root, outside = t.TempDir(), t.TempDir()
	for _, dir := range []string{"attacks", "config/pdp", "trajectories"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	return root, outside
}

// Inside the clone only reports/ is out of every mount and out of the build
// context; a run written anywhere else there sits in a container's view or in
// an image, with the collector's key among it.
func TestReportsInsideTheCloneButOutsideItsReportsAreRefused(t *testing.T) {
	root, outside := cloneWithMounts(t)
	for _, place := range []string{"attacks/runs", "runs", "config/pdp/x"} {
		for mode, environ := range map[string][]string{
			"unset": {"HOME=/nowhere"},
			"set":   {"LAB_WORKSPACE=" + outside},
		} {
			t.Run(place+" with LAB_WORKSPACE "+mode, func(t *testing.T) {
				_, err := openWorkspace(root, filepath.Join(root, place), environ)
				if err == nil {
					t.Fatalf("reports at <clone>/%s were taken", place)
				}
				if !strings.Contains(err.Error(), "reports/") {
					t.Errorf("the refusal is %v, want it to name the clone's reports/", err)
				}
			})
		}
	}
}

func TestReportsUnderTheClonesReportsOrOutsideTheCloneAreTaken(t *testing.T) {
	root, outside := cloneWithMounts(t)
	elsewhere := filepath.Join(t.TempDir(), "runs")
	for _, reports := range []string{filepath.Join(root, "reports"), filepath.Join(root, "reports", "x"), elsewhere} {
		for mode, environ := range map[string][]string{
			"unset": {"HOME=/nowhere"},
			"set":   {"LAB_WORKSPACE=" + outside},
		} {
			if _, err := openWorkspace(root, reports, environ); err != nil {
				t.Errorf("reports at %s with LAB_WORKSPACE %s were refused: %v", reports, mode, err)
			}
		}
	}
}

func TestAClonesReportsThatIsALinkIsRefused(t *testing.T) {
	root, outside := cloneWithMounts(t)
	if err := os.Symlink(filepath.Join(root, "attacks"), filepath.Join(root, "reports")); err != nil {
		t.Fatal(err)
	}
	for _, place := range []string{"reports", "reports/x"} {
		for mode, environ := range map[string][]string{
			"unset": {"HOME=/nowhere"},
			"set":   {"LAB_WORKSPACE=" + outside},
		} {
			_, err := openWorkspace(root, filepath.Join(root, place), environ)
			if err == nil {
				t.Errorf("reports at <clone>/%s through a link to attacks/ were taken with LAB_WORKSPACE %s", place, mode)
				continue
			}
			if !strings.Contains(err.Error(), "link") {
				t.Errorf("the refusal is %v, want it to say the clone's reports/ is a link", err)
			}
		}
	}
}
