package downlinkclient

import (
	"downlink/pkg/mappers"
	"downlink/pkg/models"
	"downlink/pkg/protos"
)

// GetAllArticleAnalyses retrieves all analysis results for a specific article
func (pc *DownlinkClient) GetAllArticleAnalyses(articleId string) ([]models.ArticleAnalysis, error) {
	protoReq := &protos.GetAllArticleAnalysesRequest{
		ArticleId: articleId,
	}

	protoRes, err := pc.analysisClient.GetAllArticleAnalyses(pc.ctx, protoReq)
	if err != nil {
		return nil, err
	}

	return mappers.AllArticleAnalysesToModels(protoRes.Analyses), nil
}

// GetAnalysis retrieves the analysis results by its ID
func (pc *DownlinkClient) GetAnalysis(articleId string) (models.ArticleAnalysis, error) {
	protoReq := &protos.GetAnalysisRequest{
		ArticleId: articleId,
	}

	protoRes, err := pc.analysisClient.GetAnalysis(pc.ctx, protoReq)
	if err != nil {
		return models.ArticleAnalysis{}, err
	}

	if a := mappers.ArticleAnalysisToModel(protoRes); a != nil {
		return *a, nil
	}
	return models.ArticleAnalysis{}, nil
}

// UpdateAnalysisConfig updates the enrichment configuration
func (pc *DownlinkClient) UpdateAnalysisConfig(analysisConfig models.AnalysisConfig) error {
	_, err := pc.serverConfigClient.UpdateAnalysisConfig(pc.ctx, &protos.UpdateAnalysisConfigRequest{
		AnalysisConfig: mappers.AnalysisConfigToProto(&analysisConfig),
	})

	return err
}
