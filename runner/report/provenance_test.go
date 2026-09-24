package report_test

import (
	"strings"
	"testing"

	"github.com/guardana/playground/runner/report"
)

func provenance() report.Provenance {
	return report.Provenance{
		Lab:       "0123456789abcdef0123456789abcdef01234567",
		Workspace: "`/work/policies`, 89abcdef0123456789abcdef0123456789abcdef",
		Pins:      []report.Pin{{Name: "ENFORCER_COMMIT", Value: "e72ebe261af4ca9bb4b683ddedda81bfcc5de906"}},
		Images: []report.Image{
			{
				Ref: "playground-enforcer:e72ebe261af4ca9bb4b683ddedda81bfcc5de906", ID: "sha256:c370",
				Label: "e72ebe261af4ca9bb4b683ddedda81bfcc5de906", Want: "e72ebe261af4ca9bb4b683ddedda81bfcc5de906",
				Tree: "4c1159f7c7142e0eafb56cdf1bca17b9dd5fe890", TreeWant: "4c1159f7c7142e0eafb56cdf1bca17b9dd5fe890",
			},
			{
				Ref: "playground-enforcer:hand", ID: "sha256:99",
				Label: "hand", Want: "hand", TreeWant: "4c1159f7c7142e0eafb56cdf1bca17b9dd5fe890",
			},
			{
				Ref: "playground-verifier:0.26.1", ID: "sha256:55e5",
				Label: "0.26.0", Want: "0.26.1",
			},
			{Ref: "playground-other:1", Missing: "not built on this machine"},
			{Ref: "playground-bare:2", ID: "sha256:77", Want: "2"},
		},
		Machine: "darwin/arm64, 10 CPUs, docker 29.6.2 linux/arm64",
	}
}

func TestMarkdownNamesWhatProducedTheRun(t *testing.T) {
	var out strings.Builder
	if err := report.WriteMarkdown(&out, mixed(), nil, provenance()); err != nil {
		t.Fatalf("WriteMarkdown: %v", err)
	}
	page := out.String()
	for _, want := range []string{
		"- Lab: 0123456789abcdef0123456789abcdef01234567",
		"- Workspace: `/work/policies`, 89abcdef0123456789abcdef0123456789abcdef",
		"- Pin `ENFORCER_COMMIT=e72ebe261af4ca9bb4b683ddedda81bfcc5de906`",
		"`playground-enforcer:e72ebe261af4ca9bb4b683ddedda81bfcc5de906` is `sha256:c370` on this machine, " +
			"labelled `e72ebe261af4ca9bb4b683ddedda81bfcc5de906`, the label matches the pin; " +
			"tree label `4c1159f7c7142e0eafb56cdf1bca17b9dd5fe890` matches ENFORCER_TREE\n",
		"`playground-enforcer:hand` is `sha256:99` on this machine, labelled `hand`, the label matches the pin; " +
			"no tree label, does NOT match ENFORCER_TREE `4c1159f7c7142e0eafb56cdf1bca17b9dd5fe890`\n",
		"`playground-verifier:0.26.1` is `sha256:55e5` on this machine, labelled `0.26.0`, does NOT match the pin `0.26.1`\n",
		"`playground-other:1`: not built on this machine",
		"`playground-bare:2` is `sha256:77` on this machine, with no label, does NOT match the pin `2`\n",
		"- Machine: darwin/arm64, 10 CPUs, docker 29.6.2 linux/arm64",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the report does not say %q:\n%s", want, page)
		}
	}
	if strings.Contains(page, ", matches the pin") {
		t.Errorf("the report says an image matches the pin, and only its label was compared:\n%s", page)
	}
	if strings.Contains(page, "built from") {
		t.Errorf("the report claims what an image was built from, and a label is only a build argument:\n%s", page)
	}
}

func TestMarkdownSaysProvenanceWasNotRecorded(t *testing.T) {
	var out strings.Builder
	if err := report.WriteMarkdown(&out, mixed(), nil, report.Provenance{}); err != nil {
		t.Fatalf("WriteMarkdown: %v", err)
	}
	for _, want := range []string{"- Lab: not recorded", "- Workspace: not recorded", "- Pins: not recorded", "- Images: not recorded", "- Machine: not recorded"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("an empty provenance does not say %q:\n%s", want, out.String())
		}
	}
}

func TestAnImageWithoutItsLabelDoesNotMatch(t *testing.T) {
	for _, image := range []report.Image{
		{Ref: "a", ID: "sha256:1", Label: "", Want: ""},
		{Ref: "b", ID: "sha256:1", Label: "x", Want: "y"},
		{Ref: "c", Missing: "not built", Label: "x", Want: "x"},
	} {
		if image.Matches() {
			t.Errorf("%+v matches, and it is not labelled with its pin", image)
		}
	}
	if !(report.Image{Ref: "d", ID: "sha256:1", Label: "x", Want: "x"}).Matches() {
		t.Error("an image labelled with its own pin does not match")
	}
}

func TestJUnitCarriesTheProvenance(t *testing.T) {
	var out strings.Builder
	if err := report.WriteJUnit(&out, mixed(), provenance()); err != nil {
		t.Fatalf("WriteJUnit: %v", err)
	}
	for _, want := range []string{
		`name="workspace" value="` + "`/work/policies`, 89abcdef0123456789abcdef0123456789abcdef" + `"`,
		`name="pin.ENFORCER_COMMIT" value="e72ebe261af4ca9bb4b683ddedda81bfcc5de906"`,
		`name="image.playground-verifier:0.26.1" value="sha256:55e5 labelled=0.26.0"`,
		`name="image.playground-enforcer:hand" value="sha256:99 labelled=hand tree="`,
		`name="image.playground-other:1" value="not built on this machine"`,
		`name="machine" value="darwin/arm64, 10 CPUs, docker 29.6.2 linux/arm64"`,
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the JUnit document does not carry %s:\n%s", want, out.String())
		}
	}
}
