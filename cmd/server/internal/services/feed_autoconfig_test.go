package services

import (
	"context"
	"strings"
	"testing"

	"github.com/ma111e/downlink/pkg/models"
)

// fakeTools records calls and returns canned observations, so the loop can be tested
// without a FeedManager, network, or LLM. usableModes controls which modes "work".
type fakeTools struct {
	calls       []string
	modeCalls   []string // modes passed to suggestSelectors
	usableModes map[string]bool
	blocked     bool // when true, inspectFeed only parses once a Referer header is supplied
}

func (f *fakeTools) inspectFeed(url string, headers map[string]string) feedObs {
	f.calls = append(f.calls, "inspect_feed")
	if f.blocked && headers["Referer"] == "" {
		return feedObs{ParseOK: false, Verdict: "HTTP 403"}
	}
	return feedObs{
		ParseOK:     true,
		FeedType:    "rss",
		Title:       "Example Blog",
		Verdict:     "valid rss feed, 3 items",
		SampleLinks: []string{"https://e.com/a", "https://e.com/b", "https://e.com/c"},
	}
}

func (f *fakeTools) suggestSelectors(url, mode string, headers map[string]string) suggestObs {
	f.calls = append(f.calls, "suggest_selectors")
	f.modeCalls = append(f.modeCalls, mode)
	if f.usableModes != nil && !f.usableModes[mode] {
		return suggestObs{Error: "no usable content in " + mode}
	}
	return suggestObs{Candidates: []models.SelectorCandidate{
		{Selector: "article.post", Chars: 4200, LinkDensity: 0.05, Snippet: "Real article body…"},
		{Selector: "nav.menu", Chars: 300, LinkDensity: 0.9, Snippet: "Home About…"},
	}}
}

func (f *fakeTools) testSelector(urls []string, mode, article, cutoff, blacklist string, headers map[string]string) testObs {
	f.calls = append(f.calls, "test_selector")
	if article == "article.post" {
		return testObs{Score: 1.0, Usable: len(urls), Samples: len(urls)}
	}
	return testObs{Score: 0, Usable: 0, Samples: len(urls)}
}

func scriptedGen(replies []string) autoconfigGenerate {
	i := 0
	return func(ctx context.Context, prompt string) (string, error) {
		r := replies[i]
		if i < len(replies)-1 {
			i++
		}
		return r, nil
	}
}

func TestRunAutoConfig_ProbesLocksThenDiscovers(t *testing.T) {
	// static is usable, so the probe should lock "static" after one suggest call.
	tools := &fakeTools{usableModes: map[string]bool{"static": true}}
	replies := []string{
		`{"thought":"verify","action":"test_selector","args":{"article":"article.post"}}`,
		"```json\n{\"thought\":\"article.post scored 1.0\",\"action\":\"finish\",\"config\":{\"selectors\":{\"article\":\"article.post\"}}}\n```",
	}
	var steps []autoconfigStep
	res, err := runAutoConfig(context.Background(), scriptedGen(replies), tools, "https://e.com/feed", "rss", nil, 10,
		func(st autoconfigStep) { steps = append(steps, st) })
	if err != nil {
		t.Fatalf("runAutoConfig: %v", err)
	}

	// Mode probe must have stopped at static (first usable) — never tried dynamic/full_browser.
	if len(tools.modeCalls) != 1 || tools.modeCalls[0] != "static" {
		t.Errorf("mode probe = %v, want exactly [static]", tools.modeCalls)
	}
	if res.Confidence != 1.0 {
		t.Errorf("confidence = %.2f, want 1.0", res.Confidence)
	}
	if !strings.Contains(res.ConfigYAML, "article.post") || !strings.Contains(res.ConfigYAML, "https://e.com/feed") {
		t.Errorf("unexpected config YAML:\n%s", res.ConfigYAML)
	}
	// scraping should be empty (static) in the YAML.
	if strings.Contains(res.ConfigYAML, "scraping:") {
		t.Errorf("static mode should omit scraping in YAML:\n%s", res.ConfigYAML)
	}
}

