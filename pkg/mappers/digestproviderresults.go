package mappers

import (
	"downlink/pkg/models"
	"downlink/pkg/protos"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func DigestProviderResultToProto(result *models.DigestProviderResult) *protos.DigestProviderResult {
	if result == nil {
		return nil
	}

	protoResult := &protos.DigestProviderResult{
		Id:                     result.Id,
		DigestId:               result.DigestId,
		ProviderType:           result.ProviderType,
		ModelName:              result.ModelName,
		BriefOverview:          result.BriefOverview,
		StandardSynthesis:      result.StandardSynthesis,
		ComprehensiveSynthesis: result.ComprehensiveSynthesis,
		ProcessingTime:         result.ProcessingTime,
		Error:                  result.Error,
		CreatedAt:              timestamppb.New(result.CreatedAt),
	}

	// Add digest if available
	if result.Digest != nil {
		protoResult.Digest = DigestToProto(result.Digest)
	}

	return protoResult
}

func DigestProviderResultToModel(result *protos.DigestProviderResult) *models.DigestProviderResult {
	if result == nil {
		return nil
	}

	modelResult := &models.DigestProviderResult{
		Id:                     result.Id,
		DigestId:               result.DigestId,
		ProviderType:           result.ProviderType,
		ModelName:              result.ModelName,
		BriefOverview:          result.BriefOverview,
		StandardSynthesis:      result.StandardSynthesis,
		ComprehensiveSynthesis: result.ComprehensiveSynthesis,
		ProcessingTime:         result.ProcessingTime,
		Error:                  result.Error,
		CreatedAt:              result.CreatedAt.AsTime(),
	}

	// Add digest if available
	if result.Digest != nil {
		modelResult.Digest = DigestToModel(result.Digest)
	}

	return modelResult
}

func AllDigestProviderResultsToProto(results []models.DigestProviderResult) []*protos.DigestProviderResult {
	var protoResults []*protos.DigestProviderResult

	for _, result := range results {
		protoResults = append(protoResults, DigestProviderResultToProto(&result))
	}

	return protoResults
}

func AllDigestProviderResultsToModels(results []*protos.DigestProviderResult) []models.DigestProviderResult {
	var modelResults []models.DigestProviderResult

	for _, result := range results {
		modelResults = append(modelResults, *DigestProviderResultToModel(result))
	}

	return modelResults
}
