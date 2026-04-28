package services

import (
	"context"
	"downlink/cmd/server/internal/store"
	"downlink/pkg/protos"
	"sync"
	"time"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"google.golang.org/protobuf/types/known/emptypb"
)

// queueEntry is an internal representation of a queued analysis job.
type queueEntry struct {
	ID           string
	ArticleId    string
	ProviderType string
	ModelName    string
	ProviderName string
	FastMode     bool
}

// QueueServer implements the server-side analysis queue.
//
// Concurrency model: the LLM-call cap lives in the Gateway (which llms uses
// internally). QueueServer keeps its own `workerSem` only to bound how many
// goroutines it spawns from a large backlog — not to bound LLM calls. That
// budget is auto-derived from maxConcurrentLLMRequests (4× with a floor of 4)
// so a 10k-article backlog does not spawn 10k parked goroutines.
type QueueServer struct {
	protos.UnimplementedQueueServiceServer

	mu           sync.Mutex
	queue        []queueEntry
	currentIds   map[string]struct{} // all in-flight article IDs
	isProcessing bool
	workerSem    chan struct{} // bounds fan-out of queue goroutines (NOT LLM calls)

	llms *LLMsServer // direct reference to call runAnalysisPipeline

	listenersMu  sync.Mutex
	listeners    map[uint64]chan *protos.QueueProgressEvent
	nextListenId uint64
}

// NewQueueServer creates a new QueueServer with a reference to the LLMs
// service. The optional maxConcurrentLLMRequests seeds the queue's worker
// fan-out budget (not the LLM-call cap — that lives in the Gateway).
func NewQueueServer(llms *LLMsServer, maxConcurrentLLMRequests int) *QueueServer {
	workerBudget := 4 * maxConcurrentLLMRequests
	if workerBudget < 4 {
		workerBudget = 4
	}
	return &QueueServer{
		llms:       llms,
		workerSem:  make(chan struct{}, workerBudget),
		currentIds: make(map[string]struct{}),
		listeners:  make(map[uint64]chan *protos.QueueProgressEvent),
	}
}

// --- RPC implementations ---

func (s *QueueServer) EnqueueArticles(_ context.Context, req *protos.EnqueueArticlesRequest) (*protos.EnqueueArticlesResponse, error) {
	s.mu.Lock()

	existing := make(map[string]bool)
	for _, e := range s.queue {
		existing[e.ArticleId] = true
	}
	for id := range s.currentIds {
		existing[id] = true
	}

	var added int32
	for _, id := range req.ArticleIds {
		if existing[id] {
			continue
		}
		s.queue = append(s.queue, queueEntry{
			ID:           uuid.New().String(),
			ArticleId:    id,
			ProviderType: req.ProviderType,
			ModelName:    req.ModelName,
			ProviderName: req.ProviderName,
			FastMode:     req.FastMode,
		})
		existing[id] = true
		added++
	}

	s.mu.Unlock()

	if added > 0 {
		log.WithField("count", added).Info("Articles enqueued for analysis")
		s.broadcastQueueUpdate()
		s.startIfIdle()
	}

	return &protos.EnqueueArticlesResponse{EnqueuedCount: added}, nil
}

func (s *QueueServer) DequeueArticle(_ context.Context, req *protos.DequeueArticleRequest) (*emptypb.Empty, error) {
	s.mu.Lock()
	for i, e := range s.queue {
		if e.ArticleId == req.ArticleId {
			s.queue = append(s.queue[:i], s.queue[i+1:]...)
			break
		}
	}
	s.mu.Unlock()

	s.broadcastQueueUpdate()
	return &emptypb.Empty{}, nil
}

func (s *QueueServer) ClearQueue(_ context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	s.mu.Lock()
	s.queue = nil
	s.mu.Unlock()

	s.broadcastQueueUpdate()
	return &emptypb.Empty{}, nil
}

