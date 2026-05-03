package downlinkclient

import (
	"context"
	"downlink/pkg/protos"
	"os"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
)

// DownlinkClient struct
type DownlinkClient struct {
	ctx                context.Context
	articleClient      protos.ArticleServiceClient
	analysisClient     protos.AnalysisServiceClient
	digestClient       protos.DigestServiceClient
	feedsClient        protos.FeedsServiceClient
	llmsClient         protos.LLMsServiceClient
	queueClient        protos.QueueServiceClient
	serverConfigClient protos.ServerConfigServiceClient
	authClient         protos.AuthServiceClient
	conn               *grpc.ClientConn
}

// NewDownlinkClient creates a new DownlinkClient application struct
func NewDownlinkClient(conn *grpc.ClientConn) *DownlinkClient {
	return &DownlinkClient{
		ctx:                context.Background(),
		articleClient:      protos.NewArticleServiceClient(conn),
		analysisClient:     protos.NewAnalysisServiceClient(conn),
		digestClient:       protos.NewDigestServiceClient(conn),
		feedsClient:        protos.NewFeedsServiceClient(conn),
		llmsClient:         protos.NewLLMsServiceClient(conn),
		queueClient:        protos.NewQueueServiceClient(conn),
		serverConfigClient: protos.NewServerConfigServiceClient(conn),
		authClient:         protos.NewAuthServiceClient(conn),
		conn:               conn,
	}
}

func (pc *DownlinkClient) Entry(ctx context.Context) {
	pc.startup(ctx)
}

// startup is called when the Wails app starts.
func (pc *DownlinkClient) startup(ctx context.Context) {
	pc.ctx = ctx

	log.SetFormatter(&log.TextFormatter{FullTimestamp: true})
	log.SetOutput(os.Stdout)
	log.SetLevel(log.InfoLevel)

	log.Info("Starting feed aggregator")

	// Stream queue and analysis progress events from the server to the frontend.
	go pc.StartQueueProgressStream()

	// Start monitoring gRPC connection state and notifying the frontend
	go pc.watchConnection(ctx)
}

// watchConnection polls the gRPC connection state and emits connection:status
// events to the frontend whenever the state changes.
func (pc *DownlinkClient) watchConnection(ctx context.Context) {
	prev := connectivity.Idle
	// Emit the initial state immediately
	runtime.EventsEmit(pc.ctx, "connection:status", pc.conn.GetState() == connectivity.Ready)

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}

		state := pc.conn.GetState()
		if state == prev {
			continue
		}

		wasConnected := prev == connectivity.Ready
		isConnected := state == connectivity.Ready

		log.Infof("gRPC connection state changed: %s → %s", prev, state)
		prev = state

		runtime.EventsEmit(pc.ctx, "connection:status", isConnected)

		// On reconnect, request a fresh queue status from the server.
		if !wasConnected && isConnected {
			log.Info("Reconnected to server — refreshing queue status")
			status := pc.GetQueueStatus()
			runtime.EventsEmit(pc.ctx, "queue:update", status)
		}
	}
}

// Shutdown is called when the app is shutting down
func (pc *DownlinkClient) Shutdown(_ context.Context) {
	pc.conn.Close()
	log.Info("Application shutting down")
}
