package downlinkclient

import (
	"context"
	"downlink/pkg/mappers"
	"downlink/pkg/models"
	"downlink/pkg/protos"
	"fmt"
	"io"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// Method to get a digest by Id
func (pc *DownlinkClient) GetDigest(id string) (models.Digest, error) {
	protoDigest, err := pc.digestClient.GetDigest(pc.ctx, &protos.GetDigestRequest{Id: id})
	if err != nil {
		return models.Digest{}, err
	}

	if d := mappers.DigestToModel(protoDigest.Digest); d != nil {
		return *d, nil
	}
	return models.Digest{}, nil
}

// Method to list digests
func (pc *DownlinkClient) ListDigests(limit int) ([]models.Digest, error) {
	protoDigests, err := pc.digestClient.ListDigests(pc.ctx, &protos.ListDigestsRequest{Limit: uint32(limit)})
	if err != nil {
		return []models.Digest{}, err
	}

	return mappers.AllDigestsToModels(protoDigests.Digests), nil
}

// Method to get articles for a digest
func (pc *DownlinkClient) GetDigestArticles(digestId string) ([]models.Article, error) {
	protoArticles, err := pc.digestClient.GetDigestArticles(pc.ctx, &protos.GetDigestArticlesRequest{DigestId: digestId})
	if err != nil {
		return []models.Article{}, err
	}

	return mappers.AllArticlesToModels(protoArticles.Articles), nil
}

type GenerateDigestOptions struct {
	StartTime       time.Time
	EndTime         time.Time
	SkipAnalysis    bool
	SkipDuplicates  bool
	ExcludeDigested bool
	SkipSummary     bool
	Theme           string
	OneShotAnalysis bool
	Test            bool
	TestDigestID    string
	GHPagesEnabled  *bool // When non-nil, overrides the server's GitHub Pages enabled config for this run
	OnEvent         func(*protos.DigestProgressEvent)
}

// Method to generate a new digest, streaming progress events to onEvent as they arrive.
// Returns the final digest once the stream completes with a "done" event.
// The provided ctx controls the stream lifetime: cancel it to abort generation on the server.
func (pc *DownlinkClient) GenerateDigest(ctx context.Context, startTime time.Time, endTime time.Time, skipAnalysis bool, skipDuplicates bool, excludeDigested bool, skipSummary bool, theme string, onEvent func(*protos.DigestProgressEvent)) (models.Digest, error) {
	return pc.GenerateDigestWithOptions(ctx, GenerateDigestOptions{
		StartTime:       startTime,
		EndTime:         endTime,
		SkipAnalysis:    skipAnalysis,
		SkipDuplicates:  skipDuplicates,
		ExcludeDigested: excludeDigested,
		SkipSummary:     skipSummary,
		Theme:           theme,
		OnEvent:         onEvent,
	})
}

func (pc *DownlinkClient) GenerateDigestWithOptions(ctx context.Context, options GenerateDigestOptions) (models.Digest, error) {
	stream, err := pc.digestClient.GenerateDigest(ctx, &protos.GenerateDigestRequest{
		StartTime:       timestamppb.New(options.StartTime),
		EndTime:         timestamppb.New(options.EndTime),
		SkipAnalysis:    options.SkipAnalysis,
		SkipDuplicates:  options.SkipDuplicates,
		ExcludeDigested: options.ExcludeDigested,
		SkipSummary:     options.SkipSummary,
		Theme:           options.Theme,
		OneShotAnalysis: options.OneShotAnalysis,
		Test:            options.Test,
		TestDigestId:    options.TestDigestID,
		GhPagesEnabled:  options.GHPagesEnabled,
	})
	if err != nil {
		return models.Digest{}, err
	}

	for {
		ev, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return models.Digest{}, err
		}

		if options.OnEvent != nil {
			options.OnEvent(ev)
		}

		switch ev.Stage {
		case "done":
			if d := mappers.DigestToModel(ev.Digest); d != nil {
				return *d, nil
			}
			return models.Digest{}, nil
		case "cancelled":
			// Server confirmed the cancellation — propagate as a context error so the
			// caller can distinguish it from a real failure.
			return models.Digest{}, context.Canceled
		case "error":
			return models.Digest{}, fmt.Errorf("%s", ev.Error)
		}
	}

	return models.Digest{}, fmt.Errorf("stream ended without a final digest event")
}
