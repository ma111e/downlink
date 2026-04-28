package downlinkclient

import (
	"io"
	"time"

	"downlink/pkg/mappers"
	"downlink/pkg/models"
	"downlink/pkg/protos"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// ListFeeds returns all registered feeds
func (pc *DownlinkClient) ListFeeds() ([]models.Feed, error) {
	res, err := pc.feedsClient.ListFeeds(pc.ctx, &protos.ListFeedsRequest{})
	if err != nil {
		return nil, err
	}

	return mappers.AllFeedsToModels(res.Feeds)
}

// RegisterFeed adds a new feed to be monitored
func (pc *DownlinkClient) RegisterFeed(feedConfig models.FeedConfig) error {
	protoFeedConfig, err := mappers.FeedConfigToProto(&feedConfig)
	if err != nil {
		return err
	}

	_, err = pc.feedsClient.RegisterFeed(pc.ctx, &protos.RegisterFeedRequest{
		FeedConfig: protoFeedConfig,
	})
	if err != nil {
		return err
	}

	return nil
}

// RefreshAllFeeds triggers a refresh of all registered feeds.
// onEvent is called for each streamed event — both STARTED and COMPLETED.
func (pc *DownlinkClient) RefreshAllFeeds(onEvent func(ev *protos.RefreshAllFeedsEvent)) error {
	stream, err := pc.feedsClient.RefreshAllFeeds(pc.ctx, &protos.RefreshAllFeedsRequest{})
	if err != nil {
		return err
	}

	for {
		ev, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		if onEvent != nil {
			onEvent(ev)
		}
	}

	return nil
}

// RefreshAllFeedsWithTimeWindow refreshes all enabled feeds within the given time window.
// It lists all feeds and calls RefreshFeedWithTimeWindow for each enabled feed.
// onResult is called after each feed completes (result may be nil on error).
func (pc *DownlinkClient) RefreshAllFeedsWithTimeWindow(from, to *time.Time, onResult func(res *protos.RefreshFeedResponse, err error)) error {
	feeds, err := pc.ListFeeds()
	if err != nil {
		return err
	}

	for _, feed := range feeds {
		if feed.Enabled != nil && !*feed.Enabled {
			continue
		}
		res, err := pc.RefreshFeedWithTimeWindow(feed.Id, from, to, false, false)
		if onResult != nil {
			onResult(res, err)
		}
	}

	return nil
}

// DeleteFeed removes a feed from both the models and the database
func (pc *DownlinkClient) DeleteFeed(feedId string) error {
	_, err := pc.feedsClient.DeleteFeed(pc.ctx, &protos.DeleteFeedRequest{
		FeedId: feedId,
	})
	if err != nil {
		return err
	}

	return nil
}

// RefreshFeedWithTimeWindow refreshes a specific feed with optional time window filtering.
// Returns the per-feed stats from the server.
func (pc *DownlinkClient) RefreshFeedWithTimeWindow(feedId string, from *time.Time, to *time.Time, overwrite bool, restore bool) (*protos.RefreshFeedResponse, error) {
	req := &protos.RefreshFeedRequest{
		FeedId:    feedId,
		Overwrite: overwrite,
		Restore:   restore,
	}

	if from != nil {
		req.From = timestamppb.New(*from)
	}

	if to != nil {
		req.To = timestamppb.New(*to)
	}

	return pc.feedsClient.RefreshFeed(pc.ctx, req)
}
