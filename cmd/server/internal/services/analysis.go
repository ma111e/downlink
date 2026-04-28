package services

import (
	"context"
	"fmt"
	"downlink/cmd/server/internal/store"
	"downlink/pkg/mappers"
	"downlink/pkg/protos"
)

// AnalysisServer implements the AnalysisService gRPC service
type AnalysisServer struct {
	protos.UnimplementedAnalysisServiceServer
}

// NewAnalysisServer creates a new Analysis server instance
func NewAnalysisServer() *AnalysisServer {
	return &AnalysisServer{}
}

// GetAllArticleAnalyses retrieves all analysis results for a specific article
func (s *AnalysisServer) GetAllArticleAnalyses(ctx context.Context, req *protos.GetAllArticleAnalysesRequest) (*protos.GetAllArticleAnalysesResponse, error) {
	analyses, err := store.Db.GetAllArticleAnalyses(req.ArticleId)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve article analyses: %w", err)
	}

	return &protos.GetAllArticleAnalysesResponse{
		Analyses: mappers.AllArticleAnalysesToProto(analyses),
	}, nil
}
