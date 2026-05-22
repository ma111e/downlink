package services

import (
	"context"
	"downlink/cmd/server/internal/config"
	"downlink/cmd/server/internal/manager"
	"downlink/pkg/mappers"
	"downlink/pkg/models"
	"downlink/pkg/protos"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
	"google.golang.org/protobuf/types/known/emptypb"
)

// FeedsServer implements the FeedsService gRPC service
type FeedsServer struct {
	protos.UnimplementedFeedsServiceServer
	queue *QueueServer
}

// NewFeedsServer creates a new feeds server instance
func NewFeedsServer(queue *QueueServer) *FeedsServer {
	return &FeedsServer{queue: queue}
}

// autoEnqueue submits newly stored articles to the analysis queue when auto_analyze is enabled.
func (s *FeedsServer) autoEnqueue(ctx context.Context, articleIDs []string) {
	if !config.Config.Analysis.AutoAnalyze || len(articleIDs) == 0 || s.queue == nil {
		return
	}
	req := &protos.EnqueueArticlesRequest{
		ArticleIds:   articleIDs,
		ProviderName: config.Config.Analysis.Provider,
	}
	if _, err := s.queue.EnqueueArticles(ctx, req); err != nil {
		log.WithError(err).Warn("Failed to auto-enqueue articles for analysis")
	}
}

// ListFeeds implements the ListFeeds RPC method
func (s *FeedsServer) ListFeeds(_ context.Context, _ *protos.ListFeedsRequest) (*protos.ListFeedsResponse, error) {
	log.WithFields(log.Fields{}).Info("Listing feeds")

	// Call the manager to get articles using the filter
	feeds, err := manager.Manager.ListFeeds()
	if err != nil {
		log.WithError(err).Error("Failed to list feeds")
		return nil, err
	}

	protoFeeds, err := mappers.AllFeedsToProto(feeds)
	if err != nil {
		return nil, err
	}

	return &protos.ListFeedsResponse{
		Feeds: protoFeeds,
	}, nil
}

func (s *FeedsServer) RefreshAllFeeds(req *protos.RefreshAllFeedsRequest, stream protos.FeedsService_RefreshAllFeedsServer) error {
	log.Info("Refreshing all feeds")

	feeds, err := manager.Manager.ListFeeds()
	if err != nil {
		return err
	}

	var enabledFeeds []models.Feed
	for _, f := range feeds {
		if f.Enabled != nil && *f.Enabled {
			enabledFeeds = append(enabledFeeds, f)
		}
	}

	total := int32(len(enabledFeeds))
	log.WithField("total", total).Info("Refreshing enabled feeds")

	type feedEvent struct {
		feed        models.Feed
		fetchResult models.FetchResult
		err         error
	}
	resultCh := make(chan feedEvent, total)

	// Send STARTED per feed then launch its goroutine.
	// Sequential send guarantees all STARTED events reach the client
	// before any COMPLETED event.
	for _, feed := range enabledFeeds {
		if err := stream.Send(&protos.RefreshAllFeedsEvent{
			Result:    &protos.RefreshFeedResponse{FeedId: feed.Id, FeedTitle: feed.Title},
			Total:     total,
			EventType: protos.RefreshEventType_STARTED,
		}); err != nil {
			return err
		}
		go func(f models.Feed) {
			fr, err := manager.Manager.RefreshFeedWithTimeWindow(f.Id, nil, nil, false, false)
			resultCh <- feedEvent{feed: f, fetchResult: fr, err: err}
		}(feed)
	}

	var completed int32
	for range enabledFeeds {
		ev := <-resultCh
		completed++
		s.autoEnqueue(stream.Context(), ev.fetchResult.StoredArticleIDs)
		resp := buildRefreshFeedResponse(ev.feed.Id, ev.feed.Title, ev.fetchResult, ev.err)
		if sendErr := stream.Send(&protos.RefreshAllFeedsEvent{
			Result:    resp,
			Completed: completed,
			Total:     total,
			EventType: protos.RefreshEventType_COMPLETED,
		}); sendErr != nil {
			return sendErr
		}
	}

	return nil
}

