package downlinkclient

import (
	"context"
	"testing"

	"github.com/ma111e/downlink/pkg/models"
	"github.com/ma111e/downlink/pkg/protos"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

type stubArticles struct {
	protos.UnimplementedArticleServiceServer

	lastFilter   *protos.ArticleFilter
	lastGet      *protos.GetArticleRequest
	lastMarkFeed *protos.MarkFeedReadRequest

	listResp   *protos.ListArticlesResponse
	getResp    *protos.Article
	countsResp *protos.ArticleCountsResponse
}

func (s *stubArticles) ListArticles(_ context.Context, f *protos.ArticleFilter) (*protos.ListArticlesResponse, error) {
	s.lastFilter = f
	return s.listResp, nil
}
func (s *stubArticles) GetArticle(_ context.Context, r *protos.GetArticleRequest) (*protos.Article, error) {
	s.lastGet = r
	return s.getResp, nil
}
func (s *stubArticles) GetArticleCounts(_ context.Context, f *protos.ArticleFilter) (*protos.ArticleCountsResponse, error) {
	s.lastFilter = f
	return s.countsResp, nil
}
func (s *stubArticles) MarkFeedArticlesRead(_ context.Context, r *protos.MarkFeedReadRequest) (*emptypb.Empty, error) {
	s.lastMarkFeed = r
	return &emptypb.Empty{}, nil
}

func articleClient(t *testing.T, stub protos.ArticleServiceServer) *DownlinkClient {
	return dialStub(t, func(s *grpc.Server) { protos.RegisterArticleServiceServer(s, stub) })
}

func TestListArticlesMapsFilterAndResponse(t *testing.T) {
	stub := &stubArticles{listResp: &protos.ListArticlesResponse{
		Articles: []*protos.Article{{Id: "a1", Title: "One"}, {Id: "a2", Title: "Two"}},
	}}
	pc := articleClient(t, stub)

	arts, err := pc.ListArticles(models.ArticleFilter{UnreadOnly: true, FeedId: "f1", Limit: 25})
	if err != nil {
		t.Fatalf("ListArticles() error = %v", err)
	}
	// Filter is mapped into the request.
	if stub.lastFilter == nil || !stub.lastFilter.UnreadOnly || stub.lastFilter.FeedId != "f1" || stub.lastFilter.Limit != 25 {
		t.Fatalf("received filter = %+v, want unread/f1/limit25", stub.lastFilter)
	}
	// Response is mapped back to models.
	if len(arts) != 2 || arts[0].Id != "a1" || arts[1].Title != "Two" {
		t.Fatalf("articles = %+v, want two mapped models", arts)
	}
}

func TestGetArticleReturnsModel(t *testing.T) {
	stub := &stubArticles{getResp: &protos.Article{Id: "a1", Title: "Hello", Link: "https://x/1"}}
	pc := articleClient(t, stub)

	got, err := pc.GetArticle("a1")
	if err != nil {
		t.Fatalf("GetArticle() error = %v", err)
	}
	if stub.lastGet == nil || stub.lastGet.Id != "a1" {
		t.Fatalf("received get req = %+v, want id a1", stub.lastGet)
	}
	if got.Id != "a1" || got.Title != "Hello" || got.Link != "https://x/1" {
		t.Fatalf("article = %+v, want mapped a1/Hello", got)
	}
}

func TestGetArticleCountsMapsCounts(t *testing.T) {
	stub := &stubArticles{countsResp: &protos.ArticleCountsResponse{
		AllUnreadCount:  12,
		BookmarkedCount: 3,
		UnreadByFeed:    map[string]int32{"f1": 5, "f2": 7},
	}}
	pc := articleClient(t, stub)

	counts, err := pc.GetArticleCounts(models.ArticleFilter{})
	if err != nil {
		t.Fatalf("GetArticleCounts() error = %v", err)
	}
	if counts.AllUnreadCount != 12 || counts.BookmarkedCount != 3 {
		t.Errorf("counts = %+v, want 12/3", counts)
	}
	if counts.UnreadByFeed["f1"] != 5 || counts.UnreadByFeed["f2"] != 7 {
		t.Errorf("UnreadByFeed = %v, want f1=5 f2=7", counts.UnreadByFeed)
	}
}

func TestMarkFeedArticlesReadSendsFeedId(t *testing.T) {
	stub := &stubArticles{}
	pc := articleClient(t, stub)

	if err := pc.MarkFeedArticlesRead("feed-9"); err != nil {
		t.Fatalf("MarkFeedArticlesRead() error = %v", err)
	}
	if stub.lastMarkFeed == nil || stub.lastMarkFeed.FeedId != "feed-9" {
		t.Fatalf("received req = %+v, want FeedId feed-9", stub.lastMarkFeed)
	}
}
