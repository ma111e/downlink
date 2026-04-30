package notification

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"downlink/pkg/models"
)

func TestDiscordDigestUsesExecutiveOverviewOnly(t *testing.T) {
	count := 2
	digest := models.Digest{
		Id:           "digest-discord",
		CreatedAt:    time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC),
		ArticleCount: &count,
		TimeWindow:   24 * time.Hour,
		DigestSummary: "## Executive Overview\n\n" +
			strings.Repeat("Executive signal. ", 120) +
			"\n\n## Technical Details\n\nDo not include this section.",
		ProviderResults: []models.DigestProviderResult{
			{ProviderType: "openai", ModelName: "gpt-test", StandardSynthesis: "Do not include provider synthesis."},
		},
		Articles: []models.Article{
			{Title: "Linked Article", Link: "https://example.com/article", Content: "<p>Do not include article text.</p>"},
		},
	}

	embeds := NewDiscordNotifier("https://discord.test/webhook").buildEmbeds(digest)
	if len(embeds) != 1 {
		t.Fatalf("len(embeds) = %d, want 1", len(embeds))
	}

	description := embeds[0].Description
	for _, unwanted := range []string{
		"https://example.com/article",
		"Do not include article text.",
		"Do not include provider synthesis.",
		"Do not include this section.",
	} {
		if strings.Contains(description, unwanted) {
			t.Fatalf("description contains %q: %s", unwanted, description)
		}
	}
	if !strings.Contains(description, "Executive signal.") {
		t.Fatalf("description did not include executive overview: %s", description)
	}
	if got := utf8.RuneCountInString(description); got > discordExecutiveOverviewMaxRunes {
		t.Fatalf("description length = %d, want <= %d", got, discordExecutiveOverviewMaxRunes)
	}
}

func TestDiscordDigestHandlesNilArticleCount(t *testing.T) {
	digest := models.Digest{
		Id:            "digest-no-count",
		CreatedAt:     time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC),
		TimeWindow:    2 * time.Hour,
		DigestSummary: "Short executive overview.",
	}

	embeds := NewDiscordNotifier("https://discord.test/webhook").buildEmbeds(digest)
	if len(embeds) != 1 || len(embeds[0].Fields) == 0 || embeds[0].Fields[0].Value != "0" {
		t.Fatalf("article count field = %+v", embeds)
	}
}

func TestExecutiveOverviewBriefFallsBackToFirstSection(t *testing.T) {
	got := executiveOverviewBrief("## Main Themes\n\nUse this brief.\n\n## Details\n\nDo not include.")
	if strings.Contains(got, "Do not include.") || got != "Use this brief." {
		t.Fatalf("executiveOverviewBrief() = %q", got)
	}
}
