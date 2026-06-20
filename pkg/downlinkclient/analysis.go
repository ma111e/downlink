package downlinkclient

import (
	"github.com/ma111e/downlink/pkg/mappers"
	"github.com/ma111e/downlink/pkg/models"
	"github.com/ma111e/downlink/pkg/protos"
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

// ListGlossaryEntries lists entries from the persistent global glossary (limit <= 0 = all).
func (pc *DownlinkClient) ListGlossaryEntries(limit int) ([]models.GlossaryEntry, error) {
	protoRes, err := pc.analysisClient.ListGlossaryEntries(pc.ctx, &protos.ListGlossaryEntriesRequest{
		Limit: int32(limit),
	})
	if err != nil {
		return nil, err
	}
	return mappers.AllGlossaryEntriesToModels(protoRes.Entries), nil
}

// SetGlossaryOverride sets a curated definition for a term and returns the updated entry.
func (pc *DownlinkClient) SetGlossaryOverride(term, definition string) (models.GlossaryEntry, error) {
	protoRes, err := pc.analysisClient.SetGlossaryOverride(pc.ctx, &protos.SetGlossaryOverrideRequest{
		Term:       term,
		Definition: definition,
	})
	if err != nil {
		return models.GlossaryEntry{}, err
	}
	if e := mappers.GlossaryEntryToModel(protoRes.Entry); e != nil {
		return *e, nil
	}
	return models.GlossaryEntry{}, nil
}

// UpdateAnalysisConfig updates the enrichment configuration
func (pc *DownlinkClient) UpdateAnalysisConfig(analysisConfig models.AnalysisConfig) error {
	_, err := pc.serverConfigClient.UpdateAnalysisConfig(pc.ctx, &protos.UpdateAnalysisConfigRequest{
		AnalysisConfig: mappers.AnalysisConfigToProto(&analysisConfig),
	})

	return err
}
