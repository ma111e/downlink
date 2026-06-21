// Package adminserver serves a small, read-only HTML dashboard for monitoring
// LLM activity during digest generation: a list of runs with token totals and a
// per-run view of every prompt/response that passed through the gateway.
//
// It binds to localhost only and has no authentication, matching the project's
// other local HTTP servers (feedserver, devserver).
package adminserver

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/ma111e/downlink/cmd/server/internal/store"
	"github.com/ma111e/downlink/pkg/models"

	log "github.com/sirupsen/logrus"
)

// AdminServer renders the LLM monitoring pages from the store.
type AdminServer struct {
	store store.Store
	port  int
}

// NewAdminServer creates a monitoring server reading from the given store.
func NewAdminServer(store store.Store, port int) *AdminServer {
	return &AdminServer{store: store, port: port}
}

// Start serves the dashboard on 127.0.0.1:<port> until the process exits.
func (a *AdminServer) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", a.handleRuns)
	mux.HandleFunc("GET /run/{id}", a.handleRun)
	mux.HandleFunc("GET /feeds", a.handleRefreshRuns)
	mux.HandleFunc("GET /feed-refresh/{id}", a.handleRefreshRun)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	addr := fmt.Sprintf("127.0.0.1:%d", a.port)
	log.WithField("addr", addr).Info("Starting LLM monitor server")
	return http.ListenAndServe(addr, mux)
}

// runsListLimit caps how many runs the list page renders.
const runsListLimit = 200

// handleRuns renders the runs list plus a token-per-run bar chart.
func (a *AdminServer) handleRuns(w http.ResponseWriter, _ *http.Request) {
	runs, err := a.store.ListLLMRunSummaries(runsListLimit)
	if err != nil {
		log.WithError(err).Error("failed to list LLM runs")
		http.Error(w, "failed to list runs", http.StatusInternalServerError)
		return
	}

	data := runsPageData{
		Runs:  runs,
		Chart: buildChart(runs),
		Stats: buildStats(runs),
	}
	render(w, runsTmpl, data)
}

// handleRun renders one run's conversations.
func (a *AdminServer) handleRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	run, err := a.store.GetLLMRun(id)
	if err != nil {
		http.Error(w, "run not found", http.StatusNotFound)
		return
	}
	calls, err := a.store.ListLLMCallsForRun(id)
	if err != nil {
		log.WithError(err).Error("failed to list LLM calls")
		http.Error(w, "failed to list calls", http.StatusInternalServerError)
		return
	}

	render(w, runTmpl, runPageData{Run: run, Calls: calls})
}

// handleRefreshRuns renders the feed-refresh history list.
func (a *AdminServer) handleRefreshRuns(w http.ResponseWriter, _ *http.Request) {
	runs, err := a.store.ListFeedRefreshRunSummaries(runsListLimit)
	if err != nil {
		log.WithError(err).Error("failed to list feed refresh runs")
		http.Error(w, "failed to list refresh runs", http.StatusInternalServerError)
		return
	}

	render(w, refreshesTmpl, refreshesPageData{Runs: runs, Stats: buildRefreshStats(runs)})
}

// handleRefreshRun renders one refresh run's per-feed results and failure logs.
func (a *AdminServer) handleRefreshRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	run, err := a.store.GetFeedRefreshRun(id)
	if err != nil {
		http.Error(w, "refresh run not found", http.StatusNotFound)
		return
	}
	results, err := a.store.ListFeedRefreshResultsForRun(id)
	if err != nil {
		log.WithError(err).Error("failed to list feed refresh results")
		http.Error(w, "failed to list refresh results", http.StatusInternalServerError)
		return
	}

	// Surface feeds in error first: fetch failures, then item-level errors, then ok.
	sort.SliceStable(results, func(i, j int) bool {
		return refreshResultRank(results[i]) < refreshResultRank(results[j])
	})

	render(w, refreshRunTmpl, refreshRunPageData{Run: run, Results: results})
}

// refreshResultRank orders per-feed results so failures sort ahead of successes.
func refreshResultRank(r store.FeedRefreshResultView) int {
	switch {
	case !r.Success:
		return 0
	case r.ErrorCount > 0:
		return 1
	default:
		return 2
	}
}

func render(w http.ResponseWriter, tmpl pageRenderer, data interface{}) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		log.WithError(err).Error("failed to render LLM monitor page")
	}
}

// ---------------------------------------------------------------------------
// Dashboard aggregates
// ---------------------------------------------------------------------------

type dashStats struct {
	Runs      int
	Sent      int // sum of prompt tokens
	Received  int // sum of completion tokens
	Total     int
	PeakTotal int // largest single-run total
}

func buildStats(runs []store.LLMRunSummary) dashStats {
	s := dashStats{Runs: len(runs)}
	for _, r := range runs {
		s.Sent += r.TotalPromptTokens
		s.Received += r.TotalCompletionTokens
		s.Total += r.TotalTokens
		if r.TotalTokens > s.PeakTotal {
			s.PeakTotal = r.TotalTokens
		}
	}
	return s
}

// ---------------------------------------------------------------------------
// Chart geometry (inline SVG, no JS): stacked uplink/downlink tokens per run,
// oldest on the left. All coordinates are pre-computed here so the template only
// emits <rect>/<line> elements.
// ---------------------------------------------------------------------------

// Chart canvas constants. The plot area is inset from the SVG edges to leave
// room for the y-axis tick labels (left) and a small top headroom.
const (
	chartHeight  = 200
	chartPlotTop = 8
	chartPlotBot = 22 // baseline row (x-axis)
	chartPadL    = 52 // y-axis label gutter
	chartBarW    = 12
	chartBarGap  = 10
	chartMaxBars = 48
)

