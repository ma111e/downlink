package services

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ma111e/downlink/cmd/server/internal/manager"
	"github.com/ma111e/downlink/cmd/server/internal/scrapers"
	"github.com/ma111e/downlink/pkg/models"

	"gopkg.in/yaml.v3"
)

//go:embed feed_autobuild_prompt.md
var autobuildSystemPrompt string

// defaultAutobuildSteps caps how many tool calls the agent may make before it must
// finish, bounding both runtime and token spend.
const defaultAutobuildSteps = 24

// autobuildGenerate is the single LLM entry point the agent needs: prompt in, text out.
// The handler wires it to the throttled gateway; tests inject a scripted function.
type autobuildGenerate func(ctx context.Context, prompt string) (string, error)

// autobuildTools is the set of side-effecting capabilities the agent drives. The
// production implementation (managerTools) calls the FeedManager; tests can fake it.
type autobuildTools interface {
	inspectFeed(url string, headers map[string]string) feedObs
	suggestSelectors(url, mode string, headers map[string]string) suggestObs
	testSelector(urls []string, mode, article, cutoff, blacklist string, headers map[string]string) testObs
}

type feedObs struct {
	ParseOK     bool     `json:"parse_ok"`
	FeedType    string   `json:"feed_type"`
	Title       string   `json:"title"`
	Verdict     string   `json:"verdict"`
	SampleLinks []string `json:"sample_links"`
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

// AutoBuildResult is the agent's final output.
type AutoBuildResult struct {
	ConfigYAML string
	Summary    string
	Confidence float64
}

// autobuildStep reports one tool call for streaming.
type autobuildStep struct {
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
	Scraping  string            `json:"scraping"`
	Headers   map[string]string `json:"headers"`
	Selectors struct {
		Article   string `json:"article"`
		Cutoff    string `json:"cutoff"`
		Blacklist string `json:"blacklist"`
	} `json:"selectors"`
}

// runAutoBuild executes the agent loop for one feed URL. onStep, when non-nil, is
// called once per tool invocation so callers can stream progress.
func runAutoBuild(
	ctx context.Context,
	gen autobuildGenerate,
	tools autobuildTools,
	feedURL, feedType string,
	maxSteps int,
	onStep func(autobuildStep),
) (AutoBuildResult, error) {
	if maxSteps <= 0 {
		maxSteps = defaultAutobuildSteps
	}

	var transcript strings.Builder
	fmt.Fprintf(&transcript, "TASK: Build a downlink feed config for the feed URL: %s\n", feedURL)

	var lastSamples []string // most recent sample_links, for finish-time confidence

	for step := 1; step <= maxSteps; step++ {
		if err := ctx.Err(); err != nil {
			return AutoBuildResult{}, err
		}

		prompt := autobuildSystemPrompt + "\n\n" + transcript.String() +
			"\nRespond with the next single JSON action."
		raw, err := gen(ctx, prompt)
		if err != nil {
			return AutoBuildResult{}, fmt.Errorf("llm call: %w", err)
		}

		action, err := parseAction(raw)
		if err != nil {
			// Nudge the model back onto protocol rather than aborting.
			fmt.Fprintf(&transcript, "\nASSISTANT: %s\nOBSERVATION: {\"error\":%q}\n", strings.TrimSpace(raw), err.Error())
			continue
		}
		fmt.Fprintf(&transcript, "\nASSISTANT: %s\n", compactJSON(raw))

		switch action.Action {
		case "finish":
			res, ferr := finishAutoBuild(action, feedURL, feedType, lastSamples, tools)
			if ferr != nil {
				fmt.Fprintf(&transcript, "OBSERVATION: {\"error\":%q}\n", ferr.Error())
				continue
			}
			if onStep != nil {
				onStep(autobuildStep{N: step, Tool: "finish", Detail: fmt.Sprintf("confidence %.2f", res.Confidence)})
			}
			return res, nil

		case "inspect_feed":
			var a struct {
				Headers map[string]string `json:"headers"`
			}
			_ = json.Unmarshal(action.Args, &a)
			obs := tools.inspectFeed(feedURL, a.Headers)
			if len(obs.SampleLinks) > 0 {
				lastSamples = obs.SampleLinks
			}
			emitStep(onStep, step, "inspect_feed", obs.Verdict)
			appendObs(&transcript, obs)

		case "suggest_selectors":
			var a struct {
				ArticleURL string            `json:"article_url"`
				Mode       string            `json:"mode"`
				Headers    map[string]string `json:"headers"`
			}
			_ = json.Unmarshal(action.Args, &a)
			obs := tools.suggestSelectors(a.ArticleURL, a.Mode, a.Headers)
			detail := fmt.Sprintf("%d candidates (%s)", len(obs.Candidates), a.Mode)
			if obs.Error != "" {
				detail = obs.Error
			}
			emitStep(onStep, step, "suggest_selectors", detail)
			appendObs(&transcript, obs)

		case "test_selector":
			var a struct {
				ArticleURLs []string          `json:"article_urls"`
				Mode        string            `json:"mode"`
				Article     string            `json:"article"`
				Cutoff      string            `json:"cutoff"`
				Blacklist   string            `json:"blacklist"`
				Headers     map[string]string `json:"headers"`
			}
			_ = json.Unmarshal(action.Args, &a)
			obs := tools.testSelector(a.ArticleURLs, a.Mode, a.Article, a.Cutoff, a.Blacklist, a.Headers)
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

	return AutoBuildResult{}, fmt.Errorf("agent did not converge within %d steps", maxSteps)
}

// finishAutoBuild validates the proposed config, computes a confidence score by
// re-testing the chosen selectors across the sample articles, and renders YAML.
func finishAutoBuild(action agentAction, feedURL, feedType string, samples []string, tools autobuildTools) (AutoBuildResult, error) {
	if action.Config == nil || strings.TrimSpace(action.Config.Selectors.Article) == "" {
		return AutoBuildResult{}, fmt.Errorf("finish requires config.selectors.article")
	}
	c := action.Config

	confidence := 0.0
	if len(samples) > 0 {
		n := len(samples)
		if n > 3 {
			n = 3
		}
		obs := tools.testSelector(samples[:n], c.Scraping, c.Selectors.Article, c.Selectors.Cutoff, c.Selectors.Blacklist, c.Headers)
		confidence = obs.Score
	}

	cfg := models.FeedConfig{
		URL:      feedURL,
		Type:     feedType,
		Enabled:  true,
		Scraping: c.Scraping,
		Headers:  c.Headers,
		Selectors: &models.Selectors{
			Article:   c.Selectors.Article,
			Cutoff:    c.Selectors.Cutoff,
			Blacklist: c.Selectors.Blacklist,
		},
	}
	out, err := yaml.Marshal(models.FeedsFile{Feeds: []models.FeedConfig{cfg}})
	if err != nil {
		return AutoBuildResult{}, fmt.Errorf("marshal config: %w", err)
	}

	return AutoBuildResult{
		ConfigYAML: string(out),
		Summary:    strings.TrimSpace(action.Thought),
		Confidence: confidence,
	}, nil
}

func emitStep(onStep func(autobuildStep), n int, tool, detail string) {
	if onStep != nil {
		onStep(autobuildStep{N: n, Tool: tool, Detail: detail})
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

// parseAction extracts and decodes the single JSON action object from a model reply,
// tolerating surrounding prose or ```json fences.
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

// extractJSONObject returns the first balanced top-level {...} object in s (string
// literals respected), or "" when there is none.
func extractJSONObject(s string) string {
	start := strings.IndexByte(s, '{')
	if start < 0 {
		return ""
	}
	depth := 0
	inStr := false
	esc := false
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

// managerTools is the production autobuildTools, backed by the global FeedManager.
type managerTools struct{}

func (managerTools) inspectFeed(url string, headers map[string]string) feedObs {
	insp := manager.Manager.InspectFeedURL(url, headers, 5)
	return feedObs{
		ParseOK:     insp.Diagnosis.ParseError == "",
		FeedType:    insp.Diagnosis.FeedTypeGuess,
		Title:       insp.Title,
		Verdict:     insp.Diagnosis.Verdict,
		SampleLinks: insp.SampleLinks,
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
