package notification

import (
	"time"

	"github.com/ma111e/downlink/pkg/models"
	"github.com/ma111e/downlink/pkg/scoring"
)

// SampleDigest builds a small, fully-populated digest fixture for rendering
// without a database. It backs both the test suite and the digest dev server.
//
// article-b scores 95 (Must Read) with full analysis and a referenced report;
// article-a scores 80 (Should Read) with minimal analysis.
func SampleDigest(id string, createdAt time.Time) models.Digest {
	count := 2
	return models.Digest{
		Id:            id,
		CreatedAt:     createdAt,
		ArticleCount:  &count,
		TimeWindow:    24 * time.Hour,
		DigestSummary: "## Summary\n\nA short digest.",
		ProviderResults: []models.DigestProviderResult{
			{ProviderType: "openai", ModelName: "gpt-test"},
		},
		Articles: []models.Article{
			{Id: "article-b", Title: "Article B", Link: "https://example.com/b", PublishedAt: createdAt},
			{Id: "article-a", Title: "Article A", Link: "https://example.com/a", PublishedAt: createdAt},
		},
		DigestAnalyses: []models.DigestAnalysis{
			{
				ArticleId: "article-b",
				Analysis: &models.ArticleAnalysis{
					ArticleId:              "article-b",
					ProviderType:           "openai",
					ModelName:              "gpt-test",
					ImportanceScore:        95,
					ScoreDimensions:        &scoring.Dimensions{Specificity: 4, Severity: 4, Breadth: 4, Novelty: 3, Actionability: 4, Credibility: 3},
					Tldr:                   "Article B tldr.",
					BriefOverview:          "Article B brief overview.",
					StandardSynthesis:      "Article B standard synthesis.",
					ComprehensiveSynthesis: "Article B comprehensive synthesis.",
					KeyPoints:              []string{"Article B key point"},
					Insights:               []string{"Article B insight"},
					ReferencedReports:      []models.ReferencedReport{{Title: "Article B report", URL: "https://example.com/report", Publisher: "Example Labs", Context: "Supporting source."}},
				},
			},
			{
				ArticleId: "article-a",
				Analysis: &models.ArticleAnalysis{
					ArticleId:       "article-a",
					ProviderType:    "openai",
					ModelName:       "gpt-test",
					ImportanceScore: 80,
				},
			},
		},
	}
}
