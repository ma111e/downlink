package mappers

import (
	"downlink/pkg/models"
	"downlink/pkg/protos"

	"google.golang.org/protobuf/types/known/timestamppb"
)

func ReferencedReportToProto(report models.ReferencedReport) *protos.ReferencedReport {
	return &protos.ReferencedReport{
		Title:     report.Title,
		Url:       report.URL,
		Publisher: report.Publisher,
		Context:   report.Context,
	}
}

func ReferencedReportToModel(report *protos.ReferencedReport) models.ReferencedReport {
	if report == nil {
		return models.ReferencedReport{}
	}
	return models.ReferencedReport{
		Title:     report.Title,
		URL:       report.Url,
		Publisher: report.Publisher,
		Context:   report.Context,
	}
}

func AllReferencedReportsToProto(reports []models.ReferencedReport) []*protos.ReferencedReport {
	protoReports := make([]*protos.ReferencedReport, 0, len(reports))
	for _, report := range reports {
		protoReports = append(protoReports, ReferencedReportToProto(report))
	}
	return protoReports
}

func AllReferencedReportsToModels(reports []*protos.ReferencedReport) []models.ReferencedReport {
	modelReports := make([]models.ReferencedReport, 0, len(reports))
	for _, report := range reports {
		if report == nil {
			continue
		}
		modelReports = append(modelReports, ReferencedReportToModel(report))
	}
	return modelReports
}

func ArticleAnalysisToProto(analysis *models.ArticleAnalysis) *protos.ArticleAnalysis {
	if analysis == nil {
		return nil
	}

	protoAnalysis := &protos.ArticleAnalysis{
		Id:                     analysis.Id,
		ArticleId:              analysis.ArticleId,
		ProviderType:           analysis.ProviderType,
		ModelName:              analysis.ModelName,
		ImportanceScore:        int32(analysis.ImportanceScore),
		KeyPoints:              analysis.KeyPoints,
		Insights:               analysis.Insights,
		Tldr:                   analysis.Tldr,
		Justification:          analysis.Justification,
		BriefOverview:          analysis.BriefOverview,
		StandardSynthesis:      analysis.StandardSynthesis,
		ComprehensiveSynthesis: analysis.ComprehensiveSynthesis,
		ThinkingProcess:        analysis.ThinkingProcess,
		RawResponse:            analysis.RawResponse,
		ReferencedReports:      AllReferencedReportsToProto(analysis.ReferencedReports),
		CreatedAt:              timestamppb.New(analysis.CreatedAt),
		KeyPointsJson:          analysis.KeyPointsJson,
		InsightsJson:           analysis.InsightsJson,
		ReferencedReportsJson:  analysis.ReferencedReportsJson,
	}

	return protoAnalysis
}

func ArticleAnalysisToModel(analysis *protos.ArticleAnalysis) *models.ArticleAnalysis {
	if analysis == nil {
		return nil
	}

	modelAnalysis := &models.ArticleAnalysis{
		Id:                     analysis.Id,
		ArticleId:              analysis.ArticleId,
		ProviderType:           analysis.ProviderType,
		ModelName:              analysis.ModelName,
		ImportanceScore:        int(analysis.ImportanceScore),
		KeyPointsJson:          analysis.KeyPointsJson,
		InsightsJson:           analysis.InsightsJson,
		ReferencedReportsJson:  analysis.ReferencedReportsJson,
		KeyPoints:              analysis.KeyPoints,
		Insights:               analysis.Insights,
		ReferencedReports:      AllReferencedReportsToModels(analysis.ReferencedReports),
		Tldr:                   analysis.Tldr,
		Justification:          analysis.Justification,
		BriefOverview:          analysis.BriefOverview,
		StandardSynthesis:      analysis.StandardSynthesis,
		ComprehensiveSynthesis: analysis.ComprehensiveSynthesis,
		ThinkingProcess:        analysis.ThinkingProcess,
		RawResponse:            analysis.RawResponse,
		CreatedAt:              analysis.CreatedAt.AsTime(),
	}

	return modelAnalysis
}

func AllArticleAnalysesToProto(analyses []models.ArticleAnalysis) []*protos.ArticleAnalysis {
	var protoAnalyses []*protos.ArticleAnalysis

	for _, analysis := range analyses {
		protoAnalyses = append(protoAnalyses, ArticleAnalysisToProto(&analysis))
	}

	return protoAnalyses
}

func AllArticleAnalysesToModels(analyses []*protos.ArticleAnalysis) []models.ArticleAnalysis {
	var modelAnalyses []models.ArticleAnalysis

	for _, analysis := range analyses {
		if analysis == nil {
			continue
		}
		modelAnalyses = append(modelAnalyses, *ArticleAnalysisToModel(analysis))
	}

	return modelAnalyses
}
