package downlinkclient

import (
	"downlink/pkg/mappers"
	"downlink/pkg/models"
	"downlink/pkg/protos"

	"google.golang.org/protobuf/types/known/emptypb"

	log "github.com/sirupsen/logrus"
)

// ListArticles returns articles based on the provided filter
func (pc *DownlinkClient) ListArticles(filter models.ArticleFilter) ([]models.Article, error) {
	log.WithFields(log.Fields{
		"unread_only":      filter.UnreadOnly,
		"category_name":    filter.CategoryName,
		"bookmarked_only":  filter.BookmarkedOnly,
		"feed_id":          filter.FeedId,
		"start_date":       filter.StartDate,
		"end_date":         filter.EndDate,
		"exclude_digested": filter.ExcludeDigested,
	}).Info("Requesting articles with filter")

	protoFilter := mappers.ArticleFilterToProto(&filter)

	res, err := pc.articleClient.ListArticles(pc.ctx, protoFilter)
	if err != nil {
		return nil, err
	}

	return mappers.AllArticlesToModels(res.Articles), nil
}

func (pc *DownlinkClient) GetArticleCounts(filter models.ArticleFilter) (models.ArticleCounts, error) {
	protoFilter := mappers.ArticleFilterToProto(&filter)

	res, err := pc.articleClient.GetArticleCounts(pc.ctx, protoFilter)
	if err != nil {
		return models.ArticleCounts{}, err
	}

	counts := models.ArticleCounts{
		AllUnreadCount:  int64(res.GetAllUnreadCount()),
		BookmarkedCount: int64(res.GetBookmarkedCount()),
		UnreadByFeed:    make(map[string]int64, len(res.GetUnreadByFeed())),
	}
	for feedID, count := range res.GetUnreadByFeed() {
		counts.UnreadByFeed[feedID] = int64(count)
	}

	return counts, nil
}

// GetArticle returns a single article by Id
func (pc *DownlinkClient) GetArticle(id string) (models.Article, error) {
	log.WithFields(log.Fields{
		"id": id,
	}).Info("Getting article by Id")

	req := &protos.GetArticleRequest{
		Id: id,
	}

	res, err := pc.articleClient.GetArticle(pc.ctx, req)
	if err != nil {
		return models.Article{}, err
	}

	if a := mappers.ArticleToModel(res); a != nil {
		return *a, nil
	}
	return models.Article{}, nil
}

// UpdateArticle updates an article's properties
func (pc *DownlinkClient) UpdateArticle(id string, update *models.ArticleUpdate) (*emptypb.Empty, error) {
	log.WithFields(log.Fields{
		"id": id,
	}).Info("Updating article")
	protoUpdate := mappers.ArticleUpdateToProto(update)

	req := &protos.UpdateArticleRequest{
		Id:     id,
		Update: protoUpdate,
	}
	return pc.articleClient.UpdateArticle(pc.ctx, req)
}

func (pc *DownlinkClient) MarkFeedArticlesRead(feedId string) error {
	_, err := pc.articleClient.MarkFeedArticlesRead(pc.ctx, &protos.MarkFeedReadRequest{
		FeedId: feedId,
	})
	return err
}
