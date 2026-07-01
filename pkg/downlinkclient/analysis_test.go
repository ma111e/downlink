package downlinkclient

import (
	"context"
	"testing"
	"time"

	"github.com/ma111e/downlink/pkg/protos"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// stubAnalysis is a minimal AnalysisServiceServer that returns canned responses.
type stubAnalysis struct {
	protos.UnimplementedAnalysisServiceServer

	allAnalyses []*protos.ArticleAnalysis
	oneAnalysis *protos.ArticleAnalysis
	entries     []*protos.GlossaryEntry
}

func (s *stubAnalysis) GetAllArticleAnalyses(_ context.Context, _ *protos.GetAllArticleAnalysesRequest) (*protos.GetAllArticleAnalysesResponse, error) {
	return &protos.GetAllArticleAnalysesResponse{Analyses: s.allAnalyses}, nil
}

func (s *stubAnalysis) GetAnalysis(_ context.Context, _ *protos.GetAnalysisRequest) (*protos.ArticleAnalysis, error) {
	return s.oneAnalysis, nil
}

func (s *stubAnalysis) ListGlossaryEntries(_ context.Context, _ *protos.ListGlossaryEntriesRequest) (*protos.ListGlossaryEntriesResponse, error) {
	return &protos.ListGlossaryEntriesResponse{Entries: s.entries}, nil
}

func newAnalysisClient(t *testing.T, stub *stubAnalysis) *DownlinkClient {
	t.Helper()
	return dialStub(t, func(s *grpc.Server) {
		protos.RegisterAnalysisServiceServer(s, stub)
	})
}

func TestGetAllArticleAnalysesClientEmpty(t *testing.T) {
	stub := &stubAnalysis{}
	client := newAnalysisClient(t, stub)
	got, err := client.GetAllArticleAnalyses("art1")
	if err != nil {
		t.Fatalf("GetAllArticleAnalyses error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}

func TestGetAllArticleAnalysesClientMapsResponse(t *testing.T) {
	created := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	stub := &stubAnalysis{
		allAnalyses: []*protos.ArticleAnalysis{
			{Id: "an1", ArticleId: "art1", ImportanceScore: 80, CreatedAt: timestamppb.New(created)},
			{Id: "an2", ArticleId: "art1", ImportanceScore: 60, CreatedAt: timestamppb.New(created)},
		},
	}
	client := newAnalysisClient(t, stub)
	got, err := client.GetAllArticleAnalyses("art1")
	if err != nil {
		t.Fatalf("GetAllArticleAnalyses error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Id != "an1" || got[0].ImportanceScore != 80 {
		t.Errorf("first analysis: id=%q score=%d", got[0].Id, got[0].ImportanceScore)
	}
}

func TestGetAnalysisClientMapsResponse(t *testing.T) {
	created := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	stub := &stubAnalysis{
		oneAnalysis: &protos.ArticleAnalysis{
			Id:        "an1",
			ArticleId: "art1",
			Tldr:      "summary",
			CreatedAt: timestamppb.New(created),
		},
	}
	client := newAnalysisClient(t, stub)
	got, err := client.GetAnalysis("art1")
	if err != nil {
		t.Fatalf("GetAnalysis error = %v", err)
	}
	if got.Id != "an1" || got.Tldr != "summary" {
		t.Errorf("got id=%q tldr=%q", got.Id, got.Tldr)
	}
}

func TestListGlossaryEntriesClientMapsResponse(t *testing.T) {
	stub := &stubAnalysis{
		entries: []*protos.GlossaryEntry{
			{Id: "e1", Term: "APT", NormalizedKey: "apt", Definition: "threat actor group"},
		},
	}
	client := newAnalysisClient(t, stub)
	got, err := client.ListGlossaryEntries(10)
	if err != nil {
		t.Fatalf("ListGlossaryEntries error = %v", err)
	}
	if len(got) != 1 || got[0].Term != "APT" || got[0].Definition != "threat actor group" {
		t.Errorf("got %+v", got)
	}
}
