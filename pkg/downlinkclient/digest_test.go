package downlinkclient

import (
	"context"
	"testing"

	"github.com/ma111e/downlink/pkg/protos"

	"google.golang.org/grpc"
)

type stubDigests struct {
	protos.UnimplementedDigestServiceServer

	lastGet  *protos.GetDigestRequest
	lastList *protos.ListDigestsRequest

	getResp      *protos.GetDigestResponse
	listResp     *protos.ListDigestsResponse
	articlesResp *protos.GetDigestArticlesResponse
}

func (s *stubDigests) GetDigest(_ context.Context, r *protos.GetDigestRequest) (*protos.GetDigestResponse, error) {
	s.lastGet = r
	return s.getResp, nil
}
func (s *stubDigests) ListDigests(_ context.Context, r *protos.ListDigestsRequest) (*protos.ListDigestsResponse, error) {
	s.lastList = r
	return s.listResp, nil
}
func (s *stubDigests) GetDigestArticles(_ context.Context, _ *protos.GetDigestArticlesRequest) (*protos.GetDigestArticlesResponse, error) {
	return s.articlesResp, nil
}

func digestClient(t *testing.T, stub protos.DigestServiceServer) *DownlinkClient {
	return dialStub(t, func(s *grpc.Server) { protos.RegisterDigestServiceServer(s, stub) })
}

func TestGetDigestReturnsModel(t *testing.T) {
	stub := &stubDigests{getResp: &protos.GetDigestResponse{
		Digest: &protos.Digest{Id: "d1", Title: "Weekly"},
	}}
	pc := digestClient(t, stub)

	got, err := pc.GetDigest("d1")
	if err != nil {
		t.Fatalf("GetDigest() error = %v", err)
	}
	if stub.lastGet == nil || stub.lastGet.Id != "d1" {
		t.Fatalf("received req = %+v, want id d1", stub.lastGet)
	}
	if got.Id != "d1" || got.Title != "Weekly" {
		t.Fatalf("digest = %+v, want mapped d1/Weekly", got)
	}
}

func TestListDigestsSetsSummaryMode(t *testing.T) {
	stub := &stubDigests{listResp: &protos.ListDigestsResponse{
		Digests: []*protos.Digest{{Id: "d1"}, {Id: "d2"}},
	}}
	pc := digestClient(t, stub)

	got, err := pc.ListDigests(10)
	if err != nil {
		t.Fatalf("ListDigests() error = %v", err)
	}
	// Summary variant: Full=false, Limit passed through.
	if stub.lastList == nil || stub.lastList.Full || stub.lastList.Limit != 10 {
		t.Fatalf("received req = %+v, want Full=false Limit=10", stub.lastList)
	}
	if len(got) != 2 {
		t.Fatalf("got %d digests, want 2", len(got))
	}
}

func TestListDigestsFullSetsFullMode(t *testing.T) {
	stub := &stubDigests{listResp: &protos.ListDigestsResponse{}}
	pc := digestClient(t, stub)

	if _, err := pc.ListDigestsFull(5); err != nil {
		t.Fatalf("ListDigestsFull() error = %v", err)
	}
	if stub.lastList == nil || !stub.lastList.Full || stub.lastList.Limit != 5 {
		t.Fatalf("received req = %+v, want Full=true Limit=5", stub.lastList)
	}
}
