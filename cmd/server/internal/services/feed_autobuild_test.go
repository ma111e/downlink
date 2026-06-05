package services

import (
	"context"
	"strings"
	"testing"

	"github.com/ma111e/downlink/pkg/models"
)

// fakeTools records calls and returns canned observations, so the loop can be tested
// without a FeedManager, network, or LLM.
type fakeTools struct {
	calls []string
}

func (f *fakeTools) inspectFeed(url string, headers map[string]string) feedObs {
	f.calls = append(f.calls, "inspect_feed")
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

// scriptedGen returns canned model replies in order, ignoring the prompt.
func scriptedGen(replies []string) autobuildGenerate {
	i := 0
	return func(ctx context.Context, prompt string) (string, error) {
		r := replies[i]
		if i < len(replies)-1 {
			i++
		}
		return r, nil
	}
}

func TestRunAutoBuild_HappyPath(t *testing.T) {
	replies := []string{
		`{"thought":"check feed","action":"inspect_feed","args":{}}`,
		`{"thought":"rank","action":"suggest_selectors","args":{"article_url":"https://e.com/a","mode":"static"}}`,
		`{"thought":"verify","action":"test_selector","args":{"article_urls":["https://e.com/a","https://e.com/b"],"mode":"static","article":"article.post"}}`,
		"```json\n{\"thought\":\"article.post scored 1.0\",\"action\":\"finish\",\"config\":{\"scraping\":\"\",\"selectors\":{\"article\":\"article.post\"}}}\n```",
	}
	tools := &fakeTools{}
	var steps []string
	res, err := runAutoBuild(context.Background(), scriptedGen(replies), tools, "https://e.com/feed", "rss", 10,
		func(st autobuildStep) { steps = append(steps, st.Tool) })
	if err != nil {
		t.Fatalf("runAutoBuild: %v", err)
	}

	wantCalls := []string{"inspect_feed", "suggest_selectors", "test_selector", "test_selector"} // last is finish-confirm
	if strings.Join(tools.calls, ",") != strings.Join(wantCalls, ",") {
		t.Errorf("tool calls = %v, want %v", tools.calls, wantCalls)
	}
	if res.Confidence != 1.0 {
		t.Errorf("confidence = %.2f, want 1.0", res.Confidence)
	}
	if !strings.Contains(res.ConfigYAML, "article.post") {
		t.Errorf("config YAML missing selector:\n%s", res.ConfigYAML)
	}
	if !strings.Contains(res.ConfigYAML, "https://e.com/feed") {
		t.Errorf("config YAML missing url:\n%s", res.ConfigYAML)
	}
}

func TestRunAutoBuild_RejectsFinishWithoutSelector_ThenRecovers(t *testing.T) {
	replies := []string{
		`{"action":"finish","config":{"selectors":{"article":""}}}`, // invalid: no article → rejected
		`{"action":"inspect_feed","args":{}}`,
		`{"action":"finish","config":{"selectors":{"article":"article.post"}}}`,
	}
	tools := &fakeTools{}
	res, err := runAutoBuild(context.Background(), scriptedGen(replies), tools, "https://e.com/feed", "rss", 10, nil)
	if err != nil {
		t.Fatalf("runAutoBuild: %v", err)
	}
	if !strings.Contains(res.ConfigYAML, "article.post") {
		t.Errorf("expected recovery to a valid config, got:\n%s", res.ConfigYAML)
	}
}

func TestRunAutoBuild_BudgetExhausted(t *testing.T) {
	// Always asks to inspect; never finishes.
	gen := func(ctx context.Context, prompt string) (string, error) {
		return `{"action":"inspect_feed","args":{}}`, nil
	}
	_, err := runAutoBuild(context.Background(), gen, &fakeTools{}, "https://e.com/feed", "rss", 3, nil)
	if err == nil {
		t.Fatal("expected an error when the agent never converges")
	}
	if !strings.Contains(err.Error(), "did not converge") {
		t.Errorf("error = %v, want a non-convergence message", err)
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
