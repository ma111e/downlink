package downlinkclient

import (
	"context"
	"fmt"
	"github.com/ma111e/downlink/pkg/mappers"
	"github.com/ma111e/downlink/pkg/models"
	"github.com/ma111e/downlink/pkg/protos"
	"io"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// GetDigest retrieves a digest by ID.
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

// ListDigests returns lightweight summaries (id/title/date/article count). Use
// ListDigestsFull when the full payload (summary, provider results, analyses) is
// required.
func (pc *DownlinkClient) ListDigests(limit int) ([]models.Digest, error) {
	return pc.listDigests(limit, false)
}

// ListDigestsFull lists digests with their full payload (summary, provider
// results, analyses). The response can be large; prefer ListDigests when only
// summary fields are needed.
func (pc *DownlinkClient) ListDigestsFull(limit int) ([]models.Digest, error) {
	return pc.listDigests(limit, true)
}

func (pc *DownlinkClient) listDigests(limit int, full bool) ([]models.Digest, error) {
	protoDigests, err := pc.digestClient.ListDigests(pc.ctx, &protos.ListDigestsRequest{Limit: uint32(limit), Full: full})
	if err != nil {
		return []models.Digest{}, err
	}

	return mappers.AllDigestsToModels(protoDigests.Digests), nil
}

// GetDigestArticles returns the articles belonging to a digest.
func (pc *DownlinkClient) GetDigestArticles(digestId string) ([]models.Article, error) {
	protoArticles, err := pc.digestClient.GetDigestArticles(pc.ctx, &protos.GetDigestArticlesRequest{DigestId: digestId})
	if err != nil {
		return []models.Article{}, err
	}

	return mappers.AllArticlesToModels(protoArticles.Articles), nil
}

type GenerateDigestOptions struct {
	StartTime              time.Time
	EndTime                time.Time
	SkipAnalysis           bool
	SkipDuplicates         bool
	ExcludeDigested        bool
	Theme                  string
	OneShotAnalysis        bool
	Test                   bool
	TestDigestID           string
	GHPagesEnabled         *bool // When non-nil, overrides the server's GitHub Pages enabled config for this run
	ReanalyzeOnModelChange bool
	Reanalyze              bool
	VibeScore              *bool  // When non-nil, overrides the server's vibe_score config for this run
	Glossary               *bool  // When non-nil, overrides the server's glossary config for this run
	StandardSynthesis      *bool  // When non-nil, overrides the server's standard_synthesis config for this run
	ComprehensiveSynthesis *bool  // When non-nil, overrides the server's comprehensive_synthesis config for this run
	ExecutiveSummary       *bool  // When non-nil, overrides the server's executive_summary config for this run
	Provider               string // Provider override (type or profile name, auto-detected by the server)
	Model                  string // Model override; with empty Provider the server resolves the matching provider
	OnEvent                func(*protos.DigestProgressEvent)
}

// GenerateDigest generates a new digest, streaming progress events to onEvent as they arrive.
// Returns the final digest once the stream completes with a "done" event.
// The provided ctx controls the stream lifetime: cancel it to abort generation on the server.
func (pc *DownlinkClient) GenerateDigest(ctx context.Context, startTime time.Time, endTime time.Time, skipAnalysis bool, skipDuplicates bool, excludeDigested bool, theme string, onEvent func(*protos.DigestProgressEvent)) (models.Digest, error) {
	return pc.GenerateDigestWithOptions(ctx, GenerateDigestOptions{
		StartTime:       startTime,
		EndTime:         endTime,
		SkipAnalysis:    skipAnalysis,
		SkipDuplicates:  skipDuplicates,
		ExcludeDigested: excludeDigested,
		Theme:           theme,
		OnEvent:         onEvent,
	})
}

func (pc *DownlinkClient) GenerateDigestWithOptions(ctx context.Context, options GenerateDigestOptions) (models.Digest, error) {
	stream, err := pc.digestClient.GenerateDigest(ctx, &protos.GenerateDigestRequest{
		StartTime:              timestamppb.New(options.StartTime),
		EndTime:                timestamppb.New(options.EndTime),
		SkipAnalysis:           options.SkipAnalysis,
		SkipDuplicates:         options.SkipDuplicates,
		ExcludeDigested:        options.ExcludeDigested,
		Theme:                  options.Theme,
		OneShotAnalysis:        options.OneShotAnalysis,
		Test:                   options.Test,
		TestDigestId:           options.TestDigestID,
		GhPagesEnabled:         options.GHPagesEnabled,
		ReanalyzeOnModelChange: options.ReanalyzeOnModelChange,
		Reanalyze:              options.Reanalyze,
		VibeScore:              options.VibeScore,
		Glossary:               options.Glossary,
		StandardSynthesis:      options.StandardSynthesis,
		ComprehensiveSynthesis: options.ComprehensiveSynthesis,
		ExecutiveSummary:       options.ExecutiveSummary,
		Provider:               options.Provider,
		Model:                  options.Model,
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
			// Server confirmed the cancellation; propagate as a context error so the
			// caller can distinguish it from a real failure.
			return models.Digest{}, context.Canceled
		case "error":
			return models.Digest{}, fmt.Errorf("%s", ev.Error)
		}
	}

	return models.Digest{}, fmt.Errorf("stream ended without a final digest event")
}
