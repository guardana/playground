package check

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"slices"
	"strconv"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/internal/labspec"
)

// maxHealthBytes bounds the /healthz record; the enforcer's answer is a few
// kilobytes.
const maxHealthBytes = 1 << 20

// Health grades the enforcer's own /healthz counters, as read after the replay
// and kept in the run directory. A block nothing in the trail records, such
// as one refused for want of room to record it, is counted there and nowhere
// else.
type Health struct {
	Expect labspec.HealthExpectation
	Source string
}

// healthPipeline is the part of /healthz this check reads. Pointers and a nil
// map keep a counter the answer lacks apart from one that reads zero.
type healthPipeline struct {
	Blocks                   map[string]int64 `json:"blocks"`
	ReadsUnrecorded          *int64           `json:"reads_unrecorded"`
	SinkFailuresBeforeEffect *int64           `json:"sink_failures_before_effect"`
}

type healthCount struct {
	check string
	want  int
	got   *int64
}

// ID names the check in a report.
func (Health) ID() string { return "health" }

// Run grades each stated count against the record, and fails every one of
// them when the record is missing or does not parse.
func (h Health) Run(_ context.Context, _ assertion.Records) ([]assertion.Result, error) {
	pipeline, unread := h.read()
	var results []assertion.Result
	for _, count := range h.counts(pipeline) {
		result := assertion.Result{
			Check:  count.check,
			Want:   "/healthz after the replay counts exactly " + strconv.Itoa(count.want),
			Source: h.Source,
		}
		switch {
		case unread != "":
			result.Outcome, result.Got = assertion.Fail, "no count read"
			result.Detail = unread
		case count.got == nil:
			result.Outcome, result.Got = assertion.Fail, "no such counter in the answer"
		case *count.got == int64(count.want):
			result.Outcome, result.Got = assertion.Pass, strconv.FormatInt(*count.got, 10)
		default:
			result.Outcome, result.Got = assertion.Fail, strconv.FormatInt(*count.got, 10)
		}
		results = append(results, result)
	}
	return results, nil
}

// counts pairs every stated count with the counter it names, in a fixed
// order. A reason code the answer's blocks leave out was never counted, so it
// reads as zero; blocks missing altogether is a counter the answer lacks.
func (h Health) counts(pipeline healthPipeline) []healthCount {
	var counts []healthCount
	for _, code := range slices.Sorted(maps.Keys(h.Expect.Blocks)) {
		var got *int64
		if pipeline.Blocks != nil {
			n := pipeline.Blocks[code]
			got = &n
		}
		counts = append(counts, healthCount{"health/blocks/" + code, h.Expect.Blocks[code], got})
	}
	if want := h.Expect.ReadsUnrecorded; want != nil {
		counts = append(counts, healthCount{"health/reads_unrecorded", *want, pipeline.ReadsUnrecorded})
	}
	if want := h.Expect.SinkFailuresBeforeEffect; want != nil {
		counts = append(counts, healthCount{"health/sink_failures_before_effect", *want, pipeline.SinkFailuresBeforeEffect})
	}
	return counts
}

// read parses the record, and says why when it cannot.
func (h Health) read() (healthPipeline, string) {
	info, err := os.Stat(h.Source)
	if err != nil {
		return healthPipeline{}, "no /healthz record: " + err.Error()
	}
	if info.Size() > maxHealthBytes {
		return healthPipeline{}, fmt.Sprintf("the /healthz record is %d bytes, limit %d", info.Size(), maxHealthBytes)
	}
	body, err := os.ReadFile(h.Source)
	if err != nil {
		return healthPipeline{}, "the /healthz record is unreadable: " + err.Error()
	}
	var answer struct {
		Pipeline *healthPipeline `json:"pipeline"`
	}
	if err := json.Unmarshal(body, &answer); err != nil {
		return healthPipeline{}, "the /healthz record does not parse: " + err.Error()
	}
	if answer.Pipeline == nil {
		return healthPipeline{}, "the /healthz record has no pipeline"
	}
	return *answer.Pipeline, ""
}
