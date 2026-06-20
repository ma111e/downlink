package services

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/ma111e/downlink/cmd/server/internal/manager"
	"github.com/ma111e/downlink/cmd/server/internal/scrapers"
	"github.com/ma111e/downlink/pkg/models"

	"gopkg.in/yaml.v3"
)

//go:embed feed_autoconfig_prompt.md
var autoconfigSystemPrompt string

const (
	// defaultAutoconfigSteps caps how many LLM turns the selector phase may take.
	defaultAutoconfigSteps = 16
	// autoconfigUsableChars mirrors scrapers' usable-content threshold: a candidate
	// shorter than this is treated as a stub, not an article body.
	autoconfigUsableChars = 500
	// feedContentModeThreshold is the fraction of sampled entries that must already
	// carry a full body before autoconfig short-circuits to scraping: none. Mirrors
	// SelectorScore.Reliable so "good enough" means the same thing across the agent.
	feedContentModeThreshold = 0.8
	// desktopUA is the User-Agent tried during header probing.
	desktopUA = "Mozilla/5.0 (X11; Linux x86_64; rv:130.0) Gecko/20100101 Firefox/130.0"
)

// scrapeModeOrder is the escalation order the mode probe follows: cheapest first,
// full_browser last (resource-heavy, but the reliable fallback).
var scrapeModeOrder = []string{"static", "dynamic", "full_browser"}

// autoconfigGenerate is the single LLM entry point the agent needs: prompt in, text out.
type autoconfigGenerate func(ctx context.Context, prompt string) (string, error)

// autoconfigTools is the set of side-effecting capabilities the agent drives.
type autoconfigTools interface {
	inspectFeed(url string, headers map[string]string) feedObs
	suggestSelectors(url, mode string, headers map[string]string) suggestObs
	testSelector(urls []string, mode, article, cutoff, blacklist string, headers map[string]string) testObs
}

type feedObs struct {
	ParseOK            bool     `json:"parse_ok"`
	FeedType           string   `json:"feed_type"`
	Title              string   `json:"title"`
	Verdict            string   `json:"verdict"`
	SampleLinks        []string `json:"sample_links"`
	SampleContentChars []int    `json:"sample_content_chars,omitempty"`
}

type suggestObs struct {
	Candidates []models.SelectorCandidate `json:"candidates,omitempty"`
	Error      string                     `json:"error,omitempty"`
}

type perURLResult struct {
	URL     string `json:"url"`
	Matched bool   `json:"matched"`
	Chars   int    `json:"chars"`
}

type testObs struct {
	Score   float64        `json:"score"`
	Usable  int            `json:"usable"`
	Samples int            `json:"samples"`
	Results []perURLResult `json:"results,omitempty"`
	Error   string         `json:"error,omitempty"`
}

// AutoConfigResult is the agent's final output.
type AutoConfigResult struct {
	ConfigYAML string
	Summary    string
	Confidence float64
}

// autoconfigStep reports one step for streaming.
type autoconfigStep struct {
	N      int
	Tool   string
	Detail string
}

// agentAction is one decoded model turn.
type agentAction struct {
	Thought string          `json:"thought"`
	Action  string          `json:"action"`
	Args    json.RawMessage `json:"args"`
	Config  *agentConfig    `json:"config"`
}

type agentConfig struct {
	Selectors struct {
		Article   string `json:"article"`
		Cutoff    string `json:"cutoff"`
		Blacklist string `json:"blacklist"`
	} `json:"selectors"`
}

