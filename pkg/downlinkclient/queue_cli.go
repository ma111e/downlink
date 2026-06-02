package downlinkclient

import (
	"io"
	"time"

	"google.golang.org/protobuf/types/known/emptypb"
)

// QueueStreamCallbacks holds event handlers for CLI queue streaming.
type QueueStreamCallbacks struct {
	OnQueueUpdate      func(QueueStatus)
	OnAnalysisProgress func(AnalysisProgress)
	OnDisconnect       func(error) // called before each reconnect attempt; may be nil
}

// StreamQueueEvents opens a long-lived gRPC stream and dispatches queue state
// changes and per-task analysis progress to the provided callbacks.
// Reconnects with exponential back-off up to 30s. Blocks until ctx is cancelled.
// Intended to be run in a goroutine.
func (pc *DownlinkClient) StreamQueueEvents(cb QueueStreamCallbacks) {
	backoff := time.Second
	const maxBackoff = 30 * time.Second

	for {
		err := pc.runQueueStreamCLI(cb)
		if err == nil {
			backoff = time.Second // clean EOF; reset back-off
		} else {
			if cb.OnDisconnect != nil {
				cb.OnDisconnect(err)
			}
			backoff = min(backoff*2, maxBackoff)
		}

		select {
		case <-pc.ctx.Done():
			return
		case <-time.After(backoff):
		}
	}
}

func (pc *DownlinkClient) runQueueStreamCLI(cb QueueStreamCallbacks) error {
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
			if event.QueueStatus != nil && cb.OnQueueUpdate != nil {
				cb.OnQueueUpdate(protoToQueueStatus(event.QueueStatus))
			}
		case "analysis_progress":
			if event.AnalysisProgress != nil && cb.OnAnalysisProgress != nil {
				p := event.AnalysisProgress
				cb.OnAnalysisProgress(AnalysisProgress{
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