func (s *FeedsServer) RefreshFeed(ctx context.Context, req *protos.RefreshFeedRequest) (*protos.RefreshFeedResponse, error) {
	logFields := log.Fields{"feed_id": req.FeedId}

	// Convert proto timestamps to Go time.Time pointers
	var fromTime, toTime *time.Time

	if req.From != nil {
		t := req.From.AsTime()
		fromTime = &t
		logFields["from"] = t.Format(time.RFC3339)
	}

	if req.To != nil {
		t := req.To.AsTime()
		toTime = &t
		logFields["to"] = t.Format(time.RFC3339)
	}

	if req.Overwrite {
		logFields["overwrite"] = true
	}
	log.WithFields(logFields).Info("Refreshing feed")

	fetchResult, err := manager.Manager.RefreshFeedWithTimeWindow(req.FeedId, fromTime, toTime, req.Overwrite, req.Restore)
	if err != nil {
		log.WithError(err).WithField("feed_id", req.FeedId).Error("Failed to refresh feed")
		return nil, err
	}

	s.autoEnqueue(ctx, fetchResult.StoredArticleIDs)

	feed, _ := manager.Manager.GetFeed(req.FeedId)
	return buildRefreshFeedResponse(req.FeedId, feed.Title, fetchResult, nil), nil
}

func buildRefreshFeedResponse(feedId, feedTitle string, fr models.FetchResult, fetchErr error) *protos.RefreshFeedResponse {
	resp := &protos.RefreshFeedResponse{
		FeedId:       feedId,
		FeedTitle:    feedTitle,
		TotalFetched: int32(fr.TotalFetched),
		Stored:       int32(fr.Stored),
		Skipped:      int32(fr.Skipped),
		Errors:       fr.Errors,
	}
	if fetchErr != nil {
		resp.Errors = append(resp.Errors, fetchErr.Error())
	}
	return resp
}

func (s *FeedsServer) DeleteFeed(_ context.Context, req *protos.DeleteFeedRequest) (*emptypb.Empty, error) {
	log.WithField("feedId", req.FeedId).Info("Deleting feed")

	// First, get feed details to identify it in the models
	feed, err := manager.Manager.GetFeed(req.FeedId)
	if err != nil {
		return nil, fmt.Errorf("failed to get feed details: %w", err)
	}

	// Remove from the feed manager and database
	if err := manager.Manager.RemoveFeed(req.FeedId); err != nil {
		return nil, fmt.Errorf("failed to remove feed from database: %w", err)
	}

	log.WithFields(log.Fields{
		"feedId": req.FeedId,
		"title":  feed.Title,
		"url":    feed.URL,
	}).Info("Feed deleted successfully")

	return &emptypb.Empty{}, nil
}

// ApplyFeeds reconciles the stored feeds to match the desired set in the request.
func (s *FeedsServer) ApplyFeeds(_ context.Context, req *protos.ApplyFeedsRequest) (*protos.ApplyFeedsResponse, error) {
	configs := make([]models.FeedConfig, 0, len(req.Feeds))
	for _, pf := range req.Feeds {
		mc, err := mappers.FeedConfigToModel(pf)
		if err != nil {
			return nil, fmt.Errorf("failed to parse feed config: %w", err)
		}
		configs = append(configs, *mc)
	}

	var defaults *models.Selectors
	if req.DefaultSelectors != nil {
		defaults = &models.Selectors{
			Article:   req.DefaultSelectors.Article,
			Cutoff:    req.DefaultSelectors.Cutoff,
			Blacklist: req.DefaultSelectors.Blacklist,
		}
	}

	res, err := manager.Manager.ApplyFeeds(configs, defaults, req.DryRun)
	if err != nil {
		return nil, err
	}

	log.WithFields(log.Fields{
		"created":  len(res.Created),
		"updated":  len(res.Updated),
		"disabled": len(res.Disabled),
		"dry_run":  req.DryRun,
	}).Info("Applied feeds")

	return &protos.ApplyFeedsResponse{
		Created:  res.Created,
		Updated:  res.Updated,
		Disabled: res.Disabled,
	}, nil
}

// DeleteFeeds removes the given feeds (by id) from the database.
func (s *FeedsServer) DeleteFeeds(_ context.Context, req *protos.DeleteFeedsRequest) (*protos.DeleteFeedsResponse, error) {
	res, err := manager.Manager.DeleteFeeds(req.FeedIds, req.DryRun)
	if err != nil {
		return nil, err
	}

	log.WithFields(log.Fields{
		"deleted":   len(res.Deleted),
		"not_found": len(res.NotFound),
		"dry_run":   req.DryRun,
	}).Info("Deleted feeds")

	return &protos.DeleteFeedsResponse{
		Deleted:  res.Deleted,
		NotFound: res.NotFound,
	}, nil
}

func (s *FeedsServer) RegisterFeed(_ context.Context, req *protos.RegisterFeedRequest) (*emptypb.Empty, error) {
	log.WithFields(log.Fields{}).Info("Registering feed")

	feedConfig, err := mappers.FeedConfigToModel(req.FeedConfig)
	if err != nil {
		return nil, err
	}

	// Call the manager to register the feed
	err = manager.Manager.RegisterFeed(*feedConfig)
	if err != nil {
		log.WithError(err).Error("Failed to register feed")
		return nil, err
	}

	return &emptypb.Empty{}, nil
}
