package services

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/ma111e/downlink/cmd/server/internal/config"
	"github.com/ma111e/downlink/cmd/server/internal/notification"
	"github.com/ma111e/downlink/cmd/server/internal/store"
	"github.com/ma111e/downlink/pkg/llmgateway"
	"github.com/ma111e/downlink/pkg/llmprovider"
	"github.com/ma111e/downlink/pkg/llmutil"
	"github.com/ma111e/downlink/pkg/mappers"
	"github.com/ma111e/downlink/pkg/models"
	"github.com/ma111e/downlink/pkg/protos"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

// DigestServer implements the DigestService gRPC service
type DigestServer struct {
	protos.UnimplementedDigestServiceServer

	gw   *llmgateway.Gateway
	llms *LLMsServer
}

const preferredNotificationTestDigestID = "digest-498edee4"

// NewDigestServer creates a new Digest server instance. The gateway is the
// single chokepoint for LLM calls (dedupe, summary, article analysis during
// ensureArticlesAnalyzed); llms is reused so we don't re-construct an
// LLMsServer on every digest.
func NewDigestServer(gw *llmgateway.Gateway, llms *LLMsServer) *DigestServer {
	return &DigestServer{gw: gw, llms: llms}
}

// ListDigests retrieves all digests
func (s *DigestServer) ListDigests(ctx context.Context, req *protos.ListDigestsRequest) (*protos.ListDigestsResponse, error) {
	digests, err := store.Db.ListDigests(int(req.Limit), req.Full)
	if err != nil {
		return nil, err
	}

	return &protos.ListDigestsResponse{
		Digests: mappers.AllDigestsToProto(digests),
	}, nil
}

// GetDigest returns a single digest by Id
func (s *DigestServer) GetDigest(ctx context.Context, req *protos.GetDigestRequest) (*protos.GetDigestResponse, error) {
	digest, err := store.Db.GetDigest(req.Id)
	if err != nil {
		return nil, err
	}

	return &protos.GetDigestResponse{
		Digest: mappers.DigestToProto(&digest),
	}, nil
}

// GetDigestArticles returns articles for a digest
func (s *DigestServer) GetDigestArticles(ctx context.Context, req *protos.GetDigestArticlesRequest) (*protos.GetDigestArticlesResponse, error) {
	digest, err := store.Db.GetDigest(req.DigestId)
	if err != nil {
		return nil, err
	}

	articles, err := store.Db.GetDigestArticles(digest.Id)
	if err != nil {
		return nil, err
	}

	return &protos.GetDigestArticlesResponse{
		Articles: mappers.AllArticlesToProto(articles),
	}, nil
}

// safeStream wraps a DigestService stream with a mutex so parallel workers
// (e.g. the per-article analysis goroutines in ensureArticlesAnalyzed) can
// send progress events without racing. gRPC server streams are not safe for
// concurrent Send.
type safeStream struct {
	mu     sync.Mutex
	stream protos.DigestService_GenerateDigestServer
}

func (s *safeStream) Send(ev *protos.DigestProgressEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stream.Send(ev)
}

// sendProgress sends a progress event to the stream. Errors are logged but not fatal.
func sendProgress(stream *safeStream, stage, msg string, cur, total uint32) {
	if err := stream.Send(&protos.DigestProgressEvent{
		Stage:   stage,
		Message: msg,
		Current: cur,
		Total:   total,
	}); err != nil {
		log.WithError(err).WithField("stage", stage).Debug("Failed to send progress event")
	}
}

// cancelled returns true if the stream's context has been cancelled by the client.
// When it returns true, it logs the cancellation and emits a "cancelled" event so
// the CLI can wait for this acknowledgment before exiting cleanly.
func cancelled(stream *safeStream) bool {
	if err := stream.stream.Context().Err(); err != nil {
		log.WithError(err).Info("Digest generation cancelled by client")
		_ = stream.Send(&protos.DigestProgressEvent{
			Stage:   "cancelled",
			Message: "digest generation cancelled",
		})
		return true
	}
	return false
}

// resolveRunProviderModel picks the effective provider/model for a digest run:
// an explicit per-run override (either field) wins; otherwise the profile/global
// editorial defaults apply. A model given without a provider is honored as-is so
// ResolveLLM can find the provider offering it; only when neither is set do we
// fall back to the configured defaults.
func resolveRunProviderModel(req *protos.GenerateDigestRequest, ded EffectiveEditorial) (provider, model string) {
	if req.Provider != "" || req.Model != "" {
		return req.Provider, req.Model
	}
	return ded.Provider, ded.Model
}

