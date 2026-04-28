package downlinkclient

import (
	"downlink/pkg/protos"
	"io"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"google.golang.org/protobuf/types/known/emptypb"
)

// QueueJobWithTitle represents a job with its article title for display (Wails event payload).
type QueueJobWithTitle struct {
	ID           string `json:"id"`
	ArticleId    string `json:"article_id"`
	ArticleTitle string `json:"article_title"`
	ProviderType string `json:"provider_type,omitempty"`
	ModelName    string `json:"model_name,omitempty"`
	ProviderName string `json:"provider_name,omitempty"`
}

// QueueStatus is the Wails-friendly payload for queue:update events.
type QueueStatus struct {
	Queue        []QueueJobWithTitle `json:"queue"`
	CurrentId    string              `json:"current_id"`
	CurrentTitle string              `json:"current_title"`
	IsProcessing bool                `json:"is_processing"`
}

// AnalysisProgress is the Wails-friendly payload for analysis:progress events.
type AnalysisProgress struct {
	ArticleId  string `json:"article_id"`
	TaskName   string `json:"task_name"`
	Status     string `json:"status"`
	TaskIndex  int    `json:"task_index"`
	TotalTasks int    `json:"total_tasks"`
	TaskResult string `json:"task_result,omitempty"`
	TokenChunk string `json:"token_chunk,omitempty"`
	Error      string `json:"error,omitempty"`
}

type EnqueueOptions struct {
	ArticleIds   []string `json:"article_ids"`
	ProviderType string   `json:"provider_type,omitempty"`
	ModelName    string   `json:"model_name,omitempty"`
	ProviderName string   `json:"provider_name,omitempty"`
	FastMode     bool     `json:"fast_mode,omitempty"`
}

// EnqueueArticles adds article IDs to the server queue.
func (pc *DownlinkClient) EnqueueArticles(options EnqueueOptions) error {
	_, err := pc.queueClient.EnqueueArticles(pc.ctx, &protos.EnqueueArticlesRequest{
		ArticleIds:   options.ArticleIds,
		ProviderType: options.ProviderType,
		ModelName:    options.ModelName,
		ProviderName: options.ProviderName,
		FastMode:     options.FastMode,
	})
	return err
}

// DequeueArticle removes a specific article from the server queue.
func (pc *DownlinkClient) DequeueArticle(articleId string) error {
	_, err := pc.queueClient.DequeueArticle(pc.ctx, &protos.DequeueArticleRequest{
		ArticleId: articleId,
	})
	return err
}

// ClearQueue empties the server queue.
func (pc *DownlinkClient) ClearQueue() error {
	_, err := pc.queueClient.ClearQueue(pc.ctx, &emptypb.Empty{})
	return err
}

// GetQueueStatus returns a snapshot of the server queue state.
func (pc *DownlinkClient) GetQueueStatus() QueueStatus {
	resp, err := pc.queueClient.GetQueueStatus(pc.ctx, &protos.GetQueueStatusRequest{})
	if err != nil {
		log.WithError(err).Warn("Failed to get queue status from server")
		return QueueStatus{}
	}
	return protoToQueueStatus(resp)
}

// StartQueue tells the server to start processing queued articles.
func (pc *DownlinkClient) StartQueue() error {
	_, err := pc.queueClient.StartQueue(pc.ctx, &emptypb.Empty{})
	return err
}

// StopQueue tells the server to pause queue processing.
func (pc *DownlinkClient) StopQueue() error {
	_, err := pc.queueClient.StopQueue(pc.ctx, &emptypb.Empty{})
	return err
}

// --- Streaming bridge (Wails UI only) ---

// StartQueueProgressStream opens a long-lived stream from the server and forwards
// queue state changes and analysis progress as Wails events to the frontend.
// Should be called as a goroutine from startup().
func (pc *DownlinkClient) StartQueueProgressStream() {
	backoff := time.Second
	const maxBackoff = 30 * time.Second

	for {
		err := pc.runQueueStream()
		if err != nil {
			log.WithError(err).Warnf("Queue progress stream disconnected, reconnecting in %s...", backoff)
		} else {
			// Clean disconnect (EOF): reset backoff
			backoff = time.Second
		}

		select {
		case <-pc.ctx.Done():
			return
		case <-time.After(backoff):
		}

		if err != nil {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}

func (pc *DownlinkClient) runQueueStream() error {
	stream, err := pc.queueClient.StreamQueueProgress(pc.ctx, &emptypb.Empty{})
	if err != nil {
		return err
	}

	for {
		event, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		switch event.EventType {
		case "queue_update":
			if event.QueueStatus != nil {
				status := protoToQueueStatus(event.QueueStatus)
				runtime.EventsEmit(pc.ctx, "queue:update", status)
			}
		case "analysis_progress":
			if event.AnalysisProgress != nil {
				p := event.AnalysisProgress
				runtime.EventsEmit(pc.ctx, "analysis:progress", AnalysisProgress{
					ArticleId:  event.ArticleId,
					TaskName:   p.GetTaskName(),
					Status:     p.GetStatus(),
					TaskIndex:  int(p.GetTaskIndex()),
					TotalTasks: int(p.GetTotalTasks()),
					TaskResult: p.GetTaskResult(),
					TokenChunk: p.GetTokenChunk(),
					Error:      p.GetError(),
				})
			}
		}
	}
}

// --- Helpers ---

func protoToQueueStatus(resp *protos.GetQueueStatusResponse) QueueStatus {
	jobs := make([]QueueJobWithTitle, len(resp.Queue))
	for i, j := range resp.Queue {
		jobs[i] = QueueJobWithTitle{
			ID:           j.Id,
			ArticleId:    j.ArticleId,
			ArticleTitle: j.ArticleTitle,
			ProviderType: j.ProviderType,
			ModelName:    j.ModelName,
			ProviderName: j.ProviderName,
		}
	}
	return QueueStatus{
		Queue:        jobs,
		CurrentId:    resp.CurrentId,
		CurrentTitle: resp.CurrentTitle,
		IsProcessing: resp.IsProcessing,
	}
}
