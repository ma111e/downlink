package services

import (
	"context"
	"strings"
	"testing"

	"github.com/ma111e/downlink/pkg/protos"
)

func TestBackfillFeedTopics(t *testing.T) {
	// Fake gen records every prompt and replies with a fixed topic set.
	var prompts []string
	gen := func(_ context.Context, prompt string) (string, error) {
		prompts = append(prompts, prompt)
		return `{"topics": ["Malware", " news "]}`, nil
	}
	// Fake sampler: titles for feed-a, nothing for feed-b (title-only path).
	sample := func(url string, title *string) []string {
		if url == "https://a.example/rss" {
			return []string{"New ransomware strain hits hospitals"}
		}
		if title != nil && *title == "" {
			*title = "detected B"
		}
		return nil
	}

	feeds := []*protos.FeedTopicTarget{
		{Url: "https://a.example/rss", Title: "Feed A"},
		{Url: "https://b.example/rss"}, // no title; sampler fills it
	}

	var events []*protos.BackfillFeedTopicsEvent
	send := func(ev *protos.BackfillFeedTopicsEvent) error {
		events = append(events, ev)
		return nil
	}

	if err := backfillFeedTopics(context.Background(), gen, sample, []string{"privacy"}, feeds, send); err != nil {
		t.Fatalf("backfillFeedTopics: %v", err)
	}

	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	// Topics are normalized (lowercased, trimmed) by extractTopics.
	for i, ev := range events {
		if ev.Index != uint32(i+1) || ev.Total != 2 {
			t.Errorf("event %d index/total = %d/%d", i, ev.Index, ev.Total)
		}
		if len(ev.Topics) != 2 || ev.Topics[0] != "malware" || ev.Topics[1] != "news" {
			t.Errorf("event %d topics = %v, want [malware news]", i, ev.Topics)
		}
	}

	// The seed vocabulary ("privacy") reaches the first prompt, and topics derived
	// for feed A ("malware", "news") are fed into feed B's prompt vocabulary.
	if !strings.Contains(prompts[0], "privacy") {
		t.Errorf("seed vocabulary not in first prompt:\n%s", prompts[0])
	}
	if !strings.Contains(prompts[1], "malware") || !strings.Contains(prompts[1], "news") {
		t.Errorf("batch-derived topics not carried into second prompt:\n%s", prompts[1])
	}
}

func TestBackfillFeedTopicsNoTopicsDerived(t *testing.T) {
	gen := func(_ context.Context, _ string) (string, error) { return `{"topics": []}`, nil }
	sample := func(string, *string) []string { return nil }
	feeds := []*protos.FeedTopicTarget{{Url: "https://x.example/rss", Title: "X"}}

	var got *protos.BackfillFeedTopicsEvent
	send := func(ev *protos.BackfillFeedTopicsEvent) error { got = ev; return nil }
	if err := backfillFeedTopics(context.Background(), gen, sample, nil, feeds, send); err != nil {
		t.Fatalf("backfillFeedTopics: %v", err)
	}
	if got == nil || len(got.Topics) != 0 || got.Error == "" {
		t.Errorf("expected an error event with no topics, got %+v", got)
	}
}
