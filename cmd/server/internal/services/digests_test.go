package services

import (
	"github.com/ma111e/downlink/pkg/models"
	"github.com/ma111e/downlink/pkg/protos"
	"strings"
	"testing"
	"time"
)

func TestResolveRunProviderModel(t *testing.T) {
	ded := EffectiveEditorial{Provider: "default-provider", Model: "default-model"}

	tests := []struct {
		name         string
		req          *protos.GenerateDigestRequest
		wantProvider string
		wantModel    string
	}{
		{
			name:         "model only honors override without provider",
			req:          &protos.GenerateDigestRequest{Model: "gpt-x"},
			wantProvider: "",
			wantModel:    "gpt-x",
		},
		{
			name:         "provider only",
			req:          &protos.GenerateDigestRequest{Provider: "openai"},
			wantProvider: "openai",
			wantModel:    "",
		},
		{
			name:         "both set",
			req:          &protos.GenerateDigestRequest{Provider: "openai", Model: "gpt-x"},
			wantProvider: "openai",
			wantModel:    "gpt-x",
		},
		{
			name:         "neither set falls back to defaults",
			req:          &protos.GenerateDigestRequest{},
			wantProvider: "default-provider",
			wantModel:    "default-model",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotProvider, gotModel := resolveRunProviderModel(tt.req, ded)
			if gotProvider != tt.wantProvider || gotModel != tt.wantModel {
				t.Errorf("resolveRunProviderModel() = (%q, %q), want (%q, %q)",
					gotProvider, gotModel, tt.wantProvider, tt.wantModel)
			}
		})
	}
}

func TestBuildDigestSummaryPromptIncludesWindowAndArticles(t *testing.T) {
	windowStart := time.Date(2026, 4, 29, 8, 30, 0, 0, time.FixedZone("CEST", 2*60*60))
	windowEnd := windowStart.Add(24 * time.Hour)
	analyses := []models.ArticleAnalysis{
		{
			ArticleId: "article-1",
			KeyPoints: []string{
				"Critical vulnerability exploited in the wild",
				"Patch guidance published by the vendor",
			},
		},
	}
	articleMap := map[string]models.Article{
		"article-1": {
			Title: "Campaign Targets Edge Devices",
			Link:  "https://example.test/report",
		},
	}

	prompt := buildDigestSummaryPrompt(analyses, articleMap, windowStart, windowEnd, true, EffectiveEditorial{})

	wantSubstrings := []string{
		"Digest coverage window",
		"Start: 2026-04-29T06:30:00Z",
		"End: 2026-04-30T06:30:00Z",
		"Duration: 24h0m0s",
		"Campaign Targets Edge Devices",
		"Source: https://example.test/report",
		"Critical vulnerability exploited in the wild",
		"Patch guidance published by the vendor",
		"do not imply the reported events occurred exactly within the window",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}
