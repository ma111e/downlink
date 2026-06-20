package adminserver

import (
	"strings"
	"testing"
	"time"

	"github.com/ma111e/downlink/cmd/server/internal/store"
	"github.com/ma111e/downlink/pkg/models"
)

func TestRunsTemplateExecutes(t *testing.T) {
	runs := []store.LLMRunSummary{
		{LLMRun: models.LLMRun{Id: "run-abc", Title: "Daily Brief", DigestId: "digest-1", StartedAt: time.Now()}, CallCount: 3, TotalTokens: 12345},
		{LLMRun: models.LLMRun{Id: "run-def", StartedAt: time.Now().Add(-time.Hour)}, CallCount: 1, TotalTokens: 200},
	}
	data := runsPageData{Runs: runs, Chart: buildChart(runs)}

	var sb strings.Builder
	if err := runsTmpl.Execute(&sb, data); err != nil {
		t.Fatalf("runsTmpl.Execute() error = %v", err)
	}
	out := sb.String()
	for _, want := range []string{"run-abc", "12,345", "Daily Brief", "<svg"} {
		if !strings.Contains(out, want) {
			t.Errorf("runs page missing %q", want)
		}
	}
}

func TestRunTemplateExecutes(t *testing.T) {
	fin := time.Now()
	data := runPageData{
		Run: models.LLMRun{Id: "run-abc", Title: "Daily Brief", DigestId: "digest-1", StartedAt: fin.Add(-time.Minute), FinishedAt: &fin},
		Calls: []store.LLMCallView{
			{LLMCall: models.LLMCall{Label: "digest:summary", ModelName: "claude-3-5-sonnet", TotalTokens: 1280, PromptTokens: 1200, CompletionTokens: 80, TokensKnown: true, DurationMs: 4200}, Prompt: "<script>hi</script>", Response: "ok"},
		},
		TotalTokens: 1280,
		TotalCalls:  1,
	}

	var sb strings.Builder
	if err := runTmpl.Execute(&sb, data); err != nil {
		t.Fatalf("runTmpl.Execute() error = %v", err)
	}
	out := sb.String()
	if !strings.Contains(out, "digest:summary") || !strings.Contains(out, "1,280") {
		t.Errorf("run page missing call details: %s", out)
	}
	// html/template must escape model output.
	if strings.Contains(out, "<script>hi</script>") {
		t.Errorf("prompt was not HTML-escaped")
	}
}

func TestBuildChartEmpty(t *testing.T) {
	c := buildChart(nil)
	if c.HasData {
		t.Error("expected HasData=false for no runs")
	}
	if c.Width < 1 {
		t.Error("chart width must be at least 1 to be valid SVG")
	}
}