func (s *QueueServer) GetQueueStatus(_ context.Context, _ *protos.GetQueueStatusRequest) (*protos.GetQueueStatusResponse, error) {
	return s.buildQueueStatus(), nil
}

func (s *QueueServer) StartQueue(_ context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	s.startIfIdle()
	return &emptypb.Empty{}, nil
}

func (s *QueueServer) StopQueue(_ context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	s.mu.Lock()
	s.isProcessing = false
	s.mu.Unlock()

	s.broadcastQueueUpdate()
	return &emptypb.Empty{}, nil
}

func (s *QueueServer) StreamQueueProgress(_ *emptypb.Empty, stream protos.QueueService_StreamQueueProgressServer) error {
	ch := make(chan *protos.QueueProgressEvent, 64)

	s.listenersMu.Lock()
	id := s.nextListenId
	s.nextListenId++
	s.listeners[id] = ch
	s.listenersMu.Unlock()

	defer func() {
		s.listenersMu.Lock()
		delete(s.listeners, id)
		s.listenersMu.Unlock()
	}()

	// Send initial queue status
	status := s.buildQueueStatus()
	if err := stream.Send(&protos.QueueProgressEvent{
		EventType:   "queue_update",
		QueueStatus: status,
	}); err != nil {
		return err
	}

	ctx := stream.Context()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event := <-ch:
			if err := stream.Send(event); err != nil {
				return err
			}
		}
	}
}

// --- Internal helpers ---

func (s *QueueServer) startIfIdle() {
	s.mu.Lock()
	if s.isProcessing || len(s.queue) == 0 {
		s.mu.Unlock()
		return
	}
	s.isProcessing = true
	s.mu.Unlock()

	s.broadcastQueueUpdate()
	go s.processQueue()
}

func (s *QueueServer) processQueue() {
	var wg sync.WaitGroup

	for {
		s.mu.Lock()
		if !s.isProcessing || len(s.queue) == 0 {
			s.mu.Unlock()
			break
		}

		job := s.queue[0]
		s.queue = s.queue[1:]
		s.currentIds[job.ArticleId] = struct{}{}
		s.mu.Unlock()

		s.broadcastQueueUpdate()

		// Bound how many queue goroutines we spawn. The LLM-call cap lives
		// in the Gateway (inside s.llms); this only prevents a large backlog
		// from spawning a goroutine per pending article.
		s.workerSem <- struct{}{}

		// Re-check stop after potentially waiting for the worker budget
		s.mu.Lock()
		stopped := !s.isProcessing
		s.mu.Unlock()

		if stopped {
			s.mu.Lock()
			s.queue = append([]queueEntry{job}, s.queue...)
			delete(s.currentIds, job.ArticleId)
			s.mu.Unlock()
			<-s.workerSem
			s.broadcastQueueUpdate()
			break
		}

		wg.Add(1)
		go func(j queueEntry) {
			defer wg.Done()
			defer func() {
				<-s.workerSem
				s.mu.Lock()
				delete(s.currentIds, j.ArticleId)
				s.mu.Unlock()
				s.broadcastQueueUpdate()
			}()

			log.WithFields(log.Fields{
				"article_id":    j.ArticleId,
				"provider_type": j.ProviderType,
				"model_name":    j.ModelName,
				"provider_name": j.ProviderName,
				"fast_mode":     j.FastMode,
			}).Info("Starting analysis for queued article")

			if err := s.runJob(j); err != nil {
				log.WithError(err).WithField("article_id", j.ArticleId).Error("Queue analysis failed")
				s.broadcast(&protos.QueueProgressEvent{
					EventType: "analysis_progress",
					ArticleId: j.ArticleId,
					AnalysisProgress: &protos.AnalysisProgressEvent{
						Status: "error",
						Error:  err.Error(),
					},
				})
			}
		}(job)
	}

	// Wait for all in-flight jobs before marking idle
	wg.Wait()

	s.mu.Lock()
	s.isProcessing = false
	s.mu.Unlock()
	s.broadcastQueueUpdate()
}

