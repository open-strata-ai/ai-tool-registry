package discovery_test

import (
	"testing"

	"github.com/open-strata-ai/ai-tool-registry/application/discovery"
	"github.com/open-strata-ai/ai-tool-registry/domain"
)

func insts() []domain.ToolInstance {
	return []domain.ToolInstance{
		{Endpoint: "a", Healthy: true, Weight: 1},
		{Endpoint: "b", Healthy: true, Weight: 3},
		{Endpoint: "c", Healthy: false, Weight: 5},
	}
}

func TestHealthyFiltersUnhealthy(t *testing.T) {
	h := discovery.Healthy(insts())
	if len(h) != 2 {
		t.Fatalf("want 2 healthy, got %d", len(h))
	}
}

func TestPickRespectsWeights(t *testing.T) {
	// rng near 0 always selects first healthy; near 1 selects last healthy.
	first, ok := discovery.Pick(insts(), func() float64 { return 0.0 })
	if !ok || first.Endpoint != "a" {
		t.Fatalf("rng=0 should pick a, got %+v", first)
	}
	last, _ := discovery.Pick(insts(), func() float64 { return 0.999 })
	if last.Endpoint != "b" {
		t.Fatalf("rng~1 should pick b, got %+v", last)
	}
}

func TestPickNoHealthy(t *testing.T) {
	_, ok := discovery.Pick([]domain.ToolInstance{{Endpoint: "x", Healthy: false}}, func() float64 { return 0.5 })
	if ok {
		t.Fatalf("expected no pick when no healthy instances")
	}
}
