// Package metering provides concrete sinks for the async metering Recorder
// (application/metering). Offline it logs; production pushes to ai-billing-service
// over the async event bus (R5 / DESIGN §12).
package metering

import (
	"log"

	appmetering "github.com/open-strata-ai/ai-tool-registry/application/metering"
	"github.com/open-strata-ai/ai-tool-registry/domain"
)

// LogSink returns a Sink that logs each call metric.
func LogSink() appmetering.Sink {
	return func(ev domain.CallMetric) {
		log.Printf("metering tenant=%s tool=%s latency_ms=%d success=%t",
			ev.TenantID, ev.ToolName, ev.LatencyMs, ev.Success)
	}
}

// DiscardSink returns a Sink that ignores events (tests).
func DiscardSink() appmetering.Sink {
	return func(domain.CallMetric) {}
}