func (s *QueueServer) runJob(job queueEntry) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	req := &protos.AnalyzeArticleWithProviderModelRequest{
		ArticleId:    job.ArticleId,
		ProviderType: job.ProviderType,
		ModelName:    job.ModelName,
		ProviderName: job.ProviderName,
		FastMode:     job.FastMode,
	}

	onProgress := func(taskName, status string, taskIndex, totalTasks int, data string, taskErr error) {
		event := &protos.AnalysisProgressEvent{
			TaskName:   taskName,
			Status:     status,
			TaskIndex:  int32(taskIndex),
			TotalTasks: int32(totalTasks),
		}
		switch status {
		case "token":
			event.TokenChunk = data
		case "completed":
			event.TaskResult = data
		case "error":
			if taskErr != nil {
				event.Error = taskErr.Error()
			}
		}
		s.broadcast(&protos.QueueProgressEvent{
			EventType:        "analysis_progress",
			ArticleId:        job.ArticleId,
			AnalysisProgress: event,
		})
	}

	res, err := s.llms.runAnalysisPipeline(ctx, req, onProgress)
	if err != nil {
		return err
	}

	// Send the final "done" event with the analysis result
	s.broadcast(&protos.QueueProgressEvent{
		EventType: "analysis_progress",
		ArticleId: job.ArticleId,
		AnalysisProgress: &protos.AnalysisProgressEvent{
			Status:   "done",
			Analysis: res.Analysis,
		},
	})

	return nil
}

func (s *QueueServer) buildQueueStatus() *protos.GetQueueStatusResponse {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Collect all unique article IDs (queue + in-flight) for a single batch fetch
	idSet := make(map[string]struct{}, len(s.queue)+len(s.currentIds))
	for _, e := range s.queue {
		idSet[e.ArticleId] = struct{}{}
	}
	for id := range s.currentIds {
		idSet[id] = struct{}{}
	}
	allIds := make([]string, 0, len(idSet))
	for id := range idSet {
		allIds = append(allIds, id)
	}

	titleMap := make(map[string]string, len(allIds))
	if len(allIds) > 0 {
		articles, err := store.Db.GetArticlesBatch(allIds)
		if err == nil {
			for _, a := range articles {
				titleMap[a.Id] = a.Title
			}
		}
	}

	getTitle := func(id string) string {
		if id == "" {
			return ""
		}
		if t, ok := titleMap[id]; ok {
			return t
		}
		return "(unknown)"
	}

	jobs := make([]*protos.QueueJob, len(s.queue))
	for i, e := range s.queue {
		jobs[i] = &protos.QueueJob{
			Id:           e.ID,
			ArticleId:    e.ArticleId,
			ArticleTitle: getTitle(e.ArticleId),
			ProviderType: e.ProviderType,
			ModelName:    e.ModelName,
			ProviderName: e.ProviderName,
		}
	}

	// For proto compatibility, populate CurrentId with any one in-flight ID
	var representativeCurrentId string
	for id := range s.currentIds {
		representativeCurrentId = id
		break
	}

	return &protos.GetQueueStatusResponse{
		Queue:        jobs,
		CurrentId:    representativeCurrentId,
		CurrentTitle: getTitle(representativeCurrentId),
		IsProcessing: s.isProcessing,
	}
}

func (s *QueueServer) broadcast(event *protos.QueueProgressEvent) {
	s.listenersMu.Lock()
	defer s.listenersMu.Unlock()
	for _, ch := range s.listeners {
		select {
		case ch <- event:
		default:
			// drop if listener is slow
		}
	}
}

func (s *QueueServer) broadcastQueueUpdate() {
	status := s.buildQueueStatus()
	s.broadcast(&protos.QueueProgressEvent{
		EventType:   "queue_update",
		QueueStatus: status,
	})
}
