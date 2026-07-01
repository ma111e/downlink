package services

import (
	"github.com/ma111e/downlink/cmd/server/internal/store"
	"github.com/ma111e/downlink/pkg/llmgateway"

	log "github.com/sirupsen/logrus"
)

// LLMMonitorRetention is the number of most-recent digest runs whose LLM calls
// are kept; older runs are pruned at the end of each generation. Set from the
// --llm-monitor-retention flag in main. 0 disables pruning.
var LLMMonitorRetention = 100

// AnalysisRetention is the number of most-recent analyses kept per (article,
// profile); older ones are pruned at the end of each digest generation. Analyses
// referenced by a digest are always kept regardless. Set from the
// --analysis-retention flag in main. 0 disables pruning.
var AnalysisRetention = 10

// llmRecorder implements llmgateway.Recorder, persisting each LLM call that
// belongs to a digest run to the store for the monitoring webpage. Calls made
// outside a run (no run id on the context) are ignored — they have nowhere to
// be displayed and would only orphan rows.
type llmRecorder struct{}

// NewLLMRecorder returns the gateway recorder for the monitoring layer.
func NewLLMRecorder() llmgateway.Recorder { return &llmRecorder{} }

func (r *llmRecorder) Record(rec llmgateway.CallRecord) {
	if rec.RunID == "" {
		return
	}

	in := store.LLMCallInput{
		RunID:            rec.RunID,
		Label:            rec.Label,
		ProviderType:     rec.ProviderType,
		ModelName:        rec.ModelName,
		Prompt:           rec.Prompt,
		Response:         rec.Response,
		PromptTokens:     rec.Usage.PromptTokens,
		CompletionTokens: rec.Usage.CompletionTokens,
		TotalTokens:      rec.Usage.TotalTokens,
		TokensKnown:      rec.UsageKnown,
		DurationMs:       rec.Duration.Milliseconds(),
	}
	if rec.Err != nil {
		in.Err = rec.Err.Error()
	}

	if err := store.Db.RecordLLMCall(in); err != nil {
		// Monitoring must never break generation; log and move on.
		log.WithError(err).WithField("run_id", rec.RunID).Warn("failed to record LLM call")
	}
}
