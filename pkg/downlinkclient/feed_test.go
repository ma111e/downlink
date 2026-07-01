package downlinkclient

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/ma111e/downlink/pkg/models"
	"github.com/ma111e/downlink/pkg/protos"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/emptypb"
)

// stubFeeds is a minimal FeedsServiceServer that records the last request it saw
// and returns canned responses/errors, so client wrappers can be tested for
// request mapping and response translation without a real server.
type stubFeeds struct {
	protos.UnimplementedFeedsServiceServer

	lastRegister *protos.RegisterFeedRequest
	lastDelete   *protos.DeleteFeedRequest
	lastRefresh  *protos.RefreshFeedRequest

	listResp    *protos.ListFeedsResponse
	listErr     error
	refreshResp *protos.RefreshFeedResponse
	streamEvents []*protos.RefreshAllFeedsEvent
}

func (s *stubFeeds) ListFeeds(context.Context, *protos.ListFeedsRequest) (*protos.ListFeedsResponse, error) {
	return s.listResp, s.listErr
}

func (s *stubFeeds) RegisterFeed(_ context.Context, req *protos.RegisterFeedRequest) (*emptypb.Empty, error) {
	s.lastRegister = req
	return &emptypb.Empty{}, nil
}

func (s *stubFeeds) DeleteFeed(_ context.Context, req *protos.DeleteFeedRequest) (*emptypb.Empty, error) {
	s.lastDelete = req
	return &emptypb.Empty{}, nil
}

func (s *stubFeeds) RefreshFeed(_ context.Context, req *protos.RefreshFeedRequest) (*protos.RefreshFeedResponse, error) {
	s.lastRefresh = req
	return s.refreshResp, nil
}

func (s *stubFeeds) RefreshAllFeeds(_ *protos.RefreshAllFeedsRequest, stream grpc.ServerStreamingServer[protos.RefreshAllFeedsEvent]) error {
	for _, ev := range s.streamEvents {
		if err := stream.Send(ev); err != nil {
			return err
		}
	}
	return nil
}

// newTestClient stands up an in-memory gRPC server with stub and returns a
// DownlinkClient wired to it over bufconn.
func newTestClient(t *testing.T, stub protos.FeedsServiceServer) *DownlinkClient {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	srv := grpc.NewServer()
	protos.RegisterFeedsServiceServer(srv, stub)
	go func() { _ = srv.Serve(lis) }()

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("grpc.NewClient() error = %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
		srv.Stop()
	})
	return NewDownlinkClient(conn)
}

func TestListFeedsMapsResponseToModels(t *testing.T) {
	stub := &stubFeeds{listResp: &protos.ListFeedsResponse{
		Feeds: []*protos.Feed{
			{Id: "f1", Url: "https://a/rss", Title: "A"},
			{Id: "f2", Url: "https://b/rss", Title: "B"},
		},
	}}
	pc := newTestClient(t, stub)

	feeds, err := pc.ListFeeds()
	if err != nil {
		t.Fatalf("ListFeeds() error = %v", err)
	}
	if len(feeds) != 2 || feeds[0].Id != "f1" || feeds[0].URL != "https://a/rss" || feeds[1].Title != "B" {
		t.Fatalf("feeds = %+v, want two feeds mapped to models", feeds)
	}
}

func TestListFeedsPropagatesError(t *testing.T) {
	pc := newTestClient(t, &stubFeeds{listErr: errors.New("boom")})
	_, err := pc.ListFeeds()
	if err == nil {
		t.Fatal("ListFeeds() error = nil, want the server error propagated")
	}
}

func TestRegisterFeedSendsMappedConfig(t *testing.T) {
	stub := &stubFeeds{}
	pc := newTestClient(t, stub)

	err := pc.RegisterFeed(models.FeedConfig{URL: "https://x/rss", Title: "X", Enabled: true})
	if err != nil {
		t.Fatalf("RegisterFeed() error = %v", err)
	}
	if stub.lastRegister == nil || stub.lastRegister.FeedConfig == nil {
		t.Fatal("server did not receive a RegisterFeed request with a FeedConfig")
	}
	if stub.lastRegister.FeedConfig.Url != "https://x/rss" || stub.lastRegister.FeedConfig.Title != "X" {
		t.Fatalf("received config = %+v, want url/title mapped", stub.lastRegister.FeedConfig)
	}
}

func TestDeleteFeedSendsFeedId(t *testing.T) {
	stub := &stubFeeds{}
	pc := newTestClient(t, stub)

	if err := pc.DeleteFeed("feed-42"); err != nil {
		t.Fatalf("DeleteFeed() error = %v", err)
	}
	if stub.lastDelete == nil || stub.lastDelete.FeedId != "feed-42" {
		t.Fatalf("received delete req = %+v, want FeedId feed-42", stub.lastDelete)
	}
}

func TestDiagnoseFeedSetsDiagnoseFlagAndReturnsDiagnosis(t *testing.T) {
	stub := &stubFeeds{refreshResp: &protos.RefreshFeedResponse{
		Diagnosis: &protos.FeedDiagnosis{Url: "https://x/rss", HttpStatus: 200, Verdict: "ok"},
	}}
	pc := newTestClient(t, stub)

	diag, err := pc.DiagnoseFeed("feed-7")
	if err != nil {
		t.Fatalf("DiagnoseFeed() error = %v", err)
	}
	// The wrapper must set diagnose=true and target the right feed.
	if stub.lastRefresh == nil || !stub.lastRefresh.Diagnose || stub.lastRefresh.FeedId != "feed-7" {
		t.Fatalf("refresh req = %+v, want Diagnose=true FeedId=feed-7", stub.lastRefresh)
	}
	if diag == nil || diag.Verdict != "ok" || diag.HttpStatus != 200 {
		t.Fatalf("diagnosis = %+v, want the server's diagnosis returned", diag)
	}
}

func TestRefreshAllFeedsStreamsEventsToCallback(t *testing.T) {
	stub := &stubFeeds{streamEvents: []*protos.RefreshAllFeedsEvent{
		{Completed: 1, Total: 2},
		{Completed: 2, Total: 2},
	}}
	pc := newTestClient(t, stub)

	var got []int32
	err := pc.RefreshAllFeeds(func(ev *protos.RefreshAllFeedsEvent) {
		got = append(got, ev.Completed)
	})
	if err != nil {
		t.Fatalf("RefreshAllFeeds() error = %v", err)
	}
	if len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("callback saw %v, want [1 2] (one call per streamed event, EOF terminates)", got)
	}
}