// runAutoConfig discovers a feed's config. It first decides the scraping mode and
// headers deterministically (probe-then-lock), then runs a bounded LLM loop — with
// the mode and headers FROZEN — whose only job is selector discovery. Locking the
// mode/headers up front, plus a duplicate-call guard, keeps the agent from hammering
// the target server.
func runAutoConfig(
	ctx context.Context,
	gen autoconfigGenerate,
	tools autoconfigTools,
	feedURL, feedType string,
	seedHeaders map[string]string,
	maxSteps int,
	onStep func(autoconfigStep),
	onLLM func(turn int, prompt, response string),
) (AutoConfigResult, error) {
	if maxSteps <= 0 {
		maxSteps = defaultAutoconfigSteps
	}
	step := 0
	next := func() int { step++; return step }

	// ── Pre-step 1: lock headers (probe only if the feed is blocked) ──
	lockedHeaders, feed, err := lockHeaders(tools, feedURL, seedHeaders, func(tool, detail string) {
		emitStep(onStep, next(), tool, detail)
	})
	if err != nil {
		return AutoConfigResult{}, err
	}
	samples := feed.SampleLinks
	if len(samples) == 0 {
		return AutoConfigResult{}, fmt.Errorf("feed has no sample article links to inspect")
	}

	// ── Pre-step 1b: feed-content short-circuit ──
	// If the entries already carry full article bodies, no page scraping or LLM
	// selector discovery is needed — emit a scraping: none config and stop.
	if score := feedContentScore(feed.SampleContentChars); score >= feedContentModeThreshold {
		emitStep(onStep, next(), "feed_content", fmt.Sprintf("entries carry full content (score %.2f)", score))
		res, ferr := finishFeedContent(feedURL, feedType, lockedHeaders, score)
		if ferr != nil {
			return AutoConfigResult{}, ferr
		}
		emitStep(onStep, next(), "finish", fmt.Sprintf("scraping: none, confidence %.2f", res.Confidence))
		return res, nil
	}

	// ── Pre-step 2: lock the scraping mode ──
	lockedMode, probed := lockMode(tools, samples[0], lockedHeaders, func(detail string) {
		emitStep(onStep, next(), "probe_mode", detail)
	})

	// ── Selector discovery loop (mode + headers frozen) ──
	var transcript strings.Builder
	writeSeed(&transcript, feedURL, feedType, lockedMode, lockedHeaders, samples, probed)

	seen := map[string]bool{}
	for step < maxSteps {
		if err := ctx.Err(); err != nil {
			return AutoConfigResult{}, err
		}
		step++

		prompt := autoconfigSystemPrompt + "\n\n" + transcript.String() + "\nRespond with the next single JSON action."
		raw, err := gen(ctx, prompt)
		emitLLM(onLLM, step, prompt, raw)
		if err != nil {
			return AutoConfigResult{}, fmt.Errorf("llm call: %w", err)
		}
		action, err := parseAction(raw)
		if err != nil {
			fmt.Fprintf(&transcript, "\nASSISTANT: %s\nOBSERVATION: {\"error\":%q}\n", strings.TrimSpace(raw), err.Error())
			continue
		}
		fmt.Fprintf(&transcript, "\nASSISTANT: %s\n", compactJSON(raw))

		// Duplicate-call guard: never re-hit the server for an identical action.
		dupKey := action.Action + "|" + string(action.Args)
		if action.Action != "finish" && seen[dupKey] {
			fmt.Fprintf(&transcript, "OBSERVATION: {\"note\":\"already tried this exact call — pick a different selector or finish\"}\n")
			continue
		}
		seen[dupKey] = true

		switch action.Action {
		case "finish":
			res, ferr := finishAutoConfig(action, feedURL, feedType, lockedMode, lockedHeaders, samples, tools)
			if ferr != nil {
				fmt.Fprintf(&transcript, "OBSERVATION: {\"error\":%q}\n", ferr.Error())
				continue
			}
			emitStep(onStep, step, "finish", fmt.Sprintf("confidence %.2f", res.Confidence))
			return res, nil

		case "suggest_selectors":
			var a struct {
				ArticleURL string `json:"article_url"`
			}
			_ = json.Unmarshal(action.Args, &a)
			url := a.ArticleURL
			if url == "" {
				url = samples[0]
			}
			obs := tools.suggestSelectors(url, lockedMode, lockedHeaders)
			detail := fmt.Sprintf("%d candidates", len(obs.Candidates))
			if obs.Error != "" {
				detail = obs.Error
			}
			emitStep(onStep, step, "suggest_selectors", detail)
			appendObs(&transcript, obs)

		case "test_selector":
			var a struct {
				Article   string `json:"article"`
				Cutoff    string `json:"cutoff"`
				Blacklist string `json:"blacklist"`
			}
			_ = json.Unmarshal(action.Args, &a)
			obs := tools.testSelector(samples, lockedMode, a.Article, a.Cutoff, a.Blacklist, lockedHeaders)
			detail := fmt.Sprintf("%q score %.2f (%d/%d)", a.Article, obs.Score, obs.Usable, obs.Samples)
			if obs.Error != "" {
				detail = obs.Error
			}
			emitStep(onStep, step, "test_selector", detail)
			appendObs(&transcript, obs)

		default:
			fmt.Fprintf(&transcript, "OBSERVATION: {\"error\":\"unknown action %q\"}\n", action.Action)
		}
	}

	return AutoConfigResult{}, fmt.Errorf("agent did not converge within %d steps", maxSteps)
}

