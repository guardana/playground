package compose

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

// Under rootless Docker a container's root is the user who owns the run
// directory and gid 0 that user's group, so a service that mounts part of a
// run runs as a numeric uid and gid other than 0, pinned here rather than left
// to its image.
func TestEveryServiceMountingARunPinsAUserThatIsNotRoot(t *testing.T) {
	body, err := os.ReadFile("compose.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Services map[string]struct {
			User    string `json:"user"`
			Volumes []any  `json:"volumes"`
		} `json:"services"`
	}
	if err := yaml.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("compose.yaml: %v", err)
	}
	checked := 0
	for name, service := range parsed.Services {
		if !mountsRun(service.Volumes) {
			continue
		}
		checked++
		uid, gid, _ := strings.Cut(service.User, ":")
		if !positive(uid) || !positive(gid) {
			t.Errorf("%s mounts part of a run as user %q; pin a numeric uid and gid other than 0", name, service.User)
		}
	}
	if checked == 0 {
		t.Fatal("no service mounts part of a run; this test inspected nothing")
	}
}

func mountsRun(volumes []any) bool {
	for _, volume := range volumes {
		if strings.HasPrefix(source(volume), "${LAB_RUN_HOST_DIR") {
			return true
		}
	}
	return false
}

func positive(id string) bool {
	n, err := strconv.Atoi(id)
	return err == nil && n > 0
}
