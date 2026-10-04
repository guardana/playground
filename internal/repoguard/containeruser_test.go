package repoguard_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The stub answers as a daemon that writes the probe's key for $WRITER alone
// ("*" for any user), says it is rootless when $ROOTLESS is set, logs every
// --user to $CALLS, and makes a lab key when keygen writes to /keys.
const mappingDocker = `#!/bin/sh
case "$1" in
info) [ -n "$ROOTLESS" ] && echo '["name=rootless"]' || echo '[]'; exit 0 ;;
image) exit 0 ;;
esac
user="" mount=""
while [ $# -gt 0 ]; do
	case "$1" in
	--user) user=$2; shift ;;
	-v) mount=$2; shift ;;
	esac
	shift
done
echo "$user ${mount##*:}" >>"$CALLS"
dir=${mount%:*}
[ "$WRITER" = "*" ] || [ "$user" = "$WRITER" ] || { echo "mkdir ${mount##*:}/key: permission denied" >&2; exit 1; }
mkdir -p "$dir/key" && echo key >"$dir/key/signing.key" && echo pub >"$dir/key/signing.pub"
[ "${mount##*:}" = /keys ] && printf 'key_id: k\npublic_key: p\n'
exit 0
`

type probe struct {
	writer, rootless, script string
}

// run runs script under bash in the clone with the stub first on PATH, and
// returns its output and the --user of every docker run.
func (p probe) run(t *testing.T, env ...string) (string, []string, error) {
	t.Helper()
	stubs, scratch := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(stubs, "docker"), []byte(mappingDocker), 0o700); err != nil { // #nosec G306 -- a stub to run.
		t.Fatal(err)
	}
	calls := filepath.Join(stubs, "calls")
	command := exec.Command("bash", "-c", p.script) // #nosec G204 -- scripts of this test.
	command.Dir = repoRoot
	command.Env = append(append(os.Environ(), env...), "WRITER="+p.writer, "ROOTLESS="+p.rootless,
		"CALLS="+calls, "TMPDIR="+scratch, "PATH="+stubs+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := command.CombinedOutput()
	logged, _ := os.ReadFile(calls) // #nosec G304 -- the stub's log in this test's directory.
	left, readErr := os.ReadDir(scratch)
	if readErr != nil {
		t.Fatal(readErr)
	}
	for _, entry := range left {
		t.Errorf("left behind in TMPDIR: %s", entry.Name())
	}
	return string(out), strings.Fields(string(logged)), err
}

const choose = `source scripts/container-user.sh && container_user lab-enforcer:pin`

// The scripts run the enforcer's image as their own ids, or as root under
// rootless Docker alone, and never try root on a rootful daemon.
func TestTheScriptsRunTheEnforcerAsTheUserWhoseFilesTheyOwn(t *testing.T) {
	own := fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid())
	for name, c := range map[string]struct {
		probe
		want string
	}{
		"a daemon that keeps this user's ids": {probe{own, "", choose}, own},
		"rootless Docker":                     {probe{"0:0", "1", choose}, "0:0"},
		"a rootful daemon where root writes":  {probe{"0:0", "", choose}, ""},
	} {
		t.Run(name, func(t *testing.T) {
			out, logged, err := c.run(t)
			if c.want == "" {
				if err == nil || strings.Contains(strings.Join(logged, " "), "0:0") {
					t.Fatalf("chose %q, tried %v: root on a rootful daemon", out, logged)
				}
			} else if err != nil || out != c.want {
				t.Fatalf("chose %q (%v), want %s", out, err, c.want)
			}
		})
	}
}

// A key written but owned by another uid is a mapping the lab does not know;
// a docker that failed is reported in docker's own words.
func TestTheScriptsSayWhichProbeFailed(t *testing.T) {
	other := `source scripts/container-user.sh && file_owner() { echo 99999; } && container_user lab-enforcer:pin`
	for name, c := range map[string]struct {
		probe
		says string
	}{
		"files owned by another uid": {probe{"*", "1", other}, "maps users in a way the lab does not know"},
		"a docker that refuses":      {probe{"nobody", "", choose}, "permission denied"},
	} {
		t.Run(name, func(t *testing.T) {
			out, _, err := c.run(t)
			if err == nil || !strings.Contains(out, c.says) {
				t.Fatalf("got %v\n%s\nwant an error saying %q", err, out, c.says)
			}
		})
	}
}

// lab-key.sh makes the key as the user the probe chose: root under rootless
// Docker, where its own ids cannot write the key's place.
func TestLabKeyMakesTheKeyAsTheProbedUser(t *testing.T) {
	keys := filepath.Join(t.TempDir(), "state", "lab-key")
	out, logged, err := probe{"0:0", "1", "scripts/lab-key.sh"}.run(t, "LAB_KEYS_DIR="+keys)
	if err != nil {
		t.Fatalf("lab-key: %v\n%s", err, out)
	}
	made := ""
	for i := 0; i+1 < len(logged); i += 2 {
		if logged[i+1] == "/keys" {
			made = logged[i]
		}
	}
	if made != "0:0" {
		t.Errorf("the key was made as %q, want 0:0; docker runs %v", made, logged)
	}
	if _, err := os.Stat(filepath.Join(keys, "signing.key")); err != nil {
		t.Errorf("no key at %s: %v", keys, err)
	}
}

// Only container_user picks the user a script runs the enforcer's image as.
func TestNoScriptRunsTheEnforcerAsItsOwnIdsUnprobed(t *testing.T) {
	scripts, err := filepath.Glob(filepath.Join(repoRoot, "scripts", "*.sh"))
	if err != nil || len(scripts) == 0 {
		t.Fatalf("no scripts: %v", err)
	}
	direct := regexp.MustCompile(`--user "?\$\(id -u\)`)
	for _, script := range scripts {
		body, err := os.ReadFile(script) // #nosec G304 -- a script of this repository.
		if err != nil {
			t.Fatal(err)
		}
		if direct.Match(body) {
			t.Errorf("%s passes its own ids to --user instead of container_user's choice", filepath.Base(script))
		}
	}
}