// lockHeaders inspects the feed and, if it is blocked, probes a small fixed set of
// header combinations once, returning the working headers and the feed observation
// (sample links, per-entry content lengths) from the parse that succeeded.
func lockHeaders(tools autoconfigTools, feedURL string, seed map[string]string, emit func(tool, detail string)) (map[string]string, feedObs, error) {
	obs := tools.inspectFeed(feedURL, seed)
	if obs.ParseOK {
		return seed, obs, nil
	}

	origin := originOf(feedURL)
	combos := []map[string]string{
		{"Referer": origin},
		{"Referer": origin, "User-Agent": desktopUA},
	}
	for _, c := range combos {
		h := mergeHeaders(seed, c)
		emit("probe_headers", strings.Join(sortedKeys(c), "+"))
		obs = tools.inspectFeed(feedURL, h)
		if obs.ParseOK {
			return h, obs, nil
		}
	}
	return nil, feedObs{}, fmt.Errorf("feed is blocked and no tried header set unblocked it (last verdict: %s)", obs.Verdict)
}

// lockMode probes the scraping modes in priority order against one sample article and
// returns the first that yields usable candidates. If none clears the bar it returns
// the best by candidate length (or static), so discovery can still proceed.
func lockMode(tools autoconfigTools, sampleURL string, headers map[string]string, emit func(detail string)) (string, []models.SelectorCandidate) {
	bestMode := "static"
	var bestCands []models.SelectorCandidate
	bestChars := -1

	for _, mode := range scrapeModeOrder {
		obs := tools.suggestSelectors(sampleURL, mode, headers)
		top := 0
		if len(obs.Candidates) > 0 {
			top = obs.Candidates[0].Chars
		}
		detail := fmt.Sprintf("%s: top %d chars", mode, top)
		if obs.Error != "" {
			detail = fmt.Sprintf("%s: %s", mode, obs.Error)
		}
		emit(detail)

		if obs.Error == "" && top >= autoconfigUsableChars {
			return mode, obs.Candidates // first usable mode wins
		}
		if top > bestChars {
			bestChars, bestMode, bestCands = top, mode, obs.Candidates
		}
	}
	return bestMode, bestCands
}

// writeSeed primes the transcript with everything the model needs so it can go
// straight to selector discovery without re-probing.
func writeSeed(b *strings.Builder, feedURL, feedType, mode string, headers map[string]string, samples []string, cands []models.SelectorCandidate) {
	fmt.Fprintf(b, "TASK: choose the article-content selectors for feed %s\n", feedURL)
	fmt.Fprintf(b, "FIXED: scraping mode = %q (do not change it); headers are already set (%s).\n", mode, strings.Join(sortedKeys(headers), ", "))
	fmt.Fprintf(b, "feed_type: %s\n", feedType)
	if data, err := json.Marshal(samples); err == nil {
		fmt.Fprintf(b, "sample_links: %s\n", string(data))
	}
	if data, err := json.Marshal(cands); err == nil {
		fmt.Fprintf(b, "candidate_selectors (ranked, from mode %s): %s\n", mode, string(data))
	}
	fmt.Fprintln(b, "Test candidates with test_selector across the samples, then finish with the best.")
}