func TestRunAutoConfig_EscalatesModeWhenStaticFails(t *testing.T) {
	// Only full_browser yields content → probe must escalate static→dynamic→full_browser.
	tools := &fakeTools{usableModes: map[string]bool{"full_browser": true}}
	replies := []string{
		`{"action":"test_selector","args":{"article":"article.post"}}`,
		`{"action":"finish","config":{"selectors":{"article":"article.post"}}}`,
	}
	res, err := runAutoConfig(context.Background(), scriptedGen(replies), tools, "https://e.com/feed", "rss", nil, 10, nil)
	if err != nil {
		t.Fatalf("runAutoConfig: %v", err)
	}
	want := []string{"static", "dynamic", "full_browser"}
	if strings.Join(tools.modeCalls, ",") != strings.Join(want, ",") {
		t.Errorf("mode probe = %v, want %v", tools.modeCalls, want)
	}
	if !strings.Contains(res.ConfigYAML, "scraping: full_browser") {
		t.Errorf("expected scraping: full_browser in YAML:\n%s", res.ConfigYAML)
	}
}

func TestRunAutoConfig_ProbesHeadersWhenBlocked(t *testing.T) {
	tools := &fakeTools{blocked: true, usableModes: map[string]bool{"static": true}}
	replies := []string{
		`{"action":"finish","config":{"selectors":{"article":"article.post"}}}`,
	}
	res, err := runAutoConfig(context.Background(), scriptedGen(replies), tools, "https://e.com/feed", "rss", nil, 10, nil)
	if err != nil {
		t.Fatalf("runAutoConfig: %v", err)
	}
	// The locked headers (with Referer) must appear in the final config.
	if !strings.Contains(res.ConfigYAML, "Referer") {
		t.Errorf("expected probed Referer header in YAML:\n%s", res.ConfigYAML)
	}
}

func TestRunAutoConfig_DuplicateCallGuard(t *testing.T) {
	tools := &fakeTools{usableModes: map[string]bool{"static": true}}
	// The model emits the same test_selector twice; the second must be short-circuited.
	replies := []string{
		`{"action":"test_selector","args":{"article":"div.x"}}`,
		`{"action":"test_selector","args":{"article":"div.x"}}`,
		`{"action":"finish","config":{"selectors":{"article":"article.post"}}}`,
	}
	_, err := runAutoConfig(context.Background(), scriptedGen(replies), tools, "https://e.com/feed", "rss", nil, 10, nil)
	if err != nil {
		t.Fatalf("runAutoConfig: %v", err)
	}
	// test_selector("div.x") runs once from the loop; the duplicate is suppressed.
	// One more test_selector runs at finish-confirm (article.post). So exactly 2 total.
	n := 0
	for _, c := range tools.calls {
		if c == "test_selector" {
			n++
		}
	}
	if n != 2 {
		t.Errorf("test_selector call count = %d, want 2 (duplicate suppressed + finish-confirm)", n)
	}
}

func TestRunAutoConfig_BudgetExhausted(t *testing.T) {
	tools := &fakeTools{usableModes: map[string]bool{"static": true}}
	gen := func(ctx context.Context, prompt string) (string, error) {
		return `{"action":"suggest_selectors","args":{"article_url":"https://e.com/a"}}`, nil
	}
	_, err := runAutoConfig(context.Background(), gen, tools, "https://e.com/feed", "rss", nil, 3, nil)
	if err == nil || !strings.Contains(err.Error(), "did not converge") {
		t.Fatalf("expected non-convergence error, got %v", err)
	}
}

func TestExtractJSONObject(t *testing.T) {
	in := "sure, here:\n```json\n{\"a\":{\"b\":\"}\"},\"c\":1}\n```\ntrailing"
	got := extractJSONObject(in)
	want := `{"a":{"b":"}"},"c":1}`
	if got != want {
		t.Errorf("extractJSONObject = %q, want %q", got, want)
	}
}
