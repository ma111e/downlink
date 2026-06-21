package services

import (
	"context"
	"fmt"
	"github.com/ma111e/downlink/cmd/server/internal/store"
	"github.com/ma111e/downlink/pkg/mappers"
	"github.com/ma111e/downlink/pkg/models"
	"github.com/ma111e/downlink/pkg/protos"
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
	analyses, err := store.Db.GetAllArticleAnalyses(req.ArticleId, "")
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve article analyses: %w", err)
	}

	return &protos.GetAllArticleAnalysesResponse{
		Analyses: mappers.AllArticleAnalysesToProto(analyses),
	}, nil
}

// ListGlossaryEntries lists entries from the persistent global glossary.
func (s *AnalysisServer) ListGlossaryEntries(ctx context.Context, req *protos.ListGlossaryEntriesRequest) (*protos.ListGlossaryEntriesResponse, error) {
	entries, err := store.Db.ListGlossaryEntries(int(req.Limit))
	if err != nil {
		return nil, fmt.Errorf("failed to list glossary entries: %w", err)
	}
	return &protos.ListGlossaryEntriesResponse{
		Entries: mappers.AllGlossaryEntriesToProto(entries),
	}, nil
}

// SetGlossaryOverride sets a curated definition for a term and returns the updated entry.
func (s *AnalysisServer) SetGlossaryOverride(ctx context.Context, req *protos.SetGlossaryOverrideRequest) (*protos.SetGlossaryOverrideResponse, error) {
	if err := store.Db.SetGlossaryManualOverride(req.Term, req.Definition); err != nil {
		return nil, fmt.Errorf("failed to set glossary override: %w", err)
	}
	key := models.NormalizeGlossaryKey(req.Term)
	found, err := store.Db.GetGlossaryEntriesByKeys([]string{key})
	if err != nil {
		return nil, fmt.Errorf("failed to load updated glossary entry: %w", err)
	}
	return &protos.SetGlossaryOverrideResponse{
		Entry: mappers.GlossaryEntryToProto(found[key]),
	}, nil
}