// GenerateDigest generates a new digest, streaming progress events throughout the process
func (s *DigestServer) GenerateDigest(req *protos.GenerateDigestRequest, rawStream protos.DigestService_GenerateDigestServer) error {
	// Parallel analysis workers race on stream.Send; serialize through safeStream.
	stream := &safeStream{stream: rawStream}

	ctx := rawStream.Context()
	if req.GetTest() {
		log.Info("Sending stored digest to notification channels for test")
		return s.sendNotificationTestDigest(req, stream)
	}

	// Resolve the editorial profile this digest is generated for. An empty slug
	// means the "default" profile (which inherits the live global config). Every
	// downstream step — article selection, analysis, dedupe, summary, storage —
	// is scoped to this profile.
	profileSlug := req.GetProfileSlug()
	if profileSlug == "" {
		profileSlug = defaultProfileId
	}
	profile, err := store.Db.GetProfile(profileSlug)
	if err != nil {
		_ = stream.Send(&protos.DigestProgressEvent{Stage: "error", Error: fmt.Sprintf("unknown profile %q: %v", profileSlug, err)})
		return fmt.Errorf("failed to load profile %q: %w", profileSlug, err)
	}
	ded := resolveDigestEditorial(req, profile)
	// The effective provider/model: an explicit run override wins, otherwise the
	// profile's (or global) configured provider drives all LLM steps of this run.
	effProvider, effModel := resolveRunProviderModel(req, ded)

	log.WithField("profile", profile.Id).Info("Generating digest from recent articles")

	// Start a monitoring run before any LLM work: every gateway call made under
	// this ctx is recorded against runID for the LLM monitor page. The digest id
	// (linked below) does not exist yet, so the run has its own id.
	runID := generateRunId(time.Now())
	ctx = llmgateway.WithRunID(ctx, runID)
	if err := store.Db.StartLLMRun(runID, profile.Id, time.Now()); err != nil {
		log.WithError(err).Warn("failed to start LLM monitor run")
	}
	defer func() {
		if err := store.Db.FinishLLMRun(runID, time.Now()); err != nil {
			log.WithError(err).WithField("run_id", runID).Warn("failed to finish LLM monitor run")
		}
		if err := store.Db.PruneLLMRuns(LLMMonitorRetention); err != nil {
			log.WithError(err).Warn("failed to prune old LLM monitor runs")
		}
	}()

	windowStart := req.StartTime.AsTime()
	windowEnd := time.Now()
	if req.EndTime != nil {
		windowEnd = req.EndTime.AsTime()
	}
	windowDuration := windowEnd.Sub(windowStart)

	// Fetch articles
	sendProgress(stream, "fetch", "fetching articles...", 0, 0)
	articles, err := s.getRecentArticles(windowStart, &windowEnd, req.ExcludeDigested, profile.Id)
	if err != nil {
		_ = stream.Send(&protos.DigestProgressEvent{Stage: "error", Error: fmt.Sprintf("failed to get recent articles: %v", err)})
		return fmt.Errorf("failed to get recent articles: %w", err)
	}

	if len(articles) == 0 {
		_ = stream.Send(&protos.DigestProgressEvent{Stage: "error", Error: "no articles found within the time window"})
		return fmt.Errorf("no articles found within the time window")
	}

	sendProgress(stream, "fetch", fmt.Sprintf("found %d articles", len(articles)), uint32(len(articles)), uint32(len(articles)))
	log.WithField("count", len(articles)).Info("Found articles for digest")

	// Step 1: Ensure all articles have been analyzed; trigger analysis for those that haven't
	var analyses []models.ArticleAnalysis
	var analysisErrors map[string]string // articleId → human-readable error, transient
	if req.SkipAnalysis {
		log.Info("Skipping article analysis (skip_analysis requested)")
		articleIds := make([]string, len(articles))
		for i, a := range articles {
			articleIds[i] = a.Id
		}
		analysisMap, err := store.Db.GetArticleAnalysesBatch(articleIds, profile.Id)
		if err != nil {
			log.WithError(err).Warn("Failed to batch fetch analyses for skip_analysis mode")
		} else {
			for _, article := range articles {
				if a := analysisMap[article.Id]; a != nil {
					analyses = append(analyses, *a)
				}
			}
		}
	} else {
		onAnalysisStart := func(articleId, articleTitle string, current, total uint32) {
			_ = stream.Send(&protos.DigestProgressEvent{
				Stage:        "analyze",
				Message:      articleTitle,
				Current:      current,
				Total:        total,
				ArticleId:    articleId,
				ArticleTitle: articleTitle,
			})
		}
		// onTaskProgress is per-article: ensureArticlesAnalyzed builds a fresh
		// closure for each worker that bakes in the article's id/title.
		onTaskProgress := func(articleId, articleTitle string) func(string, string, int, int, error) {
			return func(taskName, status string, taskIndex, totalTasks int, taskErr error) {
				ev := &protos.DigestProgressEvent{
					Stage:        "analyze_task",
					TaskName:     taskName,
					TaskStatus:   status,
					TaskIndex:    uint32(taskIndex),
					TaskTotal:    uint32(totalTasks),
					ArticleId:    articleId,
					ArticleTitle: articleTitle,
				}
				if taskErr != nil {
					ev.Error = taskErr.Error()
				}
				_ = stream.Send(ev)
			}
		}
		analyses, analysisErrors, err = s.ensureArticlesAnalyzed(ctx, articles, req.OneShotAnalysis, req.ReanalyzeOnModelChange, req.Reanalyze, req.VibeScore, req.Glossary, req.StandardSynthesis, req.ComprehensiveSynthesis, effProvider, effModel, profile.Id, onAnalysisStart, onTaskProgress)
		if err != nil {
			if cancelled(stream) {
				return ctx.Err()
			}
			_ = stream.Send(&protos.DigestProgressEvent{Stage: "error", Error: fmt.Sprintf("failed to ensure articles are analyzed: %v", err)})
			return fmt.Errorf("failed to ensure articles are analyzed: %w", err)
		}
		if len(analysisErrors) > 0 {
			msg := fmt.Sprintf("%d article(s) could not be analyzed after retry", len(analysisErrors))
			log.Warn(msg)
			sendProgress(stream, "analyze", "warning: "+msg, 0, 0)
		}
	}

	if cancelled(stream) {
		return ctx.Err()
	}

	log.WithField("analysisCount", len(analyses)).Info("Articles analyzed")

	// Pre-fetch articles for all analyses in a single batch query (used in steps 2 & 3)
	analysisArticleIds := make([]string, len(analyses))
	for i, a := range analyses {
		analysisArticleIds[i] = a.ArticleId
	}
	articlesBatch, batchErr := store.Db.GetArticlesBatch(analysisArticleIds)
	articleMap := make(map[string]models.Article, len(articlesBatch))
	if batchErr != nil {
		log.WithError(batchErr).Warn("Failed to batch fetch articles for digest steps, falling back to per-article lookup")
	} else {
		for _, a := range articlesBatch {
			articleMap[a.Id] = a
		}
	}

	// Step 2: Build a prompt from key points and ask the LLM for duplicate grouping
	var groupingResult *duplicateGroupingResult
	var rawResponse string
	if req.SkipDuplicates {
		log.Info("Skipping duplicate detection (skip_duplicates requested)")
		groupingResult = &duplicateGroupingResult{}
	} else {
		sendProgress(stream, "dedupe", "identifying duplicate articles...", 0, 0)
		groupingResult, rawResponse, err = s.identifyDuplicates(ctx, analyses, articleMap, effProvider, effModel, ded)
		if err != nil {
			_ = stream.Send(&protos.DigestProgressEvent{Stage: "error", Error: fmt.Sprintf("failed to identify duplicates: %v", err)})
			return fmt.Errorf("failed to identify duplicates: %w", err)
		}
		sendProgress(stream, "dedupe", fmt.Sprintf("found %d duplicate groups", len(groupingResult.DuplicateGroups)), 0, 0)
	}

	// Step 3: Generate digest title (always) and optional executive summary
	var digestTitle, digestSummary string
	var summaryProviderType, summaryModelName string
	withSummary := ded.ExecutiveSummary
	sendProgress(stream, "summarize", "generating digest title...", 0, 0)
	digestTitle, digestSummary, summaryProviderType, summaryModelName, err = s.generateDigestSummary(ctx, analyses, articleMap, windowStart, windowEnd, effProvider, effModel, withSummary, ded)
	if err != nil {
		log.WithError(err).Warn("Failed to generate digest title/summary, continuing without it")
	} else {
		msg := "digest title generated"
		if withSummary {
			msg = "digest summary generated"
		}
		sendProgress(stream, "summarize", msg, 0, 0)
	}

	// Step 4: Build the digest object
	sendProgress(stream, "store", "storing digest...", 0, 0)
	now := time.Now()
	articleLen := len(articles)
	zero := 0
	digest := models.Digest{
		Id:                  generateDigestId(now),
		ProfileId:           profile.Id,
		CreatedAt:           windowStart,
		ArticleCount:        &zero, // StoreDigestArticlesBatch will increment to the real count
		TimeWindow:          windowDuration,
		RawGroupingResponse: rawResponse,
		Title:               digestTitle,
		DigestSummary:       digestSummary,
	}

	// Store the digest
	if err = store.Db.StoreDigest(digest); err != nil {
		_ = stream.Send(&protos.DigestProgressEvent{Stage: "error", Error: fmt.Sprintf("failed to store digest: %v", err)})
		return fmt.Errorf("failed to store digest: %w", err)
	}

	// Attribute this monitoring run to the produced digest for the monitor page.
	if err := store.Db.LinkLLMRunToDigest(runID, digest.Id, digestTitle); err != nil {
		log.WithError(err).WithField("run_id", runID).Warn("failed to link LLM monitor run to digest")
	}

	// Store DigestProviderResult so the HTML notification can render provider syntheses
	providerType := summaryProviderType
	modelName := summaryModelName
	if providerType == "" {
		if ap, apErr := findEnabledProviderByName(ded.Provider); apErr == nil {
			providerType = ap.ProviderType
			modelName = ap.ModelName
		}
	}
	if providerType != "" {
		providerResult := models.DigestProviderResult{
			DigestId:               digest.Id,
			ProviderType:           providerType,
			ModelName:              modelName,
			ComprehensiveSynthesis: digestSummary,
		}
		if err := store.Db.StoreDigestProviderResult(providerResult); err != nil {
			log.WithError(err).Warn("Failed to store digest provider result")
		}
	}

	// Step 5: Build and store DigestAnalysis entries with duplicate markers
	digestAnalyses := s.buildDigestAnalyses(digest.Id, analyses, groupingResult)
	if err = store.Db.StoreDigestAnalysesBatch(digestAnalyses); err != nil {
		log.WithError(err).WithField("digestId", digest.Id).Warn("Failed to batch store digest analysis entries")
	}
	digest.DigestAnalyses = digestAnalyses

	// Store digest-article associations for all fetched articles; articles
	// without analysis will be rendered with an error message in the digest.
	articleIds := make([]string, len(articles))
	for i, article := range articles {
		articleIds[i] = article.Id
	}
	if err = store.Db.StoreDigestArticlesBatch(digest.Id, articleIds); err != nil {
		log.WithError(err).WithField("digestId", digest.Id).Warn("Failed to batch store digest-article associations")
	}
	digest.ArticleCount = &articleLen

	// Step 6: Populate the persistent global glossary from this digest (opt-in). Failures
	// here must never fail digest generation, so errors are logged as warnings only.
	glossaryEntries := 0
	if ded.Glossary {
		sendProgress(stream, "glossary", "building glossary...", 0, 0)
		n, err := s.populateGlossary(ctx, digest.Id, analyses, articleMap, effProvider, effModel)
		if err != nil {
			log.WithError(err).WithField("digestId", digest.Id).Warn("Failed to populate glossary")
		}
		glossaryEntries = n
		sendProgress(stream, "glossary", fmt.Sprintf("glossary built (%d terms)", glossaryEntries), 0, 0)
	}

	log.WithFields(log.Fields{
		"id":              digest.Id,
		"articleCount":    articleLen,
		"skipped":         articleLen - len(digestAnalyses),
		"analysisCount":   len(digestAnalyses),
		"duplicateGroups": len(groupingResult.DuplicateGroups),
		"glossaryEntries": glossaryEntries,
	}).Info("Digest generated successfully")

	// Send notifications if configured. Reload digest once so Articles,
	// ProviderResults, and DigestAnalyses are populated for renderers.
	if fullDigest, err := store.Db.GetDigest(digest.Id); err != nil {
		log.WithError(err).Warn("Failed to reload digest for notifications, skipping all")
	} else {
		fullDigest.AnalysisErrors = analysisErrors
		// Layout precedence: explicit per-run --theme override, else the profile's
		// layout (the server/global default applies when both are empty).
		effLayout := req.GetTheme()
		if effLayout == "" {
			effLayout = profile.Layout
		}
		if _, err := sendConfiguredDigestNotifications(stream, fullDigest, effLayout, false, req.GhPagesEnabled, profile); err != nil {
			log.WithError(err).Warn("Failed to send one or more digest notifications")
		}
	}

	// Final event: send the completed digest
	if err := stream.Send(&protos.DigestProgressEvent{
		Stage:   "done",
		Message: fmt.Sprintf("digest %s generated with %d articles", digest.Id, articleLen),
		Digest:  mappers.DigestToProto(&digest),
	}); err != nil {
		return fmt.Errorf("failed to send final digest event: %w", err)
	}

	return nil
}

