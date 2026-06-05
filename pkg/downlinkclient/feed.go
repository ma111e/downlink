package downlinkclient

import (
	"io"
	"time"

	"github.com/ma111e/downlink/pkg/mappers"
	"github.com/ma111e/downlink/pkg/models"
	"github.com/ma111e/downlink/pkg/protos"

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
// onEvent is called for each streamed event, both STARTED and COMPLETED.
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
func (pc *DownlinkClient) RefreshAllFeedsWithTimeWindow(from, to *time.Time, lastN int, onResult func(res *protos.RefreshFeedResponse, err error)) error {
	feeds, err := pc.ListFeeds()
	if err != nil {
		return err
	}

	for _, feed := range feeds {
		if feed.Enabled != nil && !*feed.Enabled {
			continue
		}
		res, err := pc.RefreshFeedWithTimeWindow(feed.Id, from, to, false, false, lastN)
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

// ApplyFeeds reconciles the server's feeds to match the desired set: feeds in
// configs are created/updated, feeds absent from it are disabled.
func (pc *DownlinkClient) ApplyFeeds(configs []models.FeedConfig, defaults *models.Selectors, dryRun bool) (*protos.ApplyFeedsResponse, error) {
	protoFeeds := make([]*protos.FeedConfig, 0, len(configs))
	for i := range configs {
		pf, err := mappers.FeedConfigToProto(&configs[i])
		if err != nil {
			return nil, err
		}
		protoFeeds = append(protoFeeds, pf)
	}

	req := &protos.ApplyFeedsRequest{
		Feeds:  protoFeeds,
		DryRun: dryRun,
	}
	if defaults != nil {
		req.DefaultSelectors = &protos.Selectors{
			Article:   defaults.Article,
			Cutoff:    defaults.Cutoff,
			Blacklist: defaults.Blacklist,
		}
	}

	return pc.feedsClient.ApplyFeeds(pc.ctx, req)
}

// DeleteFeeds removes the given feeds (by id) from the database.
func (pc *DownlinkClient) DeleteFeeds(feedIds []string, dryRun bool) (*protos.DeleteFeedsResponse, error) {
	return pc.feedsClient.DeleteFeeds(pc.ctx, &protos.DeleteFeedsRequest{
		FeedIds: feedIds,
		DryRun:  dryRun,
	})
}

// RefreshFeedWithTimeWindow refreshes a specific feed with optional time window filtering.
// Returns the per-feed stats from the server.
func (pc *DownlinkClient) RefreshFeedWithTimeWindow(feedId string, from *time.Time, to *time.Time, overwrite bool, restore bool, lastN int) (*protos.RefreshFeedResponse, error) {
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

	if lastN > 0 {
		req.LastN = int32(lastN)
	}

	return pc.feedsClient.RefreshFeed(pc.ctx, req)
}

// InspectFeed probes a feed URL (read-only, pre-registration) and returns its
// diagnosis, sample article links, and detected title — the scaffolding inputs for
// building a feed config.
func (pc *DownlinkClient) InspectFeed(url string, headers map[string]string, maxLinks int) (*protos.InspectFeedResponse, error) {
	return pc.feedsClient.InspectFeed(pc.ctx, &protos.InspectFeedRequest{
		Url:      url,
		Headers:  headers,
		MaxLinks: int32(maxLinks),
	})
}

// InspectArticle scrapes a single article URL in the given mode ("" / static,
// dynamic, full_browser). When selectors are supplied the response also carries the
// extracted content, for testing a candidate selector.
func (pc *DownlinkClient) InspectArticle(url, mode string, headers map[string]string, sel *protos.Selectors, htmlLimit int) (*protos.InspectArticleResponse, error) {
	return pc.feedsClient.InspectArticle(pc.ctx, &protos.InspectArticleRequest{
		Url:       url,
		Mode:      mode,
		Headers:   headers,
		Selectors: sel,
		HtmlLimit: int32(htmlLimit),
	})
}

// AutoBuildFeed runs the server-side autonomous agent that discovers a feed's
// selectors/mode/headers. onEvent is called for each streamed event (STEP updates and
// the final DONE carrying the config YAML).
func (pc *DownlinkClient) AutoBuildFeed(req *protos.AutoBuildFeedRequest, onEvent func(ev *protos.AutoBuildFeedEvent)) error {
	stream, err := pc.feedsClient.AutoBuildFeed(pc.ctx, req)
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

// DiagnoseFeed runs a read-only diagnosis of a single feed: the server fetches and
// parses the feed but stores nothing, returning a structured report of what came
// back over the wire (HTTP status, content type, feed-type guess, parse error,
// UTF-8 problems, and the on-disk path to the saved raw body).
func (pc *DownlinkClient) DiagnoseFeed(feedId string) (*protos.FeedDiagnosis, error) {
	resp, err := pc.feedsClient.RefreshFeed(pc.ctx, &protos.RefreshFeedRequest{
		FeedId:   feedId,
		Diagnose: true,
	})
	if err != nil {
		return nil, err
	}
	return resp.Diagnosis, nil
}