// finishAutoConfig validates the proposed selectors, scores them for confidence, and
// renders the final config YAML using the LOCKED mode and headers.
func finishAutoConfig(action agentAction, feedURL, feedType, mode string, headers map[string]string, samples []string, tools autoconfigTools) (AutoConfigResult, error) {
	if action.Config == nil || strings.TrimSpace(action.Config.Selectors.Article) == "" {
		return AutoConfigResult{}, fmt.Errorf("finish requires config.selectors.article")
	}
	sel := action.Config.Selectors

	confidence := 0.0
	if len(samples) > 0 {
		n := len(samples)
		if n > 3 {
			n = 3
		}
		obs := tools.testSelector(samples[:n], mode, sel.Article, sel.Cutoff, sel.Blacklist, headers)
		confidence = obs.Score
	}

	cfg := models.FeedConfig{
		URL:     feedURL,
		Enabled: true,
		Scraper: models.ScraperConfig{
			Type:     feedType,
			Scraping: scrapingValue(mode),
			Headers:  headers,
			Selectors: &models.Selectors{
				Article:   sel.Article,
				Cutoff:    sel.Cutoff,
				Blacklist: sel.Blacklist,
			},
		},
	}
	yamlStr, err := renderConfig(cfg)
	if err != nil {
		return AutoConfigResult{}, err
	}
	return AutoConfigResult{
		ConfigYAML: yamlStr,
		Summary:    strings.TrimSpace(action.Thought),
		Confidence: confidence,
	}, nil
}

// finishFeedContent renders a scraping: none config for feeds that already ship
// full article bodies in their entries — no selectors, no page fetch. confidence
// is the fraction of sampled entries that carried a usable body.
func finishFeedContent(feedURL, feedType string, headers map[string]string, confidence float64) (AutoConfigResult, error) {
	cfg := models.FeedConfig{
		URL:     feedURL,
		Enabled: true,
		Scraper: models.ScraperConfig{
			Type:     feedType,
			Scraping: "none",
			Headers:  headers,
		},
	}
	yamlStr, err := renderConfig(cfg)
	if err != nil {
		return AutoConfigResult{}, err
	}
	return AutoConfigResult{
		ConfigYAML: yamlStr,
		Summary:    "feed entries already contain full article content; using scraping: none (no page fetch)",
		Confidence: confidence,
	}, nil
}

// renderConfig marshals a single feed config to the feeds-file YAML shape.
func renderConfig(cfg models.FeedConfig) (string, error) {
	out, err := yaml.Marshal(models.FeedsFile{Feeds: []models.FeedConfig{cfg}})
	if err != nil {
		return "", fmt.Errorf("marshal config: %w", err)
	}
	return string(out), nil
}

// feedContentScore is the fraction of sampled entries whose feed content is
// already a usable full body (>= autoconfigUsableChars), mirroring the selector
// "usable" bar so "good enough" means the same thing across the agent.
func feedContentScore(chars []int) float64 {
	if len(chars) == 0 {
		return 0
	}
	usable := 0
	for _, c := range chars {
		if c >= autoconfigUsableChars {
			usable++
		}
	}
	return float64(usable) / float64(len(chars))
}

// scrapingValue maps an internal mode name to the FeedConfig.Scraping value
// ("" for static, the mode name otherwise).
func scrapingValue(mode string) string {
	if mode == "static" {
		return ""
	}
	return mode
}

func emitStep(onStep func(autoconfigStep), n int, tool, detail string) {
	if onStep != nil {
		onStep(autoconfigStep{N: n, Tool: tool, Detail: detail})
	}
}

// emitLLM reports one turn's raw LLM prompt and response when a verbose sink is set.
func emitLLM(onLLM func(turn int, prompt, response string), turn int, prompt, response string) {
	if onLLM != nil {
		onLLM(turn, prompt, response)
	}
}

