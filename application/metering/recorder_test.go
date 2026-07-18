package metering_test

import (
	"testing"

	"github.com/open-strata-ai/ai-tool-registry/application/metering"
	"github.com/open-strata-ai/ai-tool-registry/domain"
)

func TestRecorderAggregates(t *testing.T) {
	var drained []domain.CallMetric
	rec := metering.New(8, func(m domain.CallMetric) { drained = append(drained, m) })
	rec.Record(domain.CallMetric{TenantID: "t1", ToolName: "tool-a", LatencyMs: 10, Success: true})
	rec.Record(domain.CallMetric{TenantID: "t1", ToolName: "tool-a", LatencyMs: 30, Success: false})
	rec.Close()

	if len(drained) != 2 {
		t.Fatalf("want 2 drained events, got %d", len(drained))
	}
	snap := rec.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("want 1 aggregated row, got %d", len(snap))
	}
	row := snap[0]
	if row.Calls != 2 || row.Success != 1 || row.Failures != 1 {
		t.Fatalf("bad aggregates: %+v", row)
	}
	if row.AvgMs != 20 {
		t.Fatalf("want avg 20ms, got %d", row.AvgMs)
	}
}
