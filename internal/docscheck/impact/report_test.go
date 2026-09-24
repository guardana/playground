package impact_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/docscheck/impact"
)

var pages = []impact.Page{
	{Path: "docs/runbooks/run.md", Covers: []string{"runner/**", "Makefile"}},
	{Path: "docs/reference/victims.md", Covers: []string{"victims/*/README.md"}},
}

func TestReportNamesWhatAChangeMakesSuspect(t *testing.T) {
	changed := []string{"runner/lab.go", "victims/shell/README.md", "victims/shell/main.go", "docs/status.md", "LICENSE"}
	r, err := impact.Report(pages, []string{"runner/**", "victims/**"}, []string{"docs/status.md", "victims/*/README.md"}, changed)
	if err != nil {
		t.Fatal(err)
	}
	want := "5 changed paths read\n" +
		"2 pages to review of 2 examined\n" +
		"  docs/runbooks/run.md <- runner/lab.go\n" +
		"  docs/reference/victims.md <- victims/shell/README.md\n" +
		"1 changed paths under a surface no page covers, of 2 surfaces examined\n" +
		"  victims/shell/main.go (victims/**)\n" +
		"2 changed paths under a frozen path, a contract change, of 2 frozen globs examined\n" +
		"  victims/shell/README.md (victims/*/README.md)\n" +
		"  docs/status.md (docs/status.md)\n"
	if got := r.String(); got != want {
		t.Errorf("String =\n%s\nwant\n%s", got, want)
	}
}

func TestReportRefusesARunThatWouldExamineNothing(t *testing.T) {
	for name, run := range map[string]func() error{
		"no page":    func() error { _, err := impact.Report(nil, []string{"runner/**"}, nil, nil); return err },
		"no surface": func() error { _, err := impact.Report(pages, nil, nil, nil); return err },
		"a bad glob": func() error { _, err := impact.Report(pages, []string{"runner//x"}, nil, nil); return err },
		"a dirty path": func() error {
			_, err := impact.Report(pages, []string{"runner/**"}, nil, []string{"./runner/lab.go"})
			return err
		},
		"a path twice": func() error {
			_, err := impact.Report(pages, []string{"runner/**"}, nil, []string{"Makefile", "Makefile"})
			return err
		},
		"a page twice": func() error { _, err := impact.Covering(append(slices.Clone(pages), pages[0]), "Makefile"); return err },
	} {
		if err := run(); !errors.Is(err, impact.ErrInvalid) {
			t.Errorf("%s: %v, want a refusal", name, err)
		}
	}
}

func TestCoveringNamesThePagesOfOnePath(t *testing.T) {
	got, err := impact.Covering(pages, "Makefile")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []string{"docs/runbooks/run.md"}) {
		t.Errorf("Covering = %q", got)
	}
}

func TestParseNameOnlyRefusesWhatIsNotAPath(t *testing.T) {
	for _, out := range []string{"a b.go\n", "\"q\\303\\251.go\"\n", "a.go\r\n", "a.go\na.go\n", "../x\n"} {
		if _, err := impact.ParseNameOnly([]byte(out)); !errors.Is(err, impact.ErrInvalid) {
			t.Errorf("ParseNameOnly(%q) = %v, want a refusal", out, err)
		}
	}
	got, err := impact.ParseNameOnly([]byte("runner/lab.go\nMakefile\n"))
	if err != nil || strings.Join(got, ",") != "runner/lab.go,Makefile" {
		t.Errorf("ParseNameOnly = %q, %v", got, err)
	}
}
