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
	fullContent bool // when true, every sampled entry already carries a full body

	feedText string // per-entry feed content returned by inspectFeed (when set)
	pageText string // page main text returned by articleText (when set)

	existing []string // vocabulary returned by existingTopics
}

func (f *fakeTools) existingTopics() []string {
	f.calls = append(f.calls, "existing_topics")
	return f.existing
}

// longText is comfortably over feedContentFullChars and rich enough to form many
// distinct trigrams, so coverage matching is meaningful.
var longText = strings.Repeat("the quick brown fox jumps over the lazy dog and then ", 120)

func (f *fakeTools) inspectFeed(url string, headers map[string]string) feedObs {
	f.calls = append(f.calls, "inspect_feed")
	if f.blocked && headers["Referer"] == "" {
		return feedObs{ParseOK: false, Verdict: "HTTP 403"}
	}
	obs := feedObs{
		ParseOK:     true,
		FeedType:    "rss",
		Title:       "Example Blog",
		Verdict:     "valid rss feed, 3 items",
		SampleLinks: []string{"https://e.com/a", "https://e.com/b", "https://e.com/c"},
	}
	if f.fullContent {
		obs.SampleContent = []string{longText, longText, longText}
	}
	if f.feedText != "" {
		obs.SampleContent = []string{f.feedText, f.feedText, f.feedText}
	}
	for _, c := range obs.SampleContent {
		obs.SampleContentChars = append(obs.SampleContentChars, len([]rune(c)))
	}
	return obs
}

func (f *fakeTools) articleText(url, mode string, headers map[string]string) (string, error) {
	f.calls = append(f.calls, "article_text")
	if f.pageText != "" {
		return f.pageText, nil
	}
	// Default: the page body equals the feed text, so a full feed entry matches.
	return longText, nil
}

func (f *fakeTools) inspectIndex(url string, headers map[string]string) indexObs {
	f.calls = append(f.calls, "inspect_index")
	if f.blocked && headers["Referer"] == "" {
		return indexObs{OK: false, Verdict: "HTTP 403"}
	}
	return indexObs{OK: true, Verdict: "HTTP 200 — html body"}
}