type chartData struct {
	Bars     []chartBar
	Ticks    []chartTick
	Width    int // full SVG width
	Height   int
	PlotL    int // left edge of plot area (= chartPadL)
	Baseline int // y of the x-axis
	NiceMax  int
	HasData  bool
}

type chartTick struct {
	Y     int
	Label string
}

type chartBar struct {
	X, W       int
	UpY, UpH   int // uplink (sent/prompt) segment — sits on the baseline
	DownY      int // downlink (received/completion) segment, stacked above uplink
	DownH      int
	Prompt     int
	Completion int
	Total      int
	RunID      string
	When       string
	AnimDelay  string // staggered CSS animation-delay for the load reveal
}

func buildChart(runs []store.LLMRunSummary) chartData {
	// Runs arrive newest-first; draw oldest→newest left-to-right.
	pick := runs
	if len(pick) > chartMaxBars {
		pick = pick[:chartMaxBars]
	}
	ordered := make([]store.LLMRunSummary, 0, len(pick))
	for i := len(pick) - 1; i >= 0; i-- {
		ordered = append(ordered, pick[i])
	}

	maxTotal := 0
	for _, r := range ordered {
		if r.TotalTokens > maxTotal {
			maxTotal = r.TotalTokens
		}
	}

	baseline := chartHeight - chartPlotBot
	plotH := baseline - chartPlotTop
	nice := niceMax(maxTotal)

	c := chartData{
		Height:   chartHeight,
		PlotL:    chartPadL,
		Baseline: baseline,
		NiceMax:  nice,
		HasData:  maxTotal > 0,
	}

	// scale maps a token count to a pixel height within the plot area.
	scale := func(tokens int) int {
		if nice == 0 {
			return 0
		}
		return tokens * plotH / nice
	}
	// floor keeps a non-zero segment visible without ballooning small values.
	floor := func(h, tokens int) int {
		if tokens > 0 && h < 2 {
			return 2
		}
		return h
	}

	for i, r := range ordered {
		x := chartPadL + i*(chartBarW+chartBarGap)
		upH := floor(scale(r.TotalPromptTokens), r.TotalPromptTokens)
		downH := floor(scale(r.TotalCompletionTokens), r.TotalCompletionTokens)
		upY := baseline - upH
		downY := upY - downH
		c.Bars = append(c.Bars, chartBar{
			X:          x,
			W:          chartBarW,
			UpY:        upY,
			UpH:        upH,
			DownY:      downY,
			DownH:      downH,
			Prompt:     r.TotalPromptTokens,
			Completion: r.TotalCompletionTokens,
			Total:      r.TotalTokens,
			RunID:      r.Id,
			When:       r.StartedAt.Format("Jan 2 15:04"),
			AnimDelay:  fmt.Sprintf("%dms", 240+i*20),
		})
	}

	// Four horizontal gridlines/labels at 0, ¼, ½, ¾, max.
	for i := 0; i <= 4; i++ {
		val := nice * i / 4
		c.Ticks = append(c.Ticks, chartTick{
			Y:     baseline - scale(val),
			Label: shortTokens(val),
		})
	}

	c.Width = chartPadL + len(ordered)*(chartBarW+chartBarGap)
	if c.Width < chartPadL+chartBarW {
		c.Width = chartPadL + chartBarW
	}
	return c
}

// niceMax rounds n up to a clean 1/2/2.5/5×10ⁿ axis maximum so bars fill the
// plot without an awkward gap. Zero input yields a small non-zero ceiling so the
// axis still renders.
func niceMax(n int) int {
	if n <= 0 {
		return 10
	}
	mag := 1.0
	for mag*10 <= float64(n) {
		mag *= 10
	}
	for _, step := range []float64{1, 2, 2.5, 5, 10} {
		if float64(n) <= step*mag {
			return int(step * mag)
		}
	}
	return int(10 * mag)
}

// shortTokens renders an axis label compactly (1500 -> "1.5k", 2000000 -> "2M").
func shortTokens(n int) string {
	switch {
	case n >= 1_000_000:
		return strings.TrimSuffix(fmt.Sprintf("%.1f", float64(n)/1e6), ".0") + "M"
	case n >= 1_000:
		return strings.TrimSuffix(fmt.Sprintf("%.1f", float64(n)/1e3), ".0") + "k"
	default:
		return strconv.Itoa(n)
	}
}

type runsPageData struct {
	Runs  []store.LLMRunSummary
	Chart chartData
	Stats dashStats
}

type runPageData struct {
	Run   models.LLMRun
	Calls []store.LLMCallView
}

// ---------------------------------------------------------------------------
// Feed refresh history
// ---------------------------------------------------------------------------

type refreshStats struct {
	Runs    int // refresh cycles tracked
	Feeds   int // total per-feed refreshes across all cycles
	Fetched int // total items fetched
	Stored  int // total articles stored
	Failed  int // per-feed refreshes that hit a top-level fetch error
}

func buildRefreshStats(runs []store.FeedRefreshRunSummary) refreshStats {
	s := refreshStats{Runs: len(runs)}
	for _, r := range runs {
		s.Feeds += r.FeedCount
		s.Fetched += r.TotalFetched
		s.Stored += r.TotalStored
		s.Failed += r.FailCount
	}
	return s
}

type refreshesPageData struct {
	Runs  []store.FeedRefreshRunSummary
	Stats refreshStats
}

type refreshRunPageData struct {
	Run     models.FeedRefreshRun
	Results []store.FeedRefreshResultView
}