func appendObs(b *strings.Builder, obs any) {
	data, err := json.Marshal(obs)
	if err != nil {
		fmt.Fprintf(b, "OBSERVATION: {\"error\":%q}\n", err.Error())
		return
	}
	fmt.Fprintf(b, "OBSERVATION: %s\n", string(data))
}

// parseAction extracts and decodes the single JSON action object from a model reply.
func parseAction(raw string) (agentAction, error) {
	obj := extractJSONObject(raw)
	if obj == "" {
		return agentAction{}, fmt.Errorf("no JSON object found in reply")
	}
	var a agentAction
	if err := json.Unmarshal([]byte(obj), &a); err != nil {
		return agentAction{}, fmt.Errorf("invalid action JSON: %v", err)
	}
	if a.Action == "" {
		return agentAction{}, fmt.Errorf("action field is empty")
	}
	return a, nil
}

// extractJSONObject returns the first balanced top-level {...} object in s.
func extractJSONObject(s string) string {
	start := strings.IndexByte(s, '{')
	if start < 0 {
		return ""
	}
	depth, inStr, esc := 0, false, false
	for i := start; i < len(s); i++ {
		ch := s[i]
		if inStr {
			switch {
			case esc:
				esc = false
			case ch == '\\':
				esc = true
			case ch == '"':
				inStr = false
			}
			continue
		}
		switch ch {
		case '"':
			inStr = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}

// compactJSON collapses an action reply to a single line for the transcript.
func compactJSON(raw string) string {
	obj := extractJSONObject(raw)
	if obj == "" {
		return strings.TrimSpace(raw)
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, []byte(obj)); err != nil {
		return obj
	}
	return buf.String()
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func mergeHeaders(a, b map[string]string) map[string]string {
	if len(a) == 0 && len(b) == 0 {
		return nil
	}
	out := make(map[string]string, len(a)+len(b))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

// originOf returns scheme://host/ for a URL, for use as a Referer.
func originOf(rawURL string) string {
	if i := strings.Index(rawURL, "://"); i > 0 {
		rest := rawURL[i+3:]
		host := rest
		if j := strings.IndexByte(rest, '/'); j >= 0 {
			host = rest[:j]
		}
		return rawURL[:i+3] + host + "/"
	}
	return rawURL
}

// managerTools is the production autoconfigTools, backed by the global FeedManager.
type managerTools struct{}

func (managerTools) inspectFeed(url string, headers map[string]string) feedObs {
	insp := manager.Manager.InspectFeedURL(url, headers, 5)
	return feedObs{
		ParseOK:            insp.Diagnosis.ParseError == "",
		FeedType:           insp.Diagnosis.FeedTypeGuess,
		Title:              insp.Title,
		Verdict:            insp.Diagnosis.Verdict,
		SampleLinks:        insp.SampleLinks,
		SampleContentChars: insp.SampleContentChars,
	}
}

func (managerTools) suggestSelectors(url, mode string, headers map[string]string) suggestObs {
	cands, err := manager.Manager.SuggestSelectors(url, mode, headers, 8)
	if err != nil {
		return suggestObs{Error: err.Error()}
	}
	return suggestObs{Candidates: cands}
}

func (managerTools) testSelector(urls []string, mode, article, cutoff, blacklist string, headers map[string]string) testObs {
	if len(urls) == 0 || article == "" {
		return testObs{Error: "article_urls and article are required"}
	}
	sel := &models.Selectors{Article: article, Cutoff: cutoff, Blacklist: blacklist}
	var results []scrapers.ExtractResult
	var out []perURLResult
	for _, u := range urls {
		insp := manager.Manager.InspectArticle(u, mode, headers, sel, 0)
		results = append(results, scrapers.ExtractResult{URL: u, Chars: insp.ExtractedLen, Matched: insp.SelectorMatched})
		out = append(out, perURLResult{URL: u, Matched: insp.SelectorMatched, Chars: insp.ExtractedLen})
	}
	score := scrapers.ScoreSelector(results)
	return testObs{Score: score.Score, Usable: score.Usable, Samples: score.Samples, Results: out}
}
