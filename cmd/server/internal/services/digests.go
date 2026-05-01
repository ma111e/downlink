package services

import (
	"context"
	"crypto/md5"
	"downlink/cmd/server/internal/config"
	"downlink/cmd/server/internal/notification"
	"downlink/cmd/server/internal/store"
	"downlink/pkg/llmgateway"
	"downlink/pkg/llmutil"
	"downlink/pkg/mappers"
	"downlink/pkg/models"
	"downlink/pkg/protos"
	"encoding/json"
	"errors"
	"fmt"
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
	digests, err := store.Db.ListDigests(int(req.Limit))
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

// GenerateDigest generates a new digest, streaming progress events throughout the process
func (s *DigestServer) GenerateDigest(req *protos.GenerateDigestRequest, rawStream protos.DigestService_GenerateDigestServer) error {
	// Parallel analysis workers race on stream.Send; serialize through safeStream.
	stream := &safeStream{stream: rawStream}

	ctx := rawStream.Context()
	if req.GetTest() {
		log.Info("Sending stored digest to notification channels for test")
		return s.sendNotificationTestDigest(req, stream)
	}

	log.Info("Generating digest from recent articles")

	windowStart := req.StartTime.AsTime()
	windowEnd := time.Now()
	if req.EndTime != nil {
		windowEnd = req.EndTime.AsTime()
	}
	windowDuration := windowEnd.Sub(windowStart)

	// Fetch articles
	sendProgress(stream, "fetch", "fetching articles...", 0, 0)
	articles, err := s.getRecentArticles(windowStart, &windowEnd, req.ExcludeDigested)
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
	if req.SkipAnalysis {
		log.Info("Skipping article analysis (skip_analysis requested)")
		articleIds := make([]string, len(articles))
		for i, a := range articles {
			articleIds[i] = a.Id
		}
		analysisMap, err := store.Db.GetArticleAnalysesBatch(articleIds)
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
		analyses, err = s.ensureArticlesAnalyzed(ctx, articles, req.OneShotAnalysis, onAnalysisStart, onTaskProgress)
		if err != nil {
			if cancelled(stream) {
				return ctx.Err()
			}
			_ = stream.Send(&protos.DigestProgressEvent{Stage: "error", Error: fmt.Sprintf("failed to ensure articles are analyzed: %v", err)})
			return fmt.Errorf("failed to ensure articles are analyzed: %w", err)
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
		groupingResult, rawResponse, err = s.identifyDuplicates(ctx, analyses, articleMap)
		if err != nil {
			_ = stream.Send(&protos.DigestProgressEvent{Stage: "error", Error: fmt.Sprintf("failed to identify duplicates: %v", err)})
			return fmt.Errorf("failed to identify duplicates: %w", err)
		}
		sendProgress(stream, "dedupe", fmt.Sprintf("found %d duplicate groups", len(groupingResult.DuplicateGroups)), 0, 0)
	}

	// Step 3: Generate digest summary presentation
	var digestTitle, digestSummary string
	var summaryProviderType, summaryModelName string
	if req.SkipSummary {
		log.Info("Skipping digest summary generation (skip_summary requested)")
	} else {
		sendProgress(stream, "summarize", "generating digest summary...", 0, 0)
		digestTitle, digestSummary, summaryProviderType, summaryModelName, err = s.generateDigestSummary(ctx, analyses, articleMap, windowStart, windowEnd)
		if err != nil {
			log.WithError(err).Warn("Failed to generate digest summary, continuing without it")
		} else {
			sendProgress(stream, "summarize", "digest summary generated", 0, 0)
		}
	}

	// Step 4: Build the digest object
	sendProgress(stream, "store", "storing digest...", 0, 0)
	now := time.Now()
	articleLen := len(articles)
	digest := models.Digest{
		Id:                  generateDigestId(now),
		CreatedAt:           now,
		ArticleCount:        &articleLen,
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

	// Store DigestProviderResult so the HTML notification can render provider syntheses
	providerType := summaryProviderType
	modelName := summaryModelName
	if providerType == "" {
		if ap, apErr := findEnabledProviderByName(config.Config.Analysis.Provider); apErr == nil {
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

	// Store digest-article associations in a single batch operation
	articleIds := make([]string, len(articles))
	for i, article := range articles {
		articleIds[i] = article.Id
	}
	if err = store.Db.StoreDigestArticlesBatch(digest.Id, articleIds); err != nil {
		log.WithError(err).WithField("digestId", digest.Id).Warn("Failed to batch store digest-article associations")
	}

	log.WithFields(log.Fields{
		"id":              digest.Id,
		"articleCount":    articleLen,
		"analysisCount":   len(digestAnalyses),
		"duplicateGroups": len(groupingResult.DuplicateGroups),
	}).Info("Digest generated successfully")

	// Send notifications if configured. Reload digest once so Articles,
	// ProviderResults, and DigestAnalyses are populated for renderers.
	if fullDigest, err := store.Db.GetDigest(digest.Id); err != nil {
		log.WithError(err).Warn("Failed to reload digest for notifications, skipping all")
	} else if _, err := sendConfiguredDigestNotifications(stream, fullDigest, req.GetTheme(), false); err != nil {
		log.WithError(err).Warn("Failed to send one or more digest notifications")
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

	attempts, err := sendConfiguredDigestNotifications(stream, digest, req.GetTheme(), true)
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

	digests, err := store.Db.ListDigests(0)
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

func sendConfiguredDigestNotifications(stream *safeStream, digest models.Digest, theme string, failOnError bool) (int, error) {
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

		htmlBytes, err := notification.RenderDigestHTML(digest, theme)
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

	ghPagesEnabled := config.Config.Notifications.GitHubPages.Enabled && config.Config.Notifications.GitHubPages.RepoURL != ""
	if ghPagesEnabled {
		attempts++
		sendProgress(stream, "notify", "publishing digest to GitHub Pages...", 0, 0)
		ghCfg := config.Config.Notifications.GitHubPages
		if ghCfg.Token == "" {
			ghCfg.Token = os.Getenv("DOWNLINK_GH_PAGES_TOKEN")
		}
		if ghCfg.Token == "" {
			err := errors.New("GitHub Pages enabled but no token configured (set token in config or DOWNLINK_GH_PAGES_TOKEN env)")
			log.Warn(err)
			errs = append(errs, err)
		} else {
			publisher := notification.NewGitHubPagesPublisher(ghCfg)
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
func (s *DigestServer) getRecentArticles(after time.Time, before *time.Time, excludeDigested bool) ([]models.Article, error) {
	filter := models.ArticleFilter{
		StartDate:       &after,
		EndDate:         before,
		ExcludeDigested: excludeDigested,
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
// onStart is called when an article's analysis begins — the CLI uses this to
// register a new row and reserve screen space for concurrent work.
// onTaskFactory returns a per-article task-progress callback so the emitted
// events carry the article's id/title (parallel articles' events would
// otherwise be indistinguishable downstream).
func (s *DigestServer) ensureArticlesAnalyzed(
	ctx context.Context,
	articles []models.Article,
	oneShotAnalysis bool,
	onStart func(articleId, articleTitle string, current, total uint32),
	onTaskFactory func(articleId, articleTitle string) func(taskName, status string, taskIndex, totalTasks int, err error),
) ([]models.ArticleAnalysis, error) {
	// Batch-fetch existing analyses for all articles in one query
	articleIds := make([]string, len(articles))
	for i, a := range articles {
		articleIds[i] = a.Id
	}
	analysisMap, err := store.Db.GetArticleAnalysesBatch(articleIds)
	if err != nil {
		log.WithError(err).Warn("Failed to batch fetch article analyses, falling back to per-article lookup")
		analysisMap = make(map[string]*models.ArticleAnalysis)
	}

	var needsAnalysis []models.Article
	for _, article := range articles {
		if analysisMap[article.Id] == nil {
			needsAnalysis = append(needsAnalysis, article)
		}
	}

	if len(needsAnalysis) > 0 {
		log.WithField("count", len(needsAnalysis)).Info("Triggering analysis for unanalyzed articles")
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
		total            = uint32(len(needsAnalysis))
	)

	captureErr := func(e error) {
		firstErrMu.Lock()
		if firstAnalysisErr == nil {
			firstAnalysisErr = e
		}
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

			// Defer the onStart signal until the first task actually begins
			// — that is, after the gateway hands out a slot. This way the
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

			stepCtx, cancel := context.WithTimeout(gctx, 60*time.Minute)
			analysisReq := &protos.AnalyzeArticleWithProviderModelRequest{
				ArticleId:      article.Id,
				SkipCategorize: true,
			}
			var err error
			if oneShotAnalysis {
				_, err = s.llms.AnalyzeArticleOneShot(stepCtx, analysisReq, taskCb)
			} else {
				_, err = s.llms.AnalyzeArticleWithProgress(stepCtx, analysisReq, taskCb)
			}
			cancel()

			completed.Add(1)

			if err != nil {
				log.WithError(err).WithField("articleId", article.Id).Warn("Failed to analyze article, skipping")
				captureErr(err)
				return nil
			}

			analysis, err := store.Db.GetArticleAnalysis(article.Id)
			if err != nil || analysis == nil {
				log.WithError(err).WithField("articleId", article.Id).Warn("Analysis completed but could not be retrieved")
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
		return nil, err
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
			return nil, fmt.Errorf("no analyses available for any article in the time window: %w", firstAnalysisErr)
		}
		return nil, fmt.Errorf("no analyses available for any article in the time window")
	}

	return analyses, nil
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
func (s *DigestServer) identifyDuplicates(ctx context.Context, analyses []models.ArticleAnalysis, articleMap map[string]models.Article) (*duplicateGroupingResult, string, error) {
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

	resolved, err := ResolveLLM(LLMRequest{})
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

	rawResponse, err := s.gw.Generate(stepCtx, resolved.Provider, prompt, llmgateway.WithLabel("digest:dedupe"))
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

// generateDigestSummary creates a presentation/summary of the digest from articles and their key points.
// Returns (title, summary, providerType, modelName, error).
func (s *DigestServer) generateDigestSummary(ctx context.Context, analyses []models.ArticleAnalysis, articleMap map[string]models.Article, windowStart, windowEnd time.Time) (string, string, string, string, error) {
	prompt := buildDigestSummaryPrompt(analyses, articleMap, windowStart, windowEnd)

	summaryTemp := 0.5
	resolved, err := ResolveLLM(LLMRequest{Temperature: &summaryTemp})
	if err != nil {
		return "", "", "", "", fmt.Errorf("failed to resolve LLM provider: %w", err)
	}

	log.WithFields(log.Fields{
		"provider":     resolved.ProviderType,
		"model":        resolved.ModelName,
		"articleCount": len(analyses),
	}).Info("Generating digest summary")

	stepCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	rawResponse, err := s.gw.Generate(stepCtx, resolved.Provider, prompt, llmgateway.WithLabel("digest:summary"))
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

func buildDigestSummaryPrompt(analyses []models.ArticleAnalysis, articleMap map[string]models.Article, windowStart, windowEnd time.Time) string {
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

	return fmt.Sprintf(`You are a senior cyber threat intelligence analyst authoring a news digest for a technical security audience (threat hunters, detection engineers, SOC analysts, and incident responders).

Below is a list of articles with their key points. These articles were selected for the digest coverage window shown below. Use this window to frame the digest, but do not imply the reported events occurred exactly within the window unless the article details say so.

The digest should:
1. Provide an executive overview of the main themes and trends
2. Highlight the most critical threats or incidents
3. Group related topics together
4. Be written in a professional, clear, and engaging style
5. Be suitable for both technical and executive audiences

Digest coverage window:
- Start: %s
- End: %s
- Duration: %s

<articles>
%s
</articles>

Respond with a JSON object in this exact format (no markdown fences, no extra text):
{
  "title": "<concise title of 5-20 words that captures the dominant theme of this digest>",
  "summary": "<comprehensive digest summary of approximately 300-500 words that presents these articles in a cohesive narrative, focused on key takeaways>"
}

The summary must be purely factual and descriptive. Do NOT include sections on strategic recommendations, action items, mitigation advice, or "what you should do." This digest is for intelligence reporting only: present threats, incidents, and trends as reported, without prescribing any response.
`, windowStart.UTC().Format(time.RFC3339), windowEnd.UTC().Format(time.RFC3339), windowDuration.String(), articlesList.String())
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
