package mappers

import (
	"testing"

	"github.com/ma111e/downlink/pkg/models"
)

func TestRelatedArticleRoundTrip(t *testing.T) {
	in := &models.RelatedArticle{
		ArticleId:        "a1",
		RelatedArticleId: "a2",
		RelationType:     "duplicate",
		SimilarityScore:  0.87,
	}
	out := RelatedArticleToModel(RelatedArticleToProto(in))
	if out == nil {
		t.Fatal("RelatedArticleToModel returned nil")
	}
	if out.ArticleId != "a1" || out.RelatedArticleId != "a2" {
		t.Errorf("ids lost: %+v", out)
	}
	if out.RelationType != "duplicate" {
		t.Errorf("RelationType = %q, want duplicate", out.RelationType)
	}
	if out.SimilarityScore != 0.87 {
		t.Errorf("SimilarityScore = %v, want 0.87", out.SimilarityScore)
	}
}

func TestRelatedArticleToProtoNilIsNil(t *testing.T) {
	if RelatedArticleToProto(nil) != nil {
		t.Fatal("RelatedArticleToProto(nil) != nil")
	}
}

func TestRelatedArticleToModelNilIsNil(t *testing.T) {
	if RelatedArticleToModel(nil) != nil {
		t.Fatal("RelatedArticleToModel(nil) != nil")
	}
}

func TestAllRelatedArticlesRoundTrip(t *testing.T) {
	in := []models.RelatedArticle{
		{ArticleId: "a1", RelatedArticleId: "a2", RelationType: "dup", SimilarityScore: 0.9},
		{ArticleId: "a1", RelatedArticleId: "a3", RelationType: "related", SimilarityScore: 0.6},
	}
	out := AllRelatedArticlesToModels(AllRelatedArticlesToProto(in))
	if len(out) != 2 || out[0].RelatedArticleId != "a2" || out[1].RelatedArticleId != "a3" {
		t.Errorf("slice round-trip lost data: %+v", out)
	}
}
