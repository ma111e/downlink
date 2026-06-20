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
	"time"

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

	var totalTokens int
	for _, c := range calls {
		totalTokens += c.TotalTokens
	}

	render(w, runTmpl, runPageData{
		Run:         run,
		Calls:       calls,
		TotalTokens: totalTokens,
		TotalCalls:  len(calls),
	})
}

func render(w http.ResponseWriter, tmpl pageRenderer, data interface{}) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		log.WithError(err).Error("failed to render LLM monitor page")
	}
}

// ---------------------------------------------------------------------------
// Chart geometry (inline SVG, no JS) — token totals per run, newest on the right.
// ---------------------------------------------------------------------------

type chartData struct {
	Bars      []chartBar
	Width     int
	Height    int
	HasData   bool
	MaxTokens int
}

type chartBar struct {
	X, Y, W, H int
	Tokens     int
	RunID      string
	When       string
}

func buildChart(runs []store.LLMRunSummary) chartData {
	const (
		height  = 160
		barW    = 14
		gap     = 6
		maxBars = 40
	)

	// Oldest-to-newest left-to-right; runs come newest-first.
	pick := runs
	if len(pick) > maxBars {
		pick = pick[:maxBars]
	}
	ordered := make([]store.LLMRunSummary, 0, len(pick))
	for i := len(pick) - 1; i >= 0; i-- {
		ordered = append(ordered, pick[i])
	}

	maxTokens := 0
	for _, r := range ordered {
		if r.TotalTokens > maxTokens {
			maxTokens = r.TotalTokens
		}
	}

	c := chartData{Height: height, MaxTokens: maxTokens, HasData: maxTokens > 0}
	for i, r := range ordered {
		h := 1
		if maxTokens > 0 {
			h = r.TotalTokens * (height - 20) / maxTokens
			if h < 1 {
				h = 1
			}
		}
		c.Bars = append(c.Bars, chartBar{
			X:      i * (barW + gap),
			Y:      height - h,
			W:      barW,
			H:      h,
			Tokens: r.TotalTokens,
			RunID:  r.Id,
			When:   r.StartedAt.Format(time.RFC822),
		})
	}
	c.Width = len(ordered) * (barW + gap)
	if c.Width == 0 {
		c.Width = 1
	}
	return c
}

type runsPageData struct {
	Runs  []store.LLMRunSummary
	Chart chartData
}

type runPageData struct {
	Run         models.LLMRun
	Calls       []store.LLMCallView
	TotalTokens int
	TotalCalls  int
}
