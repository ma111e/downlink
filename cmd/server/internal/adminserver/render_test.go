package adminserver

import (
	"strings"
	"testing"
	"time"

	"github.com/ma111e/downlink/cmd/server/internal/store"
	"github.com/ma111e/downlink/pkg/models"
)

func TestRunsTemplateShowsProfile(t *testing.T) {
	runs := []store.LLMRunSummary{
		{LLMRun: models.LLMRun{Id: "run-x", ProfileId: "infosec", StartedAt: time.Now()}, CallCount: 1},
	}
	data := runsPageData{Runs: runs, Chart: buildChart(runs), Stats: buildStats(runs), ProfileFilter: "infosec"}

	var sb strings.Builder
	if err := runsTmpl.Execute(&sb, data); err != nil {
		t.Fatalf("runsTmpl.Execute() error = %v", err)
	}
	out := sb.String()
	for _, want := range []string{
		`href="/?profile=infosec"`, // per-row profile link
		"Filtered to profile",      // active-filter banner
		`href="/"`,                 // show-all link
	} {
		if !strings.Contains(out, want) {
			t.Errorf("runs template missing %q", want)
		}
	}
}

func TestRunsTemplateExecutes(t *testing.T) {
	runs := []store.LLMRunSummary{
		{LLMRun: models.LLMRun{Id: "run-abc", Title: "Daily Brief", DigestId: "digest-1", StartedAt: time.Now()}, CallCount: 3, TotalTokens: 12345, TotalPromptTokens: 9000, TotalCompletionTokens: 3345, ArticleCount: 5},
		{LLMRun: models.LLMRun{Id: "run-def", StartedAt: time.Now().Add(-time.Hour)}, CallCount: 1, TotalTokens: 200, TotalPromptTokens: 150, TotalCompletionTokens: 50},
	}
	data := runsPageData{Runs: runs, Chart: buildChart(runs), Stats: buildStats(runs)}

	var sb strings.Builder
	if err := runsTmpl.Execute(&sb, data); err != nil {
		t.Fatalf("runsTmpl.Execute() error = %v", err)
	}
	out := sb.String()
	// Run id, totals, sent/received split aggregates, the chart with both
	// stacked segments, and gridline labels must all render.
	for _, want := range []string{
		"run-abc", "12,345", "Daily Brief", "<svg",
		"9,150",              // sent stat (9000+150)
		"3,395",              // received stat (3345+50)
		"bar-up", "bar-down", // stacked segments present
		`class="ratio"`,      // per-row proportion bar
		`id="charttip"`,      // hover tooltip element
		`class="chart-wrap"`, // tooltip positioning context
		`class="hit"`,        // full-column hover target
		"data-total=",        // values exposed for the tooltip
		"mouseenter",         // tooltip wiring script
		"Avg / Article",      // per-run average column header
		"2,469",              // 12345 / 5 articles
		"/5",                 // article-count denominator
	} {
		if !strings.Contains(out, want) {
			t.Errorf("runs page missing %q", want)
		}
	}
	// The uplink/downlink token-label gimmick must be gone (the product name
	// "downlink" in the header brand is fine).
	for _, gone := range []string{"Uplink", "Downlink", "uplink"} {
		if strings.Contains(out, gone) {
			t.Errorf("runs page still contains gimmick term %q", gone)
		}
	}
	// Seline light theme: cream canvas + Inter, no leftover IBM Plex dark theme.
	for _, want := range []string{"#fafaf9", "Inter"} {
		if !strings.Contains(out, want) {
			t.Errorf("runs page missing Seline theme token %q", want)
		}
	}
	if strings.Contains(out, "IBM Plex") {
		t.Error("runs page still references the old IBM Plex theme")
	}
}

func TestRunTemplateExecutes(t *testing.T) {
	fin := time.Now()
	data := runPageData{
		Run: models.LLMRun{Id: "run-abc", Title: "Daily Brief", DigestId: "digest-1", StartedAt: fin.Add(-time.Minute), FinishedAt: &fin},
		Calls: []store.LLMCallView{
			{LLMCall: models.LLMCall{Label: "digest:summary", ModelName: "claude-3-5-sonnet", TotalTokens: 1280, PromptTokens: 1200, CompletionTokens: 80, TokensKnown: true, DurationMs: 4200}, Prompt: "<script>hi</script>", Response: "ok"},
		},
	}

	var sb strings.Builder
	if err := runTmpl.Execute(&sb, data); err != nil {
		t.Fatalf("runTmpl.Execute() error = %v", err)
	}
	out := sb.String()
	// Per-call details and the sent/received chips must show.
	for _, want := range []string{"digest:summary", "Sent 1,200", "Recv 80", "Daily Brief"} {
		if !strings.Contains(out, want) {
			t.Errorf("run page missing %q", want)
		}
	}
	// The recap stats strip was dropped.
	if strings.Contains(out, `class="stats"`) {
		t.Errorf("run page should no longer render the recap stats strip")
	}
	// html/template must escape model output.
	if strings.Contains(out, "<script>hi</script>") {
		t.Errorf("prompt was not HTML-escaped")
	}
}

