package mappers

import (
	"downlink/pkg/models"
	"downlink/pkg/protos"
)

func RelatedArticleToProto(relatedArticle *models.RelatedArticle) *protos.RelatedArticle {
	if relatedArticle == nil {
		return nil
	}

	protoArticle := protos.RelatedArticle{
		ArticleId:        relatedArticle.ArticleId,
		RelatedArticleId: relatedArticle.RelatedArticleId,
		RelationType:     relatedArticle.RelationType,
		SimilarityScore:  relatedArticle.SimilarityScore,
	}

	return &protoArticle
}

func RelatedArticleToModel(relatedArticle *protos.RelatedArticle) *models.RelatedArticle {
	if relatedArticle == nil {
		return nil
	}

	modelArticle := models.RelatedArticle{
		ArticleId:        relatedArticle.ArticleId,
		RelatedArticleId: relatedArticle.RelatedArticleId,
		RelationType:     relatedArticle.RelationType,
		SimilarityScore:  relatedArticle.SimilarityScore,
	}

	return &modelArticle
}

func AllRelatedArticlesToProto(relatedArticles []models.RelatedArticle) []*protos.RelatedArticle {
	return mapValueSlice(relatedArticles, RelatedArticleToProto)
}

func AllRelatedArticlesToModels(relatedArticles []*protos.RelatedArticle) []models.RelatedArticle {
	return mapPointerSlice(relatedArticles, RelatedArticleToModel)
}