func (s *DigestServer) sendNotificationTestDigest(req *protos.GenerateDigestRequest, stream *safeStream) error {
	sendProgress(stream, "notify", "loading test digest...", 0, 0)

	digestID, err := s.resolveNotificationTestDigestID(req.GetTestDigestId())
	if err != nil {
		_ = stream.Send(&protos.DigestProgressEvent{Stage: "error", Error: err.Error()})
		return err
	}

	digest, err := store.Db.GetDigest(digestID)
	if err != nil {
		err = fmt.Errorf("failed to load test digest %q: %w", digestID, err)
		_ = stream.Send(&protos.DigestProgressEvent{Stage: "error", Error: err.Error()})
		return err
	}

	// Resolve the digest's profile (fall back to default) so the test publishes
	// under the right profile section and layout.
	testProfileSlug := digest.ProfileId
	if testProfileSlug == "" {
		testProfileSlug = defaultProfileId
	}
	testProfile, perr := store.Db.GetProfile(testProfileSlug)
	if perr != nil {
		testProfile = models.Profile{Id: defaultProfileId}
	}
	testLayout := req.GetTheme()
	if testLayout == "" {
		testLayout = testProfile.Layout
	}
	attempts, err := sendConfiguredDigestNotifications(stream, digest, testLayout, true, req.GhPagesEnabled, testProfile)
	if err != nil {
		_ = stream.Send(&protos.DigestProgressEvent{Stage: "error", Error: err.Error()})
		return err
	}
	if attempts == 0 {
		err := errors.New("no notification channels are configured")
		_ = stream.Send(&protos.DigestProgressEvent{Stage: "error", Error: err.Error()})
		return err
	}

	if err := stream.Send(&protos.DigestProgressEvent{
		Stage:   "done",
		Message: fmt.Sprintf("sent test digest %s to %d notification channel(s)", digest.Id, attempts),
		Digest:  mappers.DigestToProto(&digest),
	}); err != nil {
		return fmt.Errorf("failed to send final test digest event: %w", err)
	}
	return nil
}

func (s *DigestServer) resolveNotificationTestDigestID(requestedID string) (string, error) {
	if requestedID != "" {
		return requestedID, nil
	}

	if _, err := store.Db.GetDigest(preferredNotificationTestDigestID); err == nil {
		return preferredNotificationTestDigestID, nil
	}

	// Needs full payloads: notificationTestDigestScore reads DigestSummary,
	// ProviderResults, and DigestAnalyses.
	digests, err := store.Db.ListDigests(0, true)
	if err != nil {
		return "", fmt.Errorf("failed to list digests for notification test selection: %w", err)
	}
	if len(digests) == 0 {
		return "", errors.New("no digests are available for notification testing")
	}

	best := digests[0]
	bestScore := notificationTestDigestScore(best)
	for _, candidate := range digests[1:] {
		score := notificationTestDigestScore(candidate)
		if score > bestScore {
			best = candidate
			bestScore = score
		}
	}

	return best.Id, nil
}

func notificationTestDigestScore(digest models.Digest) int {
	score := 0
	if digest.ArticleCount != nil {
		score += *digest.ArticleCount
	}
	if digest.DigestSummary != "" {
		score += 200
	}
	score += len(digest.ProviderResults) * 100

	duplicateGroups := map[string]struct{}{}
	for _, analysis := range digest.DigestAnalyses {
		if analysis.DuplicateGroup == "" {
			continue
		}
		duplicateGroups[analysis.DuplicateGroup] = struct{}{}
		score += 20
		if analysis.IsMostComprehensive {
			score += 10
		}
	}
	score += len(duplicateGroups) * 200

	return score
}

// profileSubdir resolves the concrete GitHub Pages output subdirectory for a
// profile: its explicit OutputSubdir, else the global output dir for the default
// profile, else the slug. Empty resolves to "digests" to match
// resolveGitHubPagesOutputDir's default.
func profileSubdir(profile models.Profile) string {
	sub := profile.OutputSubdir
	if sub == "" {
		if profile.Id == defaultProfileId {
			sub = config.Config.Notifications.GitHubPages.OutputDir
		} else {
			sub = profile.Id
		}
	}
	if sub == "" {
		sub = "digests"
	}
	return sub
}

// buildLandingProfiles returns the enabled profiles as landing entries (for the
// root landing page + profiles.json), each with its resolved output subdir.
func buildLandingProfiles() []notification.LandingProfile {
	profiles, err := store.Db.ListProfiles()
	if err != nil {
		log.WithError(err).Warn("Failed to list profiles for landing page")
		return nil
	}
	var entries []notification.LandingProfile
	for _, p := range profiles {
		if p.Enabled != nil && !*p.Enabled {
			continue
		}
		entries = append(entries, notification.LandingProfile{
			Slug:        p.Id,
			Name:        p.Name,
			Description: p.Description,
			Icon:        p.Icon,
			Subdir:      profileSubdir(p),
		})
	}
	return entries
}

func sendConfiguredDigestNotifications(stream *safeStream, digest models.Digest, layout string, failOnError bool, ghPagesOverride *bool, profile models.Profile) (int, error) {
	var errs []error
	attempts := 0

	discordEnabled := config.Config.Notifications.Discord.Enabled && config.Config.Notifications.Discord.WebhookURL != ""
	if discordEnabled {
		attempts++
		sendProgress(stream, "notify", "sending Discord notification...", 0, 0)
		notifier := notification.NewDiscordNotifier(config.Config.Notifications.Discord.WebhookURL)
		if err := notifier.SendDigest(digest); err != nil {
			log.WithError(err).Warn("Failed to send Discord notification")
			errs = append(errs, fmt.Errorf("discord digest: %w", err))
		}

		htmlBytes, err := notification.RenderDigestHTML(digest, layout, profile.Theme)
		if err != nil {
			log.WithError(err).Warn("Failed to render digest HTML")
			errs = append(errs, fmt.Errorf("discord html render: %w", err))
		} else {
			filename := notification.DigestHTMLFilename(digest)
			if err := notifier.SendHTMLFile(filename, htmlBytes); err != nil {
				log.WithError(err).Warn("Failed to send digest HTML to Discord")
				errs = append(errs, fmt.Errorf("discord html upload: %w", err))
			}
		}
	}

	ghPagesConfigEnabled := config.Config.Notifications.GitHubPages.Enabled
	if ghPagesOverride != nil {
		ghPagesConfigEnabled = *ghPagesOverride
	}
	ghPagesEnabled := ghPagesConfigEnabled && config.Config.Notifications.GitHubPages.RepoURL != ""
	if ghPagesEnabled {
		attempts++
		sendProgress(stream, "notify", "publishing digest to GitHub Pages...", 0, 0)
		ghCfg := config.Config.Notifications.GitHubPages
		// Per-profile publishing: this profile gets its own output subdirectory and
		// layout, and its archive/feeds/sources are scoped to its digests + feeds.
		ghCfg.OutputDir = profileSubdir(profile)
		ghCfg.Layout = layout
		if ghCfg.Token == "" {
			ghCfg.Token = os.Getenv("DOWNLINK_GH_PAGES_TOKEN")
		}
		if ghCfg.Token == "" {
			err := errors.New("GitHub Pages enabled but no token configured (set token in config or DOWNLINK_GH_PAGES_TOKEN env)")
			log.Warn(err)
			errs = append(errs, err)
		} else {
			profileID := profile.Id
			publisher := notification.NewGitHubPagesPublisher(ghCfg)
			publisher.SetProfileContext(profile.Id, profile.Theme)
			publisher.SetDigestLister(func(n int) ([]models.Digest, error) {
				return store.Db.ListDigestsByProfile(profileID, n, true)
			})
			publisher.SetSourceLister(func() ([]models.Feed, error) {
				return store.Db.ListProfileFeeds(profileID)
			})
			// When more than one profile exists, the repo root becomes a landing
			// page linking into each profile's section.
			publisher.SetLanding(buildLandingProfiles())
			if err := publisher.SendDigest(digest); err != nil {
				log.WithError(err).Warn("Failed to publish digest to GitHub Pages")
				errs = append(errs, fmt.Errorf("github pages: %w", err))
			}
		}
	}

	if failOnError && len(errs) > 0 {
		return attempts, errors.Join(errs...)
	}
	return attempts, nil
}

// getRecentArticles retrieves articles published within the given time range.
// If excludeDigested is true, articles already included in a previous digest are excluded.
func (s *DigestServer) getRecentArticles(after time.Time, before *time.Time, excludeDigested bool, profileId string) ([]models.Article, error) {
	filter := models.ArticleFilter{
		StartDate:       &after,
		EndDate:         before,
		ExcludeDigested: excludeDigested,
		// Restrict to the profile's feed pool: a digest only ever covers articles
		// from feeds the profile subscribes to.
		ProfileId: profileId,
		// The digest covers every article in the window, not a UI page, so
		// bypass the default page cap that ListArticles otherwise applies.
		Unbounded: true,
	}

	articles, err := store.Db.ListArticles(filter)
	if err != nil {
		return nil, fmt.Errorf("failed to list articles: %w", err)
	}

	return articles, nil
}