func TestRefreshesTemplateExecutes(t *testing.T) {
	runs := []store.FeedRefreshRunSummary{
		{FeedRefreshRun: models.FeedRefreshRun{Id: "refresh-abc", Trigger: "manual-all", StartedAt: time.Now()}, FeedCount: 3, OkCount: 2, FailCount: 1, TotalFetched: 1200, TotalStored: 45, ErrorCount: 2},
		{FeedRefreshRun: models.FeedRefreshRun{Id: "refresh-def", Trigger: "startup", StartedAt: time.Now().Add(-time.Hour)}, FeedCount: 1, OkCount: 1, TotalFetched: 10, TotalStored: 3},
	}
	data := refreshesPageData{Runs: runs, Stats: buildRefreshStats(runs)}

	var sb strings.Builder
	if err := refreshesTmpl.Execute(&sb, data); err != nil {
		t.Fatalf("refreshesTmpl.Execute() error = %v", err)
	}
	out := sb.String()
	for _, want := range []string{
		"refresh-abc",               // run id / row link
		"manual-all",                // trigger
		"1,210",                     // total fetched stat (1200+10)
		"48",                        // total stored stat (45+3)
		`class="ratio"`,             // ok/fail proportion bar
		"/feed-refresh/refresh-abc", // detail link
		"feed refresh history",      // crumb
	} {
		if !strings.Contains(out, want) {
			t.Errorf("refreshes page missing %q", want)
		}
	}
}

func TestRefreshRunTemplateExecutes(t *testing.T) {
	fin := time.Now()
	data := refreshRunPageData{
		Run: models.FeedRefreshRun{Id: "refresh-abc", Trigger: "manual-all", StartedAt: fin.Add(-time.Minute), FinishedAt: &fin},
		Results: []store.FeedRefreshResultView{
			{FeedRefreshResult: models.FeedRefreshResult{FeedId: "feed-1", FeedTitle: "Hacker News", FeedURL: "https://news.ycombinator.com/rss", Success: true, TotalFetched: 40, Stored: 5, Skipped: 33, ErrorCount: 1, RawStatus: 200, RawType: "application/rss+xml", DurationMs: 1200}, Errors: []string{"<script>evil</script>: scrape failed"}, RawBody: "<rss><channel><title>News</title></channel></rss>"},
			{FeedRefreshResult: models.FeedRefreshResult{FeedId: "feed-2", FeedTitle: "Broken", Success: false, FetchError: "failed to fetch feed: 503"}},
		},
	}

	var sb strings.Builder
	if err := refreshRunTmpl.Execute(&sb, data); err != nil {
		t.Fatalf("refreshRunTmpl.Execute() error = %v", err)
	}
	out := sb.String()
	for _, want := range []string{
		"Hacker News",               // feed title
		"manual-all refresh",        // run header
		"5 stored",                  // counts summary
		"fetch failed",              // failure chip on the broken feed
		"failed to fetch feed: 503", // top-level error log
		"1 item error",              // item-error chip
		"Raw body",                  // raw-body section heading
		"application/rss",           // raw content-type (the + is HTML-escaped)
		"News",                      // raw body content rendered
		"No raw body captured",      // the feed with no body
	} {
		if !strings.Contains(out, want) {
			t.Errorf("refresh run page missing %q", want)
		}
	}
	// html/template must escape error-log content lifted from feed titles.
	if strings.Contains(out, "<script>evil</script>") {
		t.Errorf("item error was not HTML-escaped")
	}
	// The raw body must be escaped, not rendered as live markup.
	if strings.Contains(out, "<rss><channel>") {
		t.Errorf("raw body was not HTML-escaped")
	}
}

func TestBuildChartStacking(t *testing.T) {
	runs := []store.LLMRunSummary{
		{LLMRun: models.LLMRun{Id: "r1", StartedAt: time.Now()}, TotalTokens: 1000, TotalPromptTokens: 700, TotalCompletionTokens: 300},
	}
	c := buildChart(runs)
	if !c.HasData || len(c.Bars) != 1 {
		t.Fatalf("expected 1 bar with data, got HasData=%v bars=%d", c.HasData, len(c.Bars))
	}
	b := c.Bars[0]
	// Downlink stacks directly on top of uplink; uplink sits on the baseline.
	if b.UpY+b.UpH != c.Baseline {
		t.Errorf("uplink not anchored to baseline: upY=%d upH=%d baseline=%d", b.UpY, b.UpH, c.Baseline)
	}
	if b.DownY+b.DownH != b.UpY {
		t.Errorf("downlink not stacked on uplink: downY=%d downH=%d upY=%d", b.DownY, b.DownH, b.UpY)
	}
	if len(c.Ticks) != 5 {
		t.Errorf("expected 5 gridline ticks, got %d", len(c.Ticks))
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

func TestNiceMax(t *testing.T) {
	cases := []struct{ in, want int }{
		{0, 10}, {1, 1}, {7, 10}, {12, 20}, {240, 250}, {8200, 10000}, {23600, 25000}, {1_500_000, 2_000_000},
	}
	for _, c := range cases {
		if got := niceMax(c.in); got != c.want {
			t.Errorf("niceMax(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestShortTokens(t *testing.T) {
	cases := []struct {
		in   int
		want string
	}{
		{500, "500"}, {1500, "1.5k"}, {2000, "2k"}, {2_000_000, "2M"},
	}
	for _, c := range cases {
		if got := shortTokens(c.in); got != c.want {
			t.Errorf("shortTokens(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}
