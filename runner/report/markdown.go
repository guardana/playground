package report

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/runner/check"
)

// WriteMarkdown writes the report a person opens after a red run. Every row
// names the file the value was read from, because the next thing that reader
// does is open it.
func WriteMarkdown(w io.Writer, r assertion.Report, rows []check.DecisionRow, p Provenance) error {
	out := &writer{to: w}
	out.printf("# %s\n\n", or(r.Scenario, "an unnamed scenario"))
	out.printf("- Run: `%s`\n", or(r.RunID, "unnamed"))
	out.printf("- Outcome: **%s**\n", r.Outcome().String())
	out.printf("- Started: %s, took %ss\n", r.StartedAt.UTC().Format("2006-01-02T15:04:05Z"), seconds(r))
	if r.Gap != "" {
		out.printf("- Known gap: %s. A pass means the system still does what it documents today; the Wanted column is what it should do\n", r.Gap)
	}
	out.printf("\n")

	writeProvenance(out, p)
	writeDecisions(out, rows)
	writeResults(out, r.Results)
	return out.err
}

func writeDecisions(out *writer, rows []check.DecisionRow) {
	out.printf("## Decisions\n\n")
	if len(rows) == 0 {
		out.printf("No step was graded, so the run establishes nothing about any decision.\n\n")
		return
	}
	wanted := slices.ContainsFunc(rows, func(row check.DecisionRow) bool { return row.Wanted != "" })
	out.printf("| Step | Expected | Recorded | Reason codes | Outcome | Source |%s\n", ifWanted(wanted, " Wanted |"))
	out.printf("|---|---|---|---|---|---|%s\n", ifWanted(wanted, "---|"))
	for _, row := range rows {
		out.printf("| %d | %s | %s | %s | %s | %s |%s\n",
			row.Step,
			cell(row.Want),
			cell(row.Got),
			cell(strings.Join(row.ReasonCodes, " ")),
			row.Outcome.String(),
			cell(row.Source),
			ifWanted(wanted, " "+cell(row.Wanted)+" |"))
	}
	out.printf("\n")
}

func writeResults(out *writer, results []assertion.Result) {
	out.printf("## Checks\n\n")
	if len(results) == 0 {
		out.printf("No check reported anything, so the run graded nothing.\n")
		return
	}
	out.printf("| Check | Outcome | Want | Got | Source |\n")
	out.printf("|---|---|---|---|---|\n")
	for _, result := range results {
		out.printf("| %s | %s | %s | %s | %s |\n",
			cell(result.Check), result.Outcome.String(),
			cell(result.Want), cell(result.Got), cell(result.Source))
	}
	out.printf("\n")
	writeDetails(out, results)
}

func writeDetails(out *writer, results []assertion.Result) {
	var unresolved []assertion.Result
	for _, result := range results {
		if result.Outcome != assertion.Pass && result.Detail != "" {
			unresolved = append(unresolved, result)
		}
	}
	if len(unresolved) == 0 {
		return
	}
	out.printf("## What went wrong\n\n")
	for _, result := range unresolved {
		out.printf("- **%s** (%s): %s\n", result.Check, result.Outcome.String(), result.Detail)
		if result.Source != "" {
			out.printf("  Read from `%s`.\n", result.Source)
		}
	}
}

func ifWanted(wanted bool, text string) string {
	if !wanted {
		return ""
	}
	return text
}

// cell keeps one value inside one table cell. A detail written by a service
// under test can hold anything, and a pipe in it would silently move a column.
func cell(value string) string {
	if value == "" {
		return "-"
	}
	value = strings.ReplaceAll(value, "|", `\|`)
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.TrimSpace(value)
}

// writer keeps the first write error and stops, so a full disk is reported once
// rather than on every line.
type writer struct {
	to  io.Writer
	err error
}

func (w *writer) printf(format string, values ...any) {
	if w.err != nil {
		return
	}
	_, w.err = fmt.Fprintf(w.to, format, values...)
}
