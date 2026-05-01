package mappers

import (
	"downlink/pkg/models"
	"downlink/pkg/protos"

	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func DigestToProto(digest *models.Digest) *protos.Digest {
	if digest == nil {
		return nil
	}

	protoDigest := &protos.Digest{
		Id:         digest.Id,
		CreatedAt:  timestamppb.New(digest.CreatedAt),
		TimeWindow: durationpb.New(digest.TimeWindow),
	}

	// Handle pointer fields that might be nil
	if digest.ArticleCount != nil {
		protoDigest.ArticleCount = int32(*digest.ArticleCount)
	}

	// Convert provider results
	if len(digest.ProviderResults) > 0 {
		protoDigest.ProviderResults = make([]*protos.DigestProviderResult, 0, len(digest.ProviderResults))
		for _, result := range digest.ProviderResults {
			protoDigest.ProviderResults = append(protoDigest.ProviderResults, DigestProviderResultToProto(&result))
		}
	}

	// Convert articles if needed
	if len(digest.Articles) > 0 {
		protoDigest.Articles = AllArticlesToProto(digest.Articles)
	}

	// Convert title and digest summary
	protoDigest.Title = digest.Title
	protoDigest.DigestSummary = digest.DigestSummary

	// Convert digest analyses
	if len(digest.DigestAnalyses) > 0 {
		protoDigest.DigestAnalyses = make([]*protos.DigestAnalysis, 0, len(digest.DigestAnalyses))
		for _, da := range digest.DigestAnalyses {
			protoDA := &protos.DigestAnalysis{
				DigestId:            da.DigestId,
				AnalysisId:          da.AnalysisId,
				ArticleId:           da.ArticleId,
				DuplicateGroup:      da.DuplicateGroup,
				IsMostComprehensive: da.IsMostComprehensive,
			}
			if da.Analysis != nil {
				protoDA.Analysis = ArticleAnalysisToProto(da.Analysis)
			}
			protoDigest.DigestAnalyses = append(protoDigest.DigestAnalyses, protoDA)
		}
	}

	return protoDigest
}

func DigestToModel(digest *protos.Digest) *models.Digest {
	if digest == nil {
		return nil
	}

	articleCount := int(digest.ArticleCount)

	modelDigest := &models.Digest{
		Id:           digest.Id,
		CreatedAt:    digest.CreatedAt.AsTime(),
		ArticleCount: &articleCount,
		TimeWindow:   digest.TimeWindow.AsDuration(),
	}

	// Convert provider results
	if len(digest.ProviderResults) > 0 {
		modelDigest.ProviderResults = make([]models.DigestProviderResult, 0, len(digest.ProviderResults))
		for _, result := range digest.ProviderResults {
			if result == nil {
				continue
			}
			modelDigest.ProviderResults = append(modelDigest.ProviderResults, *DigestProviderResultToModel(result))
		}
	}

	// Convert articles if needed
	if len(digest.Articles) > 0 {
		modelDigest.Articles = AllArticlesToModels(digest.Articles)
	}

	// Convert title and digest summary
	modelDigest.Title = digest.Title
	modelDigest.DigestSummary = digest.DigestSummary

	// Convert digest analyses
	if len(digest.DigestAnalyses) > 0 {
		modelDigest.DigestAnalyses = make([]models.DigestAnalysis, 0, len(digest.DigestAnalyses))
		for _, da := range digest.DigestAnalyses {
			if da == nil {
				continue
			}
			modelDA := models.DigestAnalysis{
				DigestId:            da.DigestId,
				AnalysisId:          da.AnalysisId,
				ArticleId:           da.ArticleId,
				DuplicateGroup:      da.DuplicateGroup,
				IsMostComprehensive: da.IsMostComprehensive,
			}
			if da.Analysis != nil {
				a := ArticleAnalysisToModel(da.Analysis)
				modelDA.Analysis = a
			}
			modelDigest.DigestAnalyses = append(modelDigest.DigestAnalyses, modelDA)
		}
	}

	return modelDigest
}

func AllDigestsToProto(digests []models.Digest) []*protos.Digest {
	var protoDigests []*protos.Digest

	for _, digest := range digests {
		protoDigests = append(protoDigests, DigestToProto(&digest))
	}

	return protoDigests
}

func AllDigestsToModels(digests []*protos.Digest) []models.Digest {
	var modelDigests []models.Digest

	for _, digest := range digests {
		if digest == nil {
			continue
		}
		modelDigests = append(modelDigests, *DigestToModel(digest))
	}

	return modelDigests
}

func ProviderConfigToProto(config *models.ProviderConfig) *protos.ProviderConfig {
	if config == nil {
		return nil
	}

	protoConfig := &protos.ProviderConfig{
		Name:         config.Name,
		ProviderType: config.ProviderType,
		ModelName:    config.ModelName,
		Enabled:      config.Enabled,
		BaseUrl:      config.BaseURL,
		ApiKey:       config.APIKey,
	}

	// Handle pointer fields that might be nil
	if config.Temperature != nil {
		temp := float32(*config.Temperature)
		protoConfig.Temperature = &temp
	}

	if config.MaxRetries != nil {
		retries := int32(*config.MaxRetries)
		protoConfig.MaxRetries = &retries
	}

	if config.TimeoutMinutes != nil {
		timeout := int32(*config.TimeoutMinutes)
		protoConfig.TimeoutMinutes = &timeout
	}

	return protoConfig
}

func AllProviderConfigsToProtos(configs []models.ProviderConfig) []*protos.ProviderConfig {
	var protoConfigs []*protos.ProviderConfig

	for _, config := range configs {
		protoConfigs = append(protoConfigs, ProviderConfigToProto(&config))
	}

	return protoConfigs
}

func AllProviderConfigsToModels(configs []*protos.ProviderConfig) []models.ProviderConfig {
	var modelConfigs []models.ProviderConfig

	for _, config := range configs {
		if config == nil {
			continue
		}
		modelConfigs = append(modelConfigs, *ProviderConfigToModel(config))
	}

	return modelConfigs
}

func ProviderConfigToModel(config *protos.ProviderConfig) *models.ProviderConfig {
	if config == nil {
		return nil
	}

	modelConfig := &models.ProviderConfig{
		Name:         config.Name,
		ProviderType: config.ProviderType,
		ModelName:    config.ModelName,
		Enabled:      config.Enabled,
		BaseURL:      config.BaseUrl,
		APIKey:       config.ApiKey,
	}

	// Handle pointer fields that might be nil
	if config.Temperature != nil {
		temp := float64(*config.Temperature)
		modelConfig.Temperature = &temp
	}

	if config.MaxRetries != nil {
		retries := int(*config.MaxRetries)
		modelConfig.MaxRetries = &retries
	}

	if config.TimeoutMinutes != nil {
		timeout := int(*config.TimeoutMinutes)
		modelConfig.TimeoutMinutes = &timeout
	}

	return modelConfig
}

func AnalysisConfigToProto(config *models.AnalysisConfig) *protos.AnalysisConfig {
	if config == nil {
		return nil
	}

	return &protos.AnalysisConfig{
		Provider: config.Provider,
		Persona:      config.Persona,
	}
}

func AnalysisConfigToModel(config *protos.AnalysisConfig) *models.AnalysisConfig {
	if config == nil {
		return nil
	}

	return &models.AnalysisConfig{
		Provider: config.Provider,
		Persona:      config.Persona,
	}
}

func DigestArticleToProto(article *models.DigestArticle) *protos.DigestArticle {
	if article == nil {
		return nil
	}

	return &protos.DigestArticle{
		DigestId:  article.DigestId,
		ArticleId: article.ArticleId,
	}
}

func DigestArticleToModel(article *protos.DigestArticle) *models.DigestArticle {
	if article == nil {
		return nil
	}

	return &models.DigestArticle{
		DigestId:  article.DigestId,
		ArticleId: article.ArticleId,
	}
}

func AllDigestArticlesToProto(articles []models.DigestArticle) []*protos.DigestArticle {
	var protoArticles []*protos.DigestArticle

	for _, article := range articles {
		protoArticles = append(protoArticles, DigestArticleToProto(&article))
	}

	return protoArticles
}

func AllDigestArticlesToModels(articles []*protos.DigestArticle) []models.DigestArticle {
	var modelArticles []models.DigestArticle

	for _, article := range articles {
		if article == nil {
			continue
		}
		modelArticles = append(modelArticles, *DigestArticleToModel(article))
	}

	return modelArticles
}
