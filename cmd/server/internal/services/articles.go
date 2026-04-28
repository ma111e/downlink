package services

import (
	"context"
	"downlink/cmd/server/internal/store"
	"downlink/pkg/mappers"
	"downlink/pkg/protos"

	log "github.com/sirupsen/logrus"
	"google.golang.org/protobuf/types/known/emptypb"
)

// ArticleServer implements the ArticleService gRPC service
type ArticleServer struct {
	protos.UnimplementedArticleServiceServer
}

// NewArticleServer creates a new article server instance
func NewArticleServer() *ArticleServer {
	return &ArticleServer{}
}

// ListArticles implements the ListArticles RPC method
func (s *ArticleServer) ListArticles(_ context.Context, filter *protos.ArticleFilter) (*protos.ListArticlesResponse, error) {
	log.WithFields(log.Fields{
		"unread_only":     filter.UnreadOnly,
		"category_name":   filter.CategoryName,
		"bookmarked_only": filter.BookmarkedOnly,
		"offset":          filter.Offset,
		"limit":           filter.Limit,
		"query":           filter.Query,
	}).Info("Listing articles with filter")

	modelFilter := mappers.ArticleFilterToModel(filter)

	// Call the manager to get articles using the filter
	articles, err := store.Db.ListArticles(*modelFilter)
	if err != nil {
		log.WithError(err).Error("Failed to list articles")
		return nil, err
	}

	protoArticles := mappers.AllArticlesToProto(articles)

	return &protos.ListArticlesResponse{
		Articles: protoArticles,
	}, nil
}

func (s *ArticleServer) GetArticleCounts(_ context.Context, filter *protos.ArticleFilter) (*protos.ArticleCountsResponse, error) {
	log.WithFields(log.Fields{
		"query":      filter.Query,
		"start_date": filter.StartDate,
		"end_date":   filter.EndDate,
	}).Info("Getting article counts with filter")

	modelFilter := mappers.ArticleFilterToModel(filter)
	counts, err := store.Db.GetArticleCounts(*modelFilter)
	if err != nil {
		log.WithError(err).Error("Failed to get article counts")
		return nil, err
	}

	resp := &protos.ArticleCountsResponse{
		AllUnreadCount:  int32(counts.AllUnreadCount),
		BookmarkedCount: int32(counts.BookmarkedCount),
		UnreadByFeed:    make(map[string]int32, len(counts.UnreadByFeed)),
	}
	for feedID, count := range counts.UnreadByFeed {
		resp.UnreadByFeed[feedID] = int32(count)
	}
	return resp, nil
}

// GetArticle implements the GetArticle RPC method
func (s *ArticleServer) GetArticle(_ context.Context, req *protos.GetArticleRequest) (*protos.Article, error) {
	log.WithFields(log.Fields{
		"id": req.Id,
	}).Info("Getting article by Id")

	// Get the article from the manager
	article, err := store.Db.GetArticle(req.Id)
	if err != nil {
		log.WithError(err).Error("Failed to get article")
		return nil, err
	}

	protoArticle := mappers.ArticleToProto(&article)

	return protoArticle, nil
}

// UpdateArticle implements the UpdateArticle RPC method
func (s *ArticleServer) UpdateArticle(_ context.Context, req *protos.UpdateArticleRequest) (*emptypb.Empty, error) {
	log.WithFields(log.Fields{
		"id": req.Id,
	}).Info("Updating article")

	modelsUpdate := mappers.ArticleUpdateToModels(req.Update)

	// Update the article
	return nil, store.Db.UpdateArticle(req.Id, *modelsUpdate)
}

func (s *ArticleServer) MarkFeedArticlesRead(_ context.Context, req *protos.MarkFeedReadRequest) (*emptypb.Empty, error) {
	log.WithField("feed_id", req.FeedId).Info("Marking feed articles as read")

	return &emptypb.Empty{}, store.Db.MarkFeedArticlesAsRead(req.FeedId)
}