// ensureArticlesAnalyzed checks each article for an existing analysis and
// triggers analysis for any articles that haven't been analyzed yet. The
// gateway enforces the global LLM-call cap, so this loop runs articles in
// parallel via a bounded worker pool.
//
// onStart is called when an article's analysis begins; the CLI uses this to
// register a new row and reserve screen space for concurrent work.
// onTaskFactory returns a per-article task-progress callback so the emitted
// events carry the article's id/title (parallel articles' events would
// otherwise be indistinguishable downstream).
func (s *DigestServer) ensureArticlesAnalyzed(
	ctx context.Context,
	articles []models.Article,
	oneShotAnalysis bool,
	reanalyzeOnModelChange bool,
	reanalyze bool,
	vibeScore *bool,
	glossary *bool,
	standardSynthesis *bool,
	comprehensiveSynthesis *bool,
	provider string,
	model string,
	profileId string,
	onStart func(articleId, articleTitle string, current, total uint32),
	onTaskFactory func(articleId, articleTitle string) func(taskName, status string, taskIndex, totalTasks int, err error),
) ([]models.ArticleAnalysis, map[string]string, error) {
	// Batch-fetch existing analyses for this profile in one query. Scoping to the
	// profile means an article analyzed under another profile is treated as
	// un-analyzed here, so each profile gets its own editorial pass.
	articleIds := make([]string, len(articles))
	for i, a := range articles {
		articleIds[i] = a.Id
	}
	analysisMap, err := store.Db.GetArticleAnalysesBatch(articleIds, profileId)
	if err != nil {
		log.WithError(err).Warn("Failed to batch fetch article analyses, falling back to per-article lookup")
		analysisMap = make(map[string]*models.ArticleAnalysis)
	}

	var currentProviderType, currentModelName string
	if reanalyzeOnModelChange {
		resolved, resolveErr := ResolveLLM(LLMRequest{Provider: provider, ModelName: model, MaxTokens: defaultMaxTokensLarge})
		if resolveErr != nil {
			log.WithError(resolveErr).Warn("reanalyze-on-model-change: could not resolve current model, will re-analyze all articles with existing analyses")
		} else {
			currentProviderType = resolved.ProviderType
			currentModelName = resolved.ModelName
		}
	}

	type glossaryBackfillJob struct {
		article  models.Article
		analysis *models.ArticleAnalysis
	}
	var needsAnalysis []models.Article
	var needsGlossary []glossaryBackfillJob
	for _, article := range articles {
		if reanalyze {
			needsAnalysis = append(needsAnalysis, article)
			continue
		}
		existing := analysisMap[article.Id]
		if existing == nil {
			needsAnalysis = append(needsAnalysis, article)
			continue
		}
		if reanalyzeOnModelChange {
			modelChanged := currentProviderType == "" ||
				existing.ProviderType != currentProviderType ||
				existing.ModelName != currentModelName
			if modelChanged {
				log.WithFields(log.Fields{
					"articleId":        article.Id,
					"existingProvider": existing.ProviderType,
					"existingModel":    existing.ModelName,
					"currentProvider":  currentProviderType,
					"currentModel":     currentModelName,
				}).Info("Re-analyzing article: analysis model differs from current model")
				needsAnalysis = append(needsAnalysis, article)
				continue
			}
		}
		if glossary != nil && *glossary && existing.GlossaryTermsJson == "" {
			log.WithField("articleId", article.Id).Info("Queuing glossary backfill: analysis exists but has no glossary terms")
			needsGlossary = append(needsGlossary, glossaryBackfillJob{article: article, analysis: existing})
		}
	}

	if len(needsAnalysis) > 0 {
		log.WithField("count", len(needsAnalysis)).Info("Triggering analysis for unanalyzed articles")
	}
	if len(needsGlossary) > 0 {
		log.WithField("count", len(needsGlossary)).Info("Triggering glossary backfill for analyses without glossary terms")
	}

	// Worker pool matches the gateway's LLM cap exactly. With the cap as the
	// upper bound, every running worker is guaranteed to be holding a slot
	// (not parked on the semaphore), so the "N in parallel" the UI shows
	// always reflects real in-flight LLM calls. Any extra workers would just
	// sit on the gateway semaphore and emit misleading progress events
	// before they actually do work.
	workerLimit := s.gw.MaxConcurrent()
	if workerLimit < 1 {
		workerLimit = 1
	}

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(workerLimit)

	var (
		analysisMu       sync.Mutex
		firstErrMu       sync.Mutex
		firstAnalysisErr error
		completed        atomic.Uint32
		articleErrors    = make(map[string]string)
		total            = uint32(len(needsAnalysis))
	)

	captureErr := func(articleId string, e error) {
		firstErrMu.Lock()
		if firstAnalysisErr == nil {
			firstAnalysisErr = e
		}
		articleErrors[articleId] = classifyAnalysisError(e)
		firstErrMu.Unlock()
	}

	for _, article := range needsAnalysis {
		article := article
		g.Go(func() error {
			if err := gctx.Err(); err != nil {
				return err
			}

			log.WithField("articleId", article.Id).WithField("title", article.Title).Info("Analyzing article for digest")

			// Bake this article's id/title into the task callback so parallel
			// workers emit events that can be distinguished downstream.
			var innerTaskCb func(string, string, int, int, error)
			if onTaskFactory != nil {
				innerTaskCb = onTaskFactory(article.Id, article.Title)
			}

			// Defer the onStart signal until the first task actually begins,
			// that is, after the gateway hands out a slot. This way the
			// "in parallel" count on the UI tracks real LLM in-flight calls,
			// not articles still parked on the gateway semaphore.
			var startedOnce sync.Once
			taskCb := func(taskName, status string, taskIndex, totalTasks int, err error) {
				if status == "started" && taskIndex == 1 {
					startedOnce.Do(func() {
						if onStart != nil {
							onStart(article.Id, article.Title, completed.Load()+1, total)
						}
					})
				}
				if innerTaskCb != nil {
					innerTaskCb(taskName, status, taskIndex, totalTasks, err)
				}
			}

			analysisReq := &protos.AnalyzeArticleWithProviderModelRequest{
				ArticleId:              article.Id,
				ProfileSlug:            profileId,
				VibeScore:              vibeScore,
				Glossary:               glossary,
				StandardSynthesis:      standardSynthesis,
				ComprehensiveSynthesis: comprehensiveSynthesis,
				Provider:               provider,
				ModelName:              model,
			}

			analyze := func(ctx context.Context) error {
				if oneShotAnalysis {
					_, err := s.llms.AnalyzeArticleOneShot(ctx, analysisReq, taskCb)
					return err
				}
				_, err := s.llms.AnalyzeArticleWithProgress(ctx, analysisReq, taskCb)
				return err
			}

			stepCtx, cancel := context.WithTimeout(gctx, 60*time.Minute)
			err := analyze(stepCtx)
			cancel()

			// A subscription usage-limit response is terminal. Record the reason
			// and abort the whole run (returning a non-nil error cancels the
			// errgroup) instead of retrying or moving to the next article, so we
			// stop hitting an already-flagged account.
			if errors.Is(err, llmprovider.ErrUsageLimitReached) {
				log.WithError(err).WithField("articleId", article.Id).Warn("Provider usage limit reached, aborting analysis run")
				captureErr(article.Id, err)
				return err
			}

			if err != nil {
				log.WithError(err).WithField("articleId", article.Id).Warn("Article analysis failed, retrying once")
				retryCtx, retryCancel := context.WithTimeout(gctx, 60*time.Minute)
				err = analyze(retryCtx)
				retryCancel()
				if errors.Is(err, llmprovider.ErrUsageLimitReached) {
					log.WithError(err).WithField("articleId", article.Id).Warn("Provider usage limit reached, aborting analysis run")
					captureErr(article.Id, err)
					return err
				}
			}

			completed.Add(1)

			if err != nil {
				log.WithError(err).WithField("articleId", article.Id).Warn("Failed to analyze article after retry, skipping")
				captureErr(article.Id, err)
				return nil
			}

			analysis, err := store.Db.GetArticleAnalysis(article.Id, profileId)
			if err != nil || analysis == nil {
				log.WithError(err).WithField("articleId", article.Id).Warn("Analysis completed but could not be retrieved")
				captureErr(article.Id, fmt.Errorf("analysis result unavailable after completion"))
				return nil
			}

			analysisMu.Lock()
			analysisMap[article.Id] = analysis
			analysisMu.Unlock()
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		log.WithError(err).WithField("articlesProcessed", completed.Load()).WithField("articlesTotal", total).Info("Article analysis loop cancelled")
		return nil, nil, err
	}

	// Glossary backfill: run the glossary task only for analyses that pre-date glossary being
	// enabled. Failures are non-fatal — the article is still included in the digest, just
	// without jargon terms for the glossary panel.
	if len(needsGlossary) > 0 {
		gg, gctxG := errgroup.WithContext(ctx)
		gg.SetLimit(workerLimit)
		for _, job := range needsGlossary {
			job := job
			gg.Go(func() error {
				err := s.llms.BackfillGlossaryTerms(gctxG, job.article.Id, job.analysis.Id, provider, model, nil)
				if err != nil {
					log.WithError(err).WithField("articleId", job.article.Id).Warn("Glossary backfill failed, skipping")
					return nil
				}
				updated, fetchErr := store.Db.GetArticleAnalysis(job.article.Id, profileId)
				if fetchErr != nil || updated == nil {
					log.WithError(fetchErr).WithField("articleId", job.article.Id).Warn("Failed to reload analysis after glossary backfill")
					return nil
				}
				analysisMu.Lock()
				analysisMap[job.article.Id] = updated
				analysisMu.Unlock()
				return nil
			})
		}
		if err := gg.Wait(); err != nil {
			log.WithError(err).Warn("Glossary backfill loop error")
		}
	}

	// Collect all analyses in article order
	var analyses []models.ArticleAnalysis
	for _, article := range articles {
		if a, ok := analysisMap[article.Id]; ok {
			analyses = append(analyses, *a)
		}
	}

	if len(analyses) == 0 {
		if firstAnalysisErr != nil {
			return nil, articleErrors, fmt.Errorf("no analyses available for any article in the time window: %w", firstAnalysisErr)
		}
		return nil, articleErrors, fmt.Errorf("no analyses available for any article in the time window")
	}

	return analyses, articleErrors, nil
}

// classifyAnalysisError converts a raw analysis error into a short human-readable
// reason shown in the rendered digest next to articles that could not be scored.
func classifyAnalysisError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	lower := strings.ToLower(msg)
	switch {
	case errors.Is(err, context.DeadlineExceeded) || strings.Contains(lower, "deadline exceeded") || strings.Contains(lower, "timeout") || strings.Contains(lower, "timed out"):
		return "Analysis timed out"
	case errors.Is(err, context.Canceled) || strings.Contains(lower, "context canceled"):
		return "Analysis cancelled"
	case errors.Is(err, llmprovider.ErrUsageLimitReached):
		return "Model provider usage limit reached"
	case strings.Contains(lower, "429") || strings.Contains(lower, "rate limit") || strings.Contains(lower, "too many requests"):
		return "Rate limited by model provider"
	case strings.Contains(lower, "401") || strings.Contains(lower, "unauthorized") || strings.Contains(lower, "invalid api key"):
		return "Model provider authentication failed"
	case strings.Contains(lower, "503") || strings.Contains(lower, "502") || strings.Contains(lower, "unavailable") || strings.Contains(lower, "overloaded"):
		return "Model provider unavailable"
	case strings.Contains(lower, "result unavailable"):
		return "Analysis result unavailable after completion"
	default:
		if len(msg) > 120 {
			msg = msg[:120] + "…"
		}
		return "Analysis error: " + msg
	}
}

// duplicateGroupingResult holds the parsed LLM response for duplicate identification
type duplicateGroupingResult struct {
	DuplicateGroups []duplicateGroup `json:"duplicate_groups"`
}

type duplicateGroup struct {
	Event               string   `json:"event"`
	ArticleIds          []string `json:"article_ids"`
	MostComprehensiveId string   `json:"most_comprehensive_id"`
}

// identifyDuplicates sends article key points to the LLM and asks it to identify
// duplicate/overlapping articles. Returns the parsed grouping result and raw response.
func (s *DigestServer) identifyDuplicates(ctx context.Context, analyses []models.ArticleAnalysis, articleMap map[string]models.Article, provider, model string, ded EffectiveEditorial) (*duplicateGroupingResult, string, error) {
	// Build the article summaries for the prompt
	var articleSummaries strings.Builder
	for _, analysis := range analyses {
		title := analysis.ArticleId
		if a, ok := articleMap[analysis.ArticleId]; ok {
			title = a.Title
		}
		articleSummaries.WriteString(fmt.Sprintf("[Article ID: %q | Title: %q]\n", analysis.ArticleId, title))
		if len(analysis.KeyPoints) > 0 {
			articleSummaries.WriteString("Key Points:\n")
			for _, kp := range analysis.KeyPoints {
				articleSummaries.WriteString(fmt.Sprintf("  - %s\n", kp))
			}
		}
		if len(analysis.BriefOverview) > 0 {
			articleSummaries.WriteString("Brief:\n")
			articleSummaries.WriteString(fmt.Sprintf("  %s\n", analysis.BriefOverview))
		}
		articleSummaries.WriteString("\n")
	}

	prompt := fmt.Sprintf(`You are an expert cyber threat intelligence analyst specializing in deduplication of threat reports.

Your task is to identify duplicate and overlapping articles based on their analysis summaries
and group them together.

Two articles belong in the same group when they cover the same specific event — meaning the same
CVE, the same named malware campaign, the same threat actor operation, or the same breach or
incident. A follow-up or update article about the same event also belongs in the group.

Do not group articles that merely share a broad category (e.g. "ransomware", "phishing") without
referring to the same specific incident, and do not group articles that share a threat actor but
describe different, unrelated operations. When in doubt, keep articles separate — a missed
duplicate is less harmful than incorrectly merging unrelated articles.

For each group, identify:
1. A concise description of the shared event (include threat actor, CVE ID, malware name, or
   victim where known)
2. The list of article IDs in the group
3. The ID of the most comprehensive article — the one that covers the event most thoroughly

<start_of_articles>
%s
<end_of_articles>

Respond with valid JSON only — no explanations, markdown, or text outside the JSON structure:

{
  "duplicate_groups": [
    {
      "event": "...",
      "article_ids": ["id_1", "id_2"],
      "most_comprehensive_id": "id_1"
    }
  ]
}

If no duplicates are found, return: {"duplicate_groups": []}`, articleSummaries.String())

	// A profile may inject extra dedupe guidance; appended so the JSON output
	// contract above is always preserved.
	if extra := strings.TrimSpace(ded.Prompts.Dedupe); extra != "" {
		prompt += "\n\nAdditional grouping guidance:\n" + extra
	}

	resolved, err := ResolveLLM(LLMRequest{Provider: provider, ModelName: model})
	if err != nil {
		return nil, "", fmt.Errorf("failed to resolve LLM provider: %w", err)
	}

	log.WithFields(log.Fields{
		"provider":    resolved.ProviderType,
		"model":       resolved.ModelName,
		"promptChars": len(prompt),
	}).Info("Sending duplicate identification prompt to LLM")

	stepCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	rawResponse, err := s.gw.Generate(stepCtx, resolved.Provider, prompt, llmgateway.WithLabel("digest:dedupe"), llmgateway.WithModelInfo(resolved.ProviderType, resolved.ModelName))
	if err != nil {
		return nil, "", fmt.Errorf("LLM call for duplicate identification failed: %w", err)
	}

	log.WithField("responseLen", len(rawResponse)).Debug("Received grouping response from LLM")

	// Parse the response
	cleaned := llmutil.CleanLLMResponse(rawResponse)
	var result duplicateGroupingResult
	if err := json.Unmarshal([]byte(cleaned), &result); err != nil {
		// Try to extract JSON from response
		potentialJSON := llmutil.ExtractJSON(cleaned)
		if err := json.Unmarshal([]byte(potentialJSON), &result); err != nil {
			log.WithError(err).Warn("Failed to parse grouping response, proceeding with no duplicates")
			return &duplicateGroupingResult{}, rawResponse, nil
		}
	}

	log.WithField("groups", len(result.DuplicateGroups)).Info("Duplicate groups identified")

	return &result, rawResponse, nil
}

type digestSummaryResponse struct {
	Title   string `json:"title"`
	Summary string `json:"summary"`
}

// generateDigestSummary creates a title (always) and optional executive summary for the digest.
// Returns (title, summary, providerType, modelName, error).
func (s *DigestServer) generateDigestSummary(ctx context.Context, analyses []models.ArticleAnalysis, articleMap map[string]models.Article, windowStart, windowEnd time.Time, provider, model string, withSummary bool, ded EffectiveEditorial) (string, string, string, string, error) {
	prompt := buildDigestSummaryPrompt(analyses, articleMap, windowStart, windowEnd, withSummary, ded)

	summaryTemp := 0.5
	resolved, err := ResolveLLM(LLMRequest{Provider: provider, ModelName: model, Temperature: &summaryTemp})
	if err != nil {
		return "", "", "", "", fmt.Errorf("failed to resolve LLM provider: %w", err)
	}

	log.WithFields(log.Fields{
		"provider":     resolved.ProviderType,
		"model":        resolved.ModelName,
		"articleCount": len(analyses),
		"withSummary":  withSummary,
	}).Info("Generating digest title/summary")

	stepCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	rawResponse, err := s.gw.Generate(stepCtx, resolved.Provider, prompt, llmgateway.WithLabel("digest:summary"), llmgateway.WithModelInfo(resolved.ProviderType, resolved.ModelName))
	if err != nil {
		return "", "", "", "", fmt.Errorf("LLM call for digest summary failed: %w", err)
	}

	cleaned := llmutil.CleanLLMResponse(rawResponse)
	var parsed digestSummaryResponse
	if err := json.Unmarshal([]byte(cleaned), &parsed); err != nil {
		potentialJSON := llmutil.ExtractJSON(cleaned)
		if err := json.Unmarshal([]byte(potentialJSON), &parsed); err != nil {
			log.WithError(err).Warn("Failed to parse digest summary JSON, storing raw response as summary")
			return "", strings.TrimSpace(rawResponse), resolved.ProviderType, resolved.ModelName, nil
		}
	}

	log.WithFields(log.Fields{
		"titleLen":   len(parsed.Title),
		"summaryLen": len(parsed.Summary),
	}).Info("Digest summary generated")

	return strings.TrimSpace(parsed.Title), strings.TrimSpace(parsed.Summary), resolved.ProviderType, resolved.ModelName, nil
}

// populateGlossary feeds this digest's extracted terms (jargon/concepts from the analyses
// plus named entities from tags) into the persistent global glossary and records which
// entries the digest references. Terms already defined in the glossary are linked without
// any LLM call; only genuinely new terms are defined, in a single batched call. Definitions
// are therefore generated exactly once per term — never re-generated and discarded. Manual
// overrides are never overwritten.
func (s *DigestServer) populateGlossary(ctx context.Context, digestId string, analyses []models.ArticleAnalysis, articleMap map[string]models.Article, provider, model string) (int, error) {
	stepCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	// candidate is a term this digest references, keyed by NormalizeGlossaryKey.
	type candidate struct {
		kind        models.GlossaryKind
		category    string
		tagId       string
		displayTerm string
	}
	candidates := make(map[string]*candidate)

	// Jargon/concepts extracted per-article (term + type + context; no definition).
	for i := range analyses {
		for _, term := range analyses[i].GlossaryTerms {
			key := models.NormalizeGlossaryKey(term.Term)
			if key == "" {
				continue
			}
			c, ok := candidates[key]
			if !ok {
				c = &candidate{kind: models.GlossaryKindJargon, displayTerm: strings.TrimSpace(term.Term)}
				candidates[key] = c
			}
			if c.category == "" || c.category == models.GlossaryCategoryOther {
				c.category = models.NormalizeGlossaryCategory(term.Type)
			}
		}
	}

	// Named entities from tags take precedence — they carry a TagId for highlighting/linkage.
	for _, article := range articleMap {
		for _, tag := range article.Tags {
			slug := strings.TrimSpace(tag.Name)
			if slug == "" {
				continue
			}
			key := models.NormalizeGlossaryKey(slug)
			if key == "" {
				continue
			}
			c, ok := candidates[key]
			if !ok {
				c = &candidate{displayTerm: slug}
				candidates[key] = c
			}
			c.kind = models.GlossaryKindEntity
			c.tagId = slug
		}
	}

	if len(candidates) == 0 {
		return 0, nil
	}

	// entryIds collects the canonical glossary entry ids this digest references (deduped).
	entryIds := make(map[string]struct{})
	// entityMeta records display term + category for entity-kind entry keys, so per-article
	// context can be generated and attached for tag-derived terms (which bypass extraction).
	entityMeta := make(map[string]glossaryTermCtx)

	keys := make([]string, 0, len(candidates))
	for key := range candidates {
		keys = append(keys, key)
	}
	existing, err := store.Db.GetGlossaryEntriesByKeys(keys)
	if err != nil {
		return 0, fmt.Errorf("failed to load existing glossary entries: %w", err)
	}

	// Reference terms that already have a definition; queue the rest for one batched LLM call.
	var undefined []string
	for key, c := range candidates {
		if e, ok := existing[key]; ok && e.EffectiveDefinition() != "" {
			entryIds[e.Id] = struct{}{}
			if e.Kind == models.GlossaryKindEntity {
				entityMeta[key] = glossaryTermCtx{Term: e.Term, Category: e.Category}
			}
			continue
		}
		undefined = append(undefined, c.displayTerm)
	}

	if len(undefined) > 0 {
		defs, err := s.defineEntities(stepCtx, undefined, provider, model)
		if err != nil {
			log.WithError(err).Warn("Failed to generate glossary definitions")
		}
		for key, ed := range defs {
			c, ok := candidates[key]
			if !ok {
				continue
			}
			// Prefer the model's returned type; fall back to the extracted type.
			category := models.NormalizeGlossaryCategory(ed.Type)
			if category == models.GlossaryCategoryOther && c.category != "" {
				category = c.category
			}
			source := "tag"
			if c.kind == models.GlossaryKindJargon {
				source = "analysis"
			}
			entry := &models.GlossaryEntry{
				NormalizedKey:   key,
				Term:            c.displayTerm,
				Kind:            c.kind,
				Category:        category,
				Difficulty:      models.NormalizeGlossaryDifficulty(ed.Difficulty),
				Definition:      ed.Def,
				TagId:           c.tagId,
				DefinitionModel: model,
				Source:          source,
			}
			if err := store.Db.UpsertGlossaryEntry(entry); err != nil {
				log.WithError(err).WithField("term", c.displayTerm).Warn("Failed to upsert glossary entry")
				continue
			}
			entryIds[entry.Id] = struct{}{}
			if c.kind == models.GlossaryKindEntity {
				entityMeta[key] = glossaryTermCtx{Term: c.displayTerm, Category: category}
			}
		}
	}

	if len(entryIds) == 0 {
		return 0, nil
	}
	rows := make([]models.DigestGlossary, 0, len(entryIds))
	for id := range entryIds {
		rows = append(rows, models.DigestGlossary{DigestId: digestId, EntryId: id})
	}
	if err := store.Db.StoreDigestGlossaryBatch(rows); err != nil {
		return 0, fmt.Errorf("failed to store digest glossary links: %w", err)
	}

	// Backfill per-article "in this article" context for matched entity terms (which bypass
	// the extraction task), even when their global definition is cached. Warn-only — never
	// fails digest generation.
	s.populateArticleContexts(stepCtx, analyses, articleMap, entityMeta, provider, model)

	log.WithFields(log.Fields{"digestId": digestId, "entries": len(rows)}).Info("Glossary populated")
	return len(rows), nil
}

// populateArticleContexts generates and persists a per-article "in this article" context
// sentence for each tag-derived entity term that is part of the digest glossary but does not
// already carry context for that article. Entity terms come from tags and never pass through
// the per-article extraction task, so without this they show only a global definition. One
// batched LLM call per article (bounded by the gateway's concurrency cap); all failures are
// logged and swallowed so digest generation is never affected.
func (s *DigestServer) populateArticleContexts(ctx context.Context, analyses []models.ArticleAnalysis, articleMap map[string]models.Article, entityMeta map[string]glossaryTermCtx, provider, model string) {
	if len(entityMeta) == 0 {
		return
	}

	idxByArticle := make(map[string]int, len(analyses))
	for i := range analyses {
		idxByArticle[analyses[i].ArticleId] = i
	}

	type job struct {
		analysisIdx int
		content     string
		terms       []string
	}
	var jobs []job
	for artId, art := range articleMap {
		ai, ok := idxByArticle[artId]
		if !ok {
			continue
		}
		existingCtx := make(map[string]bool)
		for _, t := range analyses[ai].GlossaryTerms {
			if strings.TrimSpace(t.Context) != "" {
				existingCtx[models.NormalizeGlossaryKey(t.Term)] = true
			}
		}
		var terms []string
		for _, tag := range art.Tags {
			key := models.NormalizeGlossaryKey(tag.Name)
			if key == "" || existingCtx[key] {
				continue
			}
			if _, isEntry := entityMeta[key]; !isEntry {
				continue
			}
			terms = append(terms, strings.TrimSpace(tag.Name))
		}
		if len(terms) > 0 {
			jobs = append(jobs, job{analysisIdx: ai, content: art.Content, terms: terms})
		}
	}
	if len(jobs) == 0 {
		return
	}

	limit := s.gw.MaxConcurrent()
	if limit < 1 {
		limit = 1
	}
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(limit)
	var mu sync.Mutex
	results := make(map[int]map[string]string, len(jobs))
	for _, j := range jobs {
		j := j
		g.Go(func() error {
			m, err := s.articleTermContexts(gctx, j.content, j.terms, provider, model)
			if err != nil {
				log.WithError(err).Warn("Failed to generate article term contexts")
				return nil
			}
			if len(m) == 0 {
				return nil
			}
			mu.Lock()
			results[j.analysisIdx] = m
			mu.Unlock()
			return nil
		})
	}
	_ = g.Wait()

	for ai, ctxByKey := range results {
		add := make(map[string]glossaryTermCtx, len(ctxByKey))
		for key, c := range ctxByKey {
			meta := entityMeta[key]
			add[key] = glossaryTermCtx{Term: meta.Term, Category: meta.Category, Context: c}
		}
		merged := mergeArticleContexts(analyses[ai].GlossaryTerms, add)
		if len(merged) == len(analyses[ai].GlossaryTerms) {
			continue // nothing new
		}
		analyses[ai].GlossaryTerms = merged
		if analyses[ai].Id == "" {
			continue
		}
		b, err := json.Marshal(merged)
		if err != nil {
			log.WithError(err).Warn("Failed to marshal merged glossary terms")
			continue
		}
		if err := store.Db.UpdateArticleAnalysisGlossaryTerms(analyses[ai].Id, string(b)); err != nil {
			log.WithError(err).WithField("analysisId", analyses[ai].Id).Warn("Failed to persist article term contexts")
		}
	}
}

// entityDefinition is a generated definition plus its semantic type and difficulty for a
// named entity.
type entityDefinition struct {
	Def        string
	Type       string
	Difficulty string
}

// defineEntities asks the model for a one-line plain-language definition and a type for each
// term — a named entity or a security concept — returned keyed by NormalizedGlossaryKey.
// Unknown terms (empty definitions) are dropped so they can be retried on a future digest.
func (s *DigestServer) defineEntities(ctx context.Context, entities []string, provider, model string) (map[string]entityDefinition, error) {
	if len(entities) == 0 {
		return map[string]entityDefinition{}, nil
	}

	// Address each term by a stable synthetic id (t1, t2, …) so matching the response
	// back to the candidate does not depend on the model echoing a normalization-stable
	// string. Models routinely reword or expand a term (e.g. "mcp" → "Model Context
	// Protocol", "mitre-attack" → "MITRE ATT&CK"), which would otherwise break the key
	// match and silently discard a perfectly good definition.
	idToTerm := make(map[string]string, len(entities))
	var list strings.Builder
	for i, e := range entities {
		id := fmt.Sprintf("t%d", i+1)
		idToTerm[id] = e
		list.WriteString(id)
		list.WriteString("\t")
		list.WriteString(e)
		list.WriteString("\n")
	}

	prompt := fmt.Sprintf(`You are explaining cybersecurity terms to a complete beginner. For each term
below — a named entity (threat actor, malware family, tool, CVE, vendor, organization, or
country) or a security concept/technique/protocol relevant to a security story — give one
plain-language sentence a newcomer can understand, classify its type, and rate how much help
a reader needs with it. If you do not recognize a term, or cannot define it confidently,
return an empty definition for it — do not guess.

type must be ONE of: threat-actor, malware, tool, technique, vulnerability, protocol, concept,
organization, product, other.

difficulty must be ONE of — rate by how much help a reader needs with the term, not by how
sophisticated it sounds:
  - beginner: a common, widely-known term most general readers have already heard outside security
    (e.g. phishing, password, VPN, malware, firewall). Test: would a non-technical reader recognise
    it? Do not rate a term higher just because it appears in a technical article.
  - intermediate: a security-specific term a newcomer to the field would need explained but any
    practitioner knows cold (e.g. C2, lateral movement, RAT, privilege escalation, EDR). This is
    the default bucket for standard tradecraft and well-known protocols, tools, and techniques.
  - advanced: a niche or obscure term even many practitioners would not know offhand — a specific
    named malware family, an uncommon technique, a narrow CVE, a less-known threat actor or tool
    (e.g. a specific loader family, an unusual evasion technique). Do not rate generic tradecraft as
    advanced just because it is technical.

Each line below is "<id>\t<term>". Use the id (e.g. t1) as the JSON key for that term's
definition — do not use the term text as the key.

<start_of_entities>
%s<end_of_entities>

Respond with valid JSON only — no explanations, markdown, or text outside the JSON structure:

{
  "definitions": {
    "<id>": {"definition": "<one plain-language sentence, or empty string if unknown>", "type": "<category>", "difficulty": "<difficulty>"}
  }
}`, list.String())

	resolved, err := ResolveLLM(LLMRequest{Provider: provider, ModelName: model})
	if err != nil {
		return nil, fmt.Errorf("failed to resolve LLM provider: %w", err)
	}

	rawResponse, err := s.gw.Generate(ctx, resolved.Provider, prompt, llmgateway.WithLabel("digest:glossary-entities"), llmgateway.WithModelInfo(resolved.ProviderType, resolved.ModelName))
	if err != nil {
		return nil, fmt.Errorf("LLM call for entity definitions failed: %w", err)
	}

	cleaned := llmutil.CleanLLMResponse(rawResponse)
	var parsed struct {
		Definitions map[string]struct {
			Definition string `json:"definition"`
			Type       string `json:"type"`
			Difficulty string `json:"difficulty"`
		} `json:"definitions"`
	}
	if err := json.Unmarshal([]byte(cleaned), &parsed); err != nil {
		if err := json.Unmarshal([]byte(llmutil.ExtractJSON(cleaned)), &parsed); err != nil {
			return nil, fmt.Errorf("failed to parse entity definitions: %w", err)
		}
	}

	raw := make(map[string]entityDefinition, len(parsed.Definitions))
	for id, v := range parsed.Definitions {
		raw[id] = entityDefinition{Def: v.Definition, Type: v.Type, Difficulty: v.Difficulty}
	}
	return entityDefinitionsFromResult(remapEntityDefsByID(raw, idToTerm)), nil
}

// remapEntityDefsByID translates a model response keyed by synthetic ids (t1, t2, …) back
// to the original input terms those ids were assigned to. This keeps the round-trip
// echo-independent: the model may reword or expand a term freely without breaking the link
// to its tag candidate. Ids the model invents or returns unknown are dropped.
func remapEntityDefsByID(byID map[string]entityDefinition, idToTerm map[string]string) map[string]entityDefinition {
	out := make(map[string]entityDefinition, len(byID))
	for id, v := range byID {
		term, ok := idToTerm[strings.ToLower(strings.TrimSpace(id))]
		if !ok {
			continue
		}
		out[term] = v
	}
	return out
}

// entityDefinitionsFromResult normalizes entity keys, coerces types, and drops empty definitions.
func entityDefinitionsFromResult(raw map[string]entityDefinition) map[string]entityDefinition {
	out := make(map[string]entityDefinition)
	for entity, v := range raw {
		def := strings.TrimSpace(v.Def)
		if def == "" {
			continue
		}
		key := models.NormalizeGlossaryKey(entity)
		if key == "" {
			continue
		}
		out[key] = entityDefinition{Def: def, Type: models.NormalizeGlossaryCategory(v.Type), Difficulty: models.NormalizeGlossaryDifficulty(v.Difficulty)}
	}
	return out
}

// articleTermContexts asks the model for a one-sentence "why this term matters in THIS
// article" explanation for each given term, grounded only in the article text. Returned
// keyed by NormalizeGlossaryKey; terms the article does not actually discuss (empty answers)
// are dropped. Used to give tag-derived entity terms — which never pass through the
// per-article extraction task — the same contextual line jargon terms already get.
func (s *DigestServer) articleTermContexts(ctx context.Context, articleContent string, terms []string, provider, model string) (map[string]string, error) {
	if len(terms) == 0 {
		return map[string]string{}, nil
	}

	// Reuse the analysis pipeline's HTML→markdown conversion for prompt parity.
	stripped := dataImageRe.ReplaceAllString(articleContent, "")
	content := stripped
	if md, err := htmltomarkdown.ConvertString(stripped); err == nil {
		content = md
	}

	var list strings.Builder
	for _, t := range terms {
		list.WriteString(t)
		list.WriteString("\n")
	}

	prompt := fmt.Sprintf(`You are explaining a cybersecurity article to someone brand new to the field. Below is the
article text, followed by a list of terms that appear in it (threat actors, malware, tools,
CVEs, vendors, techniques, or concepts). For EACH term, write a single plain-language sentence
explaining why that term matters in THIS article specifically — what role it plays in the
events described. Use ONLY information present in the article; do not infer or add outside
context. If a term is not actually discussed in the article, or you cannot say what role it
plays from the text alone, return an empty string for it — do not guess.

Echo back each term exactly as given (the key).

<start_of_article>
%s
<end_of_article>

<start_of_terms>
%s<end_of_terms>

Respond with valid JSON only — no explanations, markdown, or text outside the JSON structure:

{
  "contexts": {
    "<term>": "<one plain sentence on its role in this article, or empty string if not discussed>"
  }
}`, content, list.String())

	resolved, err := ResolveLLM(LLMRequest{Provider: provider, ModelName: model})
	if err != nil {
		return nil, fmt.Errorf("failed to resolve LLM provider: %w", err)
	}

	rawResponse, err := s.gw.Generate(ctx, resolved.Provider, prompt, llmgateway.WithLabel("digest:glossary-context"), llmgateway.WithModelInfo(resolved.ProviderType, resolved.ModelName))
	if err != nil {
		return nil, fmt.Errorf("LLM call for article term contexts failed: %w", err)
	}

	cleaned := llmutil.CleanLLMResponse(rawResponse)
	var parsed struct {
		Contexts map[string]string `json:"contexts"`
	}
	if err := json.Unmarshal([]byte(cleaned), &parsed); err != nil {
		if err := json.Unmarshal([]byte(llmutil.ExtractJSON(cleaned)), &parsed); err != nil {
			return nil, fmt.Errorf("failed to parse article term contexts: %w", err)
		}
	}

	return articleTermContextsFromResult(parsed.Contexts), nil
}

// articleTermContextsFromResult normalizes term keys and drops empty contexts.
func articleTermContextsFromResult(raw map[string]string) map[string]string {
	out := make(map[string]string)
	for term, v := range raw {
		ctx := strings.TrimSpace(v)
		if ctx == "" {
			continue
		}
		key := models.NormalizeGlossaryKey(term)
		if key == "" {
			continue
		}
		out[key] = ctx
	}
	return out
}

// glossaryTermCtx carries the display term, category, and context for a generated
// per-article entity context, ready to be merged into ArticleAnalysis.GlossaryTerms.
type glossaryTermCtx struct {
	Term     string
	Category string
	Context  string
}

// mergeArticleContexts appends new per-article entity contexts to an article's existing
// glossary terms, skipping any term whose normalized key is already present so repeated
// digests over the same article never accumulate duplicate rows. The add map is keyed by
// NormalizeGlossaryKey. Returns the merged slice (input is not mutated).
func mergeArticleContexts(existing []models.GlossaryTerm, add map[string]glossaryTermCtx) []models.GlossaryTerm {
	seen := make(map[string]bool, len(existing))
	for _, t := range existing {
		seen[models.NormalizeGlossaryKey(t.Term)] = true
	}
	merged := existing
	for key, v := range add {
		if key == "" || seen[key] {
			continue
		}
		ctx := strings.TrimSpace(v.Context)
		if ctx == "" {
			continue
		}
		seen[key] = true
		merged = append(merged, models.GlossaryTerm{
			Term:    v.Term,
			Type:    models.NormalizeGlossaryCategory(v.Category),
			Context: ctx,
		})
	}
	return merged
}

func buildDigestSummaryPrompt(analyses []models.ArticleAnalysis, articleMap map[string]models.Article, windowStart, windowEnd time.Time, withSummary bool, ded EffectiveEditorial) string {
	var articlesList strings.Builder
	for i, analysis := range analyses {
		article, ok := articleMap[analysis.ArticleId]
		if !ok {
			log.WithField("articleId", analysis.ArticleId).Warn("Article not found in batch map, skipping for summary")
			continue
		}

		articlesList.WriteString(fmt.Sprintf("%d. **%s**\n", i+1, article.Title))
		articlesList.WriteString(fmt.Sprintf("   Source: %s\n", article.Link))
		if len(analysis.KeyPoints) > 0 {
			articlesList.WriteString("   Key Points:\n")
			for _, kp := range analysis.KeyPoints {
				articlesList.WriteString(fmt.Sprintf("   - %s\n", kp))
			}
		}
		articlesList.WriteString("\n")
	}

	windowDuration := windowEnd.Sub(windowStart)

	// Editorial guidance injected from the profile (writing style / audience /
	// optional extra summary guidance). Each part is only emitted when set, so a
	// profile that customizes nothing produces the historical prompt verbatim.
	var styleBlock string
	if ws := ded.WritingStyle; ws != "" {
		styleBlock += fmt.Sprintf("\nWriting style:\n%s\n", ws)
	}
	if aud := ded.Audience; aud != "" {
		styleBlock += fmt.Sprintf("\nTarget audience:\n%s\n", aud)
	}
	if extra := ded.Prompts.DigestSummary; strings.TrimSpace(extra) != "" {
		styleBlock += fmt.Sprintf("\nAdditional guidance:\n%s\n", extra)
	}

	titleInstruction := `<concise title of 5-20 words that captures the dominant theme of this digest. Be factual and specific so a reader immediately knows what's inside: name the actual CVEs, products, threat actors, campaigns, or incidents driving the digest. Avoid vague, abstract framing like 'AI-Accelerated Vulnerability Discovery and Advanced Espionage Campaigns Define Current Threat Landscape' when the underlying articles cover concrete, nameable items>`

	if !withSummary {
		return fmt.Sprintf(`You are a senior cyber threat intelligence analyst authoring a news digest for a technical security audience.

Below is a list of articles with their key points, selected for the digest coverage window shown below.
%s
Digest coverage window:
- Start: %s
- End: %s
- Duration: %s

<articles>
%s
</articles>

Respond with a JSON object in this exact format (no markdown fences, no extra text):
{
  "title": "%s"
}
`, styleBlock, windowStart.UTC().Format(time.RFC3339), windowEnd.UTC().Format(time.RFC3339), windowDuration.String(), articlesList.String(), titleInstruction)
	}

	return fmt.Sprintf(`You are a senior cyber threat intelligence analyst authoring a news digest for a technical security audience (threat hunters, detection engineers, SOC analysts, and incident responders).

Below is a list of articles with their key points. These articles were selected for the digest coverage window shown below. Use this window to frame the digest, but do not imply the reported events occurred exactly within the window unless the article details say so.

The digest should:
1. Provide an executive overview of the main themes and trends
2. Highlight the most critical threats or incidents
3. Group related topics together
4. Be written in a professional, clear, and engaging style
5. Be suitable for both technical and executive audiences
%s
Digest coverage window:
- Start: %s
- End: %s
- Duration: %s

<articles>
%s
</articles>

Respond with a JSON object in this exact format (no markdown fences, no extra text):
{
  "title": "%s",
  "summary": "<digest summary written in markdown. Begin with 2-3 introductory sentences (no heading) giving an overall snapshot of the threat landscape covered in this digest. Then organize the remaining content into thematic sections using ## markdown headings. Each section should group multiple related articles under a shared threat theme, event type, or actor cluster — do not create a section for a single article unless it stands alone with no thematic peers. Aim for the fewest sections that still meaningfully separate distinct areas; prefer broader groupings over narrow ones. Section titles must be concrete and specific (e.g. '## Ransomware Campaigns', '## Critical Infrastructure Attacks', '## Espionage & APT Activity'), not generic labels. Within each section, write 2-3 markdown paragraphs (blank line between each), 2-4 sentences per paragraph. Total length approximately 300-600 words.>"
}

The summary must be purely factual and descriptive. Do NOT include sections on strategic recommendations, action items, mitigation advice, or "what you should do." This digest is for intelligence reporting only: present threats, incidents, and trends as reported, without prescribing any response.
`, styleBlock, windowStart.UTC().Format(time.RFC3339), windowEnd.UTC().Format(time.RFC3339), windowDuration.String(), articlesList.String(), titleInstruction)
}

// buildDigestAnalyses creates DigestAnalysis entries from analyses and grouping results
func (s *DigestServer) buildDigestAnalyses(digestId string, analyses []models.ArticleAnalysis, grouping *duplicateGroupingResult) []models.DigestAnalysis {
	// Build lookup: articleId -> duplicate group info
	type groupInfo struct {
		event               string
		isMostComprehensive bool
	}
	articleGroupMap := make(map[string]groupInfo)

	if grouping != nil {
		for _, group := range grouping.DuplicateGroups {
			for _, articleId := range group.ArticleIds {
				articleGroupMap[articleId] = groupInfo{
					event:               group.Event,
					isMostComprehensive: articleId == group.MostComprehensiveId,
				}
			}
		}
	}

	var entries []models.DigestAnalysis
	for _, analysis := range analyses {
		entry := models.DigestAnalysis{
			DigestId:   digestId,
			AnalysisId: analysis.Id,
			ArticleId:  analysis.ArticleId,
		}

		if info, ok := articleGroupMap[analysis.ArticleId]; ok {
			entry.DuplicateGroup = info.event
			entry.IsMostComprehensive = info.isMostComprehensive
		}

		entries = append(entries, entry)
	}

	return entries
}

// generateDigestId generates a unique Id for a digest
func generateDigestId(t time.Time) string {
	timestamp := t.Format(time.RFC3339)
	hash := md5.Sum([]byte(timestamp))
	return fmt.Sprintf("digest-%s", fmt.Sprintf("%x", hash)[:8])
}

// generateRunId generates a unique Id for an LLM monitoring run. Uses
// nanosecond precision so back-to-back runs don't collide.
func generateRunId(t time.Time) string {
	hash := md5.Sum([]byte(t.Format(time.RFC3339Nano)))
	return fmt.Sprintf("run-%s", fmt.Sprintf("%x", hash)[:12])
}
