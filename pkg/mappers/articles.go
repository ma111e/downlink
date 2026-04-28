package mappers

import (
	"downlink/pkg/models"
	"downlink/pkg/protos"

	"google.golang.org/protobuf/types/known/timestamppb"
)

func AllArticlesToProto(articles []models.Article) []*protos.Article {
	var protoArticles []*protos.Article

	for _, article := range articles {
		protoArticles = append(protoArticles, ArticleToProto(&article))
	}

	return protoArticles
}

func AllArticlesToModels(articles []*protos.Article) []models.Article {
	var modelsArticles []models.Article

	for _, article := range articles {
		if article == nil {
			continue
		}
		modelsArticles = append(modelsArticles, *ArticleToModel(article))
	}

	return modelsArticles
}

func ArticleToProto(article *models.Article) *protos.Article {
	protoArticle := &protos.Article{
		Id:          article.Id,
		FeedId:      article.FeedId,
		Title:       article.Title,
		Content:     article.Content,
		Link:        article.Link,
		FetchedAt:   timestamppb.New(article.FetchedAt),
		PublishedAt: timestamppb.New(article.PublishedAt),
		HeroImage:   article.HeroImage,
	}

	// Handle pointer fields that might be nil
	if article.Read != nil {
		protoArticle.Read = article.Read
	}

	if article.Bookmarked != nil {
		protoArticle.Bookmarked = article.Bookmarked
	}

	if article.CategoryName != nil {
		protoArticle.CategoryName = article.CategoryName
	}

	if article.LatestImportanceScore != nil {
		protoArticle.LatestImportanceScore = article.LatestImportanceScore
	}

	// Add category if available
	if article.Category != nil {
		protoArticle.Category = &protos.Category{
			Name:  article.Category.Name,
			Color: article.Category.Color,
			Icon:  article.Category.Icon,
		}
	}

	protoArticle.Tags = AllTagsToProto(article.Tags)
	protoArticle.RelatedArticles = AllRelatedArticlesToProto(article.RelatedArticles)

	return protoArticle
}

func ArticleToModel(article *protos.Article) *models.Article {
	if article == nil {
		return nil
	}

	modelArticle := &models.Article{
		Id:          article.Id,
		FeedId:      article.FeedId,
		Title:       article.Title,
		Content:     article.Content,
		Link:        article.Link,
		FetchedAt:   article.FetchedAt.AsTime(),
		PublishedAt: article.PublishedAt.AsTime(),
		HeroImage:   article.HeroImage,
	}

	// Handle pointer fields that might be nil
	if article.Read != nil {
		modelArticle.Read = article.Read
	}

	if article.Bookmarked != nil {
		modelArticle.Bookmarked = article.Bookmarked
	}

	if article.CategoryName != nil {
		modelArticle.CategoryName = article.CategoryName
	}

	if article.LatestImportanceScore != nil {
		modelArticle.LatestImportanceScore = article.LatestImportanceScore
	}

	// Add category if available
	if article.Category != nil {
		modelArticle.Category = &models.Category{
			Name:  article.Category.Name,
			Color: article.Category.Color,
			Icon:  article.Category.Icon,
		}
	}

	modelArticle.Tags = AllTagsToModels(article.Tags)
	modelArticle.RelatedArticles = AllRelatedArticlesToModels(article.RelatedArticles)

	return modelArticle
}

func ArticleFilterToModel(protoFilter *protos.ArticleFilter) *models.ArticleFilter {
	filter := &models.ArticleFilter{
		UnreadOnly:      protoFilter.UnreadOnly,
		CategoryName:    protoFilter.CategoryName,
		TagId:           protoFilter.TagId,
		BookmarkedOnly:  protoFilter.BookmarkedOnly,
		RelatedToId:     protoFilter.RelatedToId,
		FeedId:          protoFilter.FeedId,
		Offset:          int(protoFilter.Offset),
		Limit:           int(protoFilter.Limit),
		Query:           protoFilter.Query,
		ExcludeDigested: protoFilter.ExcludeDigested,
		// These fields MUST BE INITIALIZED or they won't be set in the query (??)
		// StartDate: &time.Time{},
		// EndDate:   &time.Time{},
	}

	if protoFilter.StartDate != nil {
		startDate := protoFilter.StartDate.AsTime()
		filter.StartDate = &startDate
	}

	if protoFilter.EndDate != nil {
		endDate := protoFilter.EndDate.AsTime()
		filter.EndDate = &endDate
	}

	return filter
}

func ArticleFilterToProto(modelFilter *models.ArticleFilter) *protos.ArticleFilter {
	filter := protos.ArticleFilter{
		UnreadOnly:      modelFilter.UnreadOnly,
		CategoryName:    modelFilter.CategoryName,
		TagId:           modelFilter.TagId,
		BookmarkedOnly:  modelFilter.BookmarkedOnly,
		RelatedToId:     modelFilter.RelatedToId,
		FeedId:          modelFilter.FeedId,
		Offset:          uint32(modelFilter.Offset),
		Limit:           uint32(modelFilter.Limit),
		Query:           modelFilter.Query,
		ExcludeDigested: modelFilter.ExcludeDigested,
		// These fields MUST BE INITIALIZED or they won't be set in the query (??)
		// StartDate: &timestamppb.Timestamp{},
		// EndDate:   &timestamppb.Timestamp{},
	}

	if modelFilter.StartDate != nil {
		filter.StartDate = timestamppb.New(*modelFilter.StartDate)
	}

	if modelFilter.EndDate != nil {
		filter.EndDate = timestamppb.New(*modelFilter.EndDate)
	}

	return &filter
}

func ArticleUpdateToModels(protoUpdate *protos.ArticleUpdate) *models.ArticleUpdate {
	filter := models.ArticleUpdate{
		Read:         protoUpdate.Read,
		CategoryName: protoUpdate.CategoryName,
		HeroImage:    protoUpdate.HeroImage,
		Bookmarked:   protoUpdate.Bookmarked,
	}

	if protoUpdate.TagIds != nil {
		filter.TagIds = &protoUpdate.TagIds
	}

	if protoUpdate.RelatedArticles != nil {
		relatedArticles := AllRelatedArticlesToModels(protoUpdate.RelatedArticles)
		filter.RelatedArticles = &relatedArticles
	}

	return &filter
}

func ArticleUpdateToProto(modelUpdate *models.ArticleUpdate) *protos.ArticleUpdate {
	filter := protos.ArticleUpdate{
		Read:         modelUpdate.Read,
		CategoryName: modelUpdate.CategoryName,
		HeroImage:    modelUpdate.HeroImage,
		Bookmarked:   modelUpdate.Bookmarked,
	}

	if modelUpdate.TagIds != nil {
		filter.TagIds = *modelUpdate.TagIds
	}

	if modelUpdate.RelatedArticles != nil {
		filter.RelatedArticles = AllRelatedArticlesToProto(*modelUpdate.RelatedArticles)
	}

	return &filter

}
