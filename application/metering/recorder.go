// Package metering implements domain.MeteringPort (R5 / DESIGN §12): call
// metrics are buffered on a channel and drained by a background worker so the
// hot path never blocks (DESIGN §9). Aggregates are kept for /metrics.
package metering

import (
	"sort"
	"sync"

	"github.com/open-strata-ai/ai-tool-registry/domain"
)

// Sink receives drained call metrics (production: async push to ai-billing-service).
type Sink func(domain.CallMetric)

// MetricLine is an aggregated per-tool metric row.
type MetricLine struct {
	ToolName string `json:"tool"`
	Calls    int64  `json:"calls"`
	Success  int64  `json:"success"`
	Failures int64  `json:"failures"`
	AvgMs    int64  `json:"avg_latency_ms"`
}

// Recorder is an async, non-blocking metering recorder with live aggregates.
type Recorder struct {
	ch     chan domain.CallMetric
	wg     sync.WaitGroup
	closed chan struct{}
	sink   Sink

	mu      sync.Mutex
	calls   map[string]int64
	success map[string]int64
	fail    map[string]int64
	latSum  map[string]int64
}

// New builds a Recorder with a buffered queue and starts the drain worker.
func New(buffer int, sink Sink) *Recorder {
	if buffer <= 0 {
		buffer = 1024
	}
	r := &Recorder{
		ch:      make(chan domain.CallMetric, buffer),
		closed:  make(chan struct{}),
		sink:    sink,
		calls:   map[string]int64{},
		success: map[string]int64{},
		fail:    map[string]int64{},
		latSum:  map[string]int64{},
	}
	r.wg.Add(1)
	go r.drain()
	return r
}

// Record enqueues a metric without blocking; a full buffer drops the event to
// protect the hot path (DESIGN §9).
func (r *Recorder) Record(m domain.CallMetric) {
	select {
	case r.ch <- m:
	default:
	}
}

func (r *Recorder) drain() {
	defer r.wg.Done()
	for {
		select {
		case m := <-r.ch:
			r.aggregate(m)
			if r.sink != nil {
				r.sink(m)
			}
		case <-r.closed:
			for {
				select {
				case m := <-r.ch:
					r.aggregate(m)
					if r.sink != nil {
						r.sink(m)
					}
				default:
					return
				}
			}
		}
	}
}

func (r *Recorder) aggregate(m domain.CallMetric) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls[m.ToolName]++
	r.latSum[m.ToolName] += m.LatencyMs
	if m.Success {
		r.success[m.ToolName]++
	} else {
		r.fail[m.ToolName]++
	}
}

// Snapshot returns the current aggregates, sorted by tool name.
func (r *Recorder) Snapshot() []MetricLine {
	r.mu.Lock()
	defer r.mu.Unlock()
	names := make([]string, 0, len(r.calls))
	for n := range r.calls {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]MetricLine, 0, len(names))
	for _, n := range names {
		calls := r.calls[n]
		avg := int64(0)
		if calls > 0 {
			avg = r.latSum[n] / calls
		}
		out = append(out, MetricLine{
			ToolName: n,
			Calls:    calls,
			Success:  r.success[n],
			Failures: r.fail[n],
			AvgMs:    avg,
		})
	}
	return out
}

// Close stops the worker after flushing buffered events.
func (r *Recorder) Close() {
	close(r.closed)
	r.wg.Wait()
}

var _ domain.MeteringPort = (*Recorder)(nil)