func (f *fakeTools) suggestLinkSelectors(url, mode string, headers map[string]string) linkListObs {
	f.calls = append(f.calls, "suggest_link_selectors")
	f.modeCalls = append(f.modeCalls, mode)
	if f.usableModes != nil && !f.usableModes[mode] {
		return linkListObs{Error: "no repeating list in " + mode}
	}
	return linkListObs{Candidates: []models.LinkListCandidate{{
		LinksSelector: "article.card a",
		Count:         4,
		SampleHrefs:   []string{"https://e.com/blog/a", "https://e.com/blog/b", "https://e.com/blog/c"},
		DateSelector:  "time",
		URLFilter:     "/blog/",
	}}}
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
	res, err := runAutoConfig(context.Background(), scriptedGen(replies), tools, "https://e.com/feed", "rss", nil, 10, false,
		func(st autoconfigStep) { steps = append(steps, st) }, nil)
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

func TestRunAutoConfig_HTMLDiscoversLinkListThenSelectors(t *testing.T) {
	// An html index page: the deterministic pre-phase locks the link list (and its
	// date selector), then the LLM loop picks the article-body selector for the posts.
	tools := &fakeTools{usableModes: map[string]bool{"static": true}}
	replies := []string{
		`{"action":"finish","config":{"selectors":{"article":"article.post"}}}`,
	}
	res, err := runAutoConfig(context.Background(), scriptedGen(replies), tools, "https://e.com/blog/", "html", nil, 10, false, nil, nil)
	if err != nil {
		t.Fatalf("runAutoConfig: %v", err)
	}

	yaml := res.ConfigYAML
	for _, want := range []string{
		"type: html",
		"links_selector: article.card a",
		"date_selector: time",
		"url_filter: /blog/",
		"article: article.post",
	} {
		if !strings.Contains(yaml, want) {
			t.Errorf("config YAML missing %q:\n%s", want, yaml)
		}
	}
	if res.Confidence != 1.0 {
		t.Errorf("confidence = %.2f, want 1.0", res.Confidence)
	}
}

func TestRunAutoConfig_HTMLNoListErrors(t *testing.T) {
	// No mode yields a repeating link list → the html path reports it rather than
	// emitting a bogus config.
	tools := &fakeTools{usableModes: map[string]bool{}} // every mode returns an error
	_, err := runAutoConfig(context.Background(), scriptedGen([]string{`{}`}), tools, "https://e.com/blog/", "html", nil, 10, false, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "no repeating post-link list") {
		t.Fatalf("expected no-list error, got %v", err)
	}
}

func TestRunAutoConfig_DetectsFullContentInDescription(t *testing.T) {
	// Every entry already carries a full body → short-circuit to scraping: none,
	// with no mode probing and no LLM call.
	tools := &fakeTools{fullContent: true, usableModes: map[string]bool{"static": true}}
	gen := func(ctx context.Context, prompt string) (string, error) {
		t.Fatalf("LLM must not be called when feed content is full")
		return "", nil
	}
	var steps []autoconfigStep
	res, err := runAutoConfig(context.Background(), gen, tools, "https://e.com/feed", "rss", nil, 10, false,
		func(st autoconfigStep) { steps = append(steps, st) }, nil)
	if err != nil {
		t.Fatalf("runAutoConfig: %v", err)
	}

	if len(tools.modeCalls) != 0 {
		t.Errorf("mode probe = %v, want none (content already full)", tools.modeCalls)
	}
	if !strings.Contains(res.ConfigYAML, "scraping: none") {
		t.Errorf("expected scraping: none in YAML:\n%s", res.ConfigYAML)
	}
	if strings.Contains(res.ConfigYAML, "selectors:") {
		t.Errorf("scraping: none config must omit selectors:\n%s", res.ConfigYAML)
	}
	if res.Confidence != 1.0 {
		t.Errorf("confidence = %.2f, want 1.0", res.Confidence)
	}
}

func TestRunAutoConfig_LongTeaserIsNotFullContent(t *testing.T) {
	// Entries clear the char bar but the page holds far more text than the feed, so
	// the entry is a long teaser, not a full body. Autoconfig must NOT short-circuit:
	// it should probe a mode and run selector discovery.
	feed := strings.Repeat("alpha beta gamma delta ", 200)             // long, but a teaser
	page := feed + strings.Repeat("one two three four five six ", 400) // page has much more
	tools := &fakeTools{
		feedText:    feed,
		pageText:    page,
		usableModes: map[string]bool{"static": true},
	}
	replies := []string{
		`{"action":"finish","config":{"selectors":{"article":"article.post"}}}`,
	}
	res, err := runAutoConfig(context.Background(), scriptedGen(replies), tools, "https://e.com/feed", "rss", nil, 10, false, nil, nil)
	if err != nil {
		t.Fatalf("runAutoConfig: %v", err)
	}
	if strings.Contains(res.ConfigYAML, "scraping: none") {
		t.Errorf("long teaser must not short-circuit to scraping: none:\n%s", res.ConfigYAML)
	}
	if len(tools.modeCalls) == 0 {
		t.Errorf("expected mode probing for a teaser feed, got none")
	}
}

func TestRunAutoConfig_EscalatesModeWhenStaticFails(t *testing.T) {
	// Only full_browser yields content → probe must escalate static→dynamic→full_browser.
	tools := &fakeTools{usableModes: map[string]bool{"full_browser": true}}
	replies := []string{
		`{"action":"test_selector","args":{"article":"article.post"}}`,
		`{"action":"finish","config":{"selectors":{"article":"article.post"}}}`,
	}
	res, err := runAutoConfig(context.Background(), scriptedGen(replies), tools, "https://e.com/feed", "rss", nil, 10, false, nil, nil)
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
	res, err := runAutoConfig(context.Background(), scriptedGen(replies), tools, "https://e.com/feed", "rss", nil, 10, false, nil, nil)
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
	_, err := runAutoConfig(context.Background(), scriptedGen(replies), tools, "https://e.com/feed", "rss", nil, 10, false, nil, nil)
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

func TestRunAutoConfig_EmitsLLMIO(t *testing.T) {
	tools := &fakeTools{usableModes: map[string]bool{"static": true}}
	replies := []string{
		`{"action":"test_selector","args":{"article":"article.post"}}`,
		`{"action":"finish","config":{"selectors":{"article":"article.post"}}}`,
	}
	type io struct {
		turn     int
		prompt   string
		response string
	}
	var ios []io
	onLLM := func(turn int, prompt, response string) {
		ios = append(ios, io{turn, prompt, response})
	}
	_, err := runAutoConfig(context.Background(), scriptedGen(replies), tools, "https://e.com/feed", "rss", nil, 10, false, nil, onLLM)
	if err != nil {
		t.Fatalf("runAutoConfig: %v", err)
	}

	// One LLM_IO per turn, in order, carrying the system prompt and the scripted reply.
	if len(ios) != len(replies) {
		t.Fatalf("onLLM calls = %d, want %d", len(ios), len(replies))
	}
	for i, got := range ios {
		if !strings.Contains(got.prompt, "autonomous feed-configuration agent") {
			t.Errorf("turn %d prompt missing system prompt:\n%s", i, got.prompt)
		}
		if got.response != replies[i] {
			t.Errorf("turn %d response = %q, want %q", i, got.response, replies[i])
		}
	}
}

func TestRunAutoConfig_BudgetExhausted(t *testing.T) {
	tools := &fakeTools{usableModes: map[string]bool{"static": true}}
	gen := func(ctx context.Context, prompt string) (string, error) {
		return `{"action":"suggest_selectors","args":{"article_url":"https://e.com/a"}}`, nil
	}
	_, err := runAutoConfig(context.Background(), gen, tools, "https://e.com/feed", "rss", nil, 3, false, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "did not converge") {
		t.Fatalf("expected non-convergence error, got %v", err)
	}
}

func TestExtractTopics(t *testing.T) {
	t.Run("parses, cleans, and normalizes", func(t *testing.T) {
		gen := func(ctx context.Context, prompt string) (string, error) {
			// Noise the cleaner must strip, plus mixed case / dupes / blanks to normalize.
			return "<think>hmm</think>\n```json\n{\"topics\":[\"Threat Intelligence\",\" malware \",\"\",\"malware\"]}\n```", nil
		}
		got := extractTopics(context.Background(), gen, "Example Blog", nil, nil)
		want := []string{"threat intelligence", "malware"}
		if strings.Join(got, "|") != strings.Join(want, "|") {
			t.Errorf("extractTopics = %v, want %v", got, want)
		}
	})

	t.Run("existing vocabulary reaches the prompt", func(t *testing.T) {
		var seenPrompt string
		gen := func(ctx context.Context, prompt string) (string, error) {
			seenPrompt = prompt
			return `{"topics":["vulnerabilities"]}`, nil
		}
		extractTopics(context.Background(), gen, "Blog", []string{"entry body text"}, []string{"vulnerabilities", "privacy"})
		if !strings.Contains(seenPrompt, "vulnerabilities") || !strings.Contains(seenPrompt, "entry body text") {
			t.Errorf("prompt missing existing vocab or sample content:\n%s", seenPrompt)
		}
	})

	t.Run("garbage yields nil", func(t *testing.T) {
		gen := func(ctx context.Context, prompt string) (string, error) { return "not json at all", nil }
		if got := extractTopics(context.Background(), gen, "Blog", nil, nil); got != nil {
			t.Errorf("expected nil for unparseable reply, got %v", got)
		}
	})
}

func TestRunAutoConfig_ExtractsTopics(t *testing.T) {
	tools := &fakeTools{usableModes: map[string]bool{"static": true}, existing: []string{"malware"}}
	// First reply is the topic-extraction call, then the selector-discovery turns.
	replies := []string{
		`{"topics":["malware","threat-intelligence"]}`,
		`{"action":"finish","config":{"selectors":{"article":"article.post"}}}`,
	}
	res, err := runAutoConfig(context.Background(), scriptedGen(replies), tools, "https://e.com/feed", "rss", nil, 10, true, nil, nil)
	if err != nil {
		t.Fatalf("runAutoConfig: %v", err)
	}
	if !strings.Contains(res.ConfigYAML, "topics:") || !strings.Contains(res.ConfigYAML, "threat-intelligence") {
		t.Errorf("expected topics in YAML:\n%s", res.ConfigYAML)
	}
}

func TestRunAutoConfig_NoTopicsSkipsExtraction(t *testing.T) {
	tools := &fakeTools{usableModes: map[string]bool{"static": true}, existing: []string{"malware"}}
	replies := []string{
		`{"action":"finish","config":{"selectors":{"article":"article.post"}}}`,
	}
	res, err := runAutoConfig(context.Background(), scriptedGen(replies), tools, "https://e.com/feed", "rss", nil, 10, false, nil, nil)
	if err != nil {
		t.Fatalf("runAutoConfig: %v", err)
	}
	if strings.Contains(res.ConfigYAML, "topics:") {
		t.Errorf("topics must be absent when extraction is disabled:\n%s", res.ConfigYAML)
	}
	for _, c := range tools.calls {
		if c == "existing_topics" {
			t.Errorf("existingTopics must not be called when topics are disabled")
		}
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
