// Package discovery holds the pure load-balancing logic for tool instances
// (DESIGN §5.3): weighted-random (default) and round-robin selection. The
// instance set itself lives behind domain.Discovery (infrastructure/discovery).
package discovery

import (
	"math/rand"

	"github.com/open-strata-ai/ai-tool-registry/domain"
)

// Healthy returns only the healthy instances (DESIGN §5.3).
func Healthy(insts []domain.ToolInstance) []domain.ToolInstance {
	var out []domain.ToolInstance
	for _, i := range insts {
		if i.Healthy {
			out = append(out, i)
		}
	}
	return out
}

// TotalWeight sums instance weights (minimum 1 each to avoid zero totals).
func TotalWeight(insts []domain.ToolInstance) int {
	sum := 0
	for _, i := range insts {
		w := i.Weight
		if w <= 0 {
			w = 1
		}
		sum += w
	}
	return sum
}

// Pick selects one instance by weighted random. rng must return [0,1). If no
// healthy instances exist it returns the zero value with ok=false.
func Pick(insts []domain.ToolInstance, rng func() float64) (domain.ToolInstance, bool) {
	healthy := Healthy(insts)
	if len(healthy) == 0 {
		return domain.ToolInstance{}, false
	}
	total := TotalWeight(healthy)
	if total <= 0 {
		return healthy[rand.Intn(len(healthy))], true
	}
	r := rng()
	target := r * float64(total)
	acc := 0
	for _, i := range healthy {
		w := i.Weight
		if w <= 0 {
			w = 1
		}
		acc += w
		if float64(acc) >= target {
			return i, true
		}
	}
	return healthy[len(healthy)-1], true
}
