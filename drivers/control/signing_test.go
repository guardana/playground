package control

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestTheSignerRunsUnprivilegedFromThePinnedImageOnly(t *testing.T) {
	root := t.TempDir()
	writeFile(filepath.Join(root, versionFile), "ENFORCER_IMAGE=lab-enforcer\nENFORCER_COMMIT="+testPin+"\n")
	d := &daemon{writer: ownIDs()}
	sign := signWithEnforcer(root, d.run)
	if err := sign(context.Background(), "/keys", "/repo/config/policies/p.json", "/tmp/out"); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(d.calls[len(d.calls)-1], " ")
	for _, want := range []string{
		"--pull never", "--cap-drop ALL", "--security-opt no-new-privileges", "--network none", "--read-only",
		"-v /keys:/key:ro", "-v /tmp/out:/out", "lab-enforcer:" + testPin,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("the signer ran without %q: %s", want, joined)
		}
	}
}
