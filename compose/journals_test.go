package compose

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/labspec"
)

// journalMount is one writer's own journal directory at the container path
// every writer writes to. A missing source is refused rather than created:
// Docker would create it root-owned, where the writer cannot write.
func journalMount(writer string) map[string]any {
	return map[string]any{
		"type":   "bind",
		"source": runDir + "/journals/" + writer,
		"target": "/reports/${LAB_RUN_ID:-manual}/journals",
		"bind":   map[string]any{"create_host_path": false},
	}
}

// A writer that could reach another writer's directory could rewrite the
// journal that writer is graded on, so each writer mounts its own directory
// once and no service mounts anyone else's or the journals directory above.
func TestEachJournalWriterMountsItsOwnDirectoryAlone(t *testing.T) {
	writers := append(labspec.Victims(), "pdp-double", "approver")
	mounts := map[string]int{}
	for name, service := range readMounts(t).Services {
		for _, volume := range service.Volumes {
			if !strings.Contains(source(volume), "/journals") {
				continue
			}
			if !slices.Contains(writers, name) || !reflect.DeepEqual(volume, any(journalMount(name))) {
				t.Errorf("%s mounts %v, want only %v", name, volume, journalMount(name))
				continue
			}
			mounts[name]++
		}
	}
	for _, writer := range writers {
		if mounts[writer] != 1 {
			t.Errorf("%s mounts its journal directory %d time(s), want once", writer, mounts[writer])
		}
	}
}

// A victim names its journal file after LAB_SERVER_NAME, and the runner reads
// it from the directory named after the service: the two names have to agree.
func TestEachVictimIsNamedForItsService(t *testing.T) {
	services := readMounts(t).Services
	for _, victim := range labspec.Victims() {
		if got := services[victim].Environment["LAB_SERVER_NAME"]; got != victim {
			t.Errorf("%s sets LAB_SERVER_NAME %v, want %s", victim, got, victim)
		}
	}
}
