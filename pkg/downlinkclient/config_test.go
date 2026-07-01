package downlinkclient

import (
	"context"
	"testing"

	"github.com/ma111e/downlink/pkg/models"
	"github.com/ma111e/downlink/pkg/protos"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

type stubServerConfig struct {
	protos.UnimplementedServerConfigServiceServer

	lastSave *protos.SaveConfigRequest
	getResp  *protos.GetConfigResponse
}

func (s *stubServerConfig) GetConfig(_ context.Context, _ *protos.GetConfigRequest) (*protos.GetConfigResponse, error) {
	return s.getResp, nil
}
func (s *stubServerConfig) SaveConfig(_ context.Context, r *protos.SaveConfigRequest) (*emptypb.Empty, error) {
	s.lastSave = r
	return &emptypb.Empty{}, nil
}

func configClient(t *testing.T, stub protos.ServerConfigServiceServer) *DownlinkClient {
	return dialStub(t, func(s *grpc.Server) { protos.RegisterServerConfigServiceServer(s, stub) })
}

func TestGetConfigMapsToModel(t *testing.T) {
	stub := &stubServerConfig{getResp: &protos.GetConfigResponse{
		Config: &protos.ServerConfig{DbPath: "/data/downlink.db"},
	}}
	pc := configClient(t, stub)

	cfg, err := pc.GetConfig()
	if err != nil {
		t.Fatalf("GetConfig() error = %v", err)
	}
	if cfg.DbPath != "/data/downlink.db" {
		t.Fatalf("DbPath = %q, want /data/downlink.db", cfg.DbPath)
	}
}

func TestSaveConfigSendsMappedConfig(t *testing.T) {
	stub := &stubServerConfig{}
	pc := configClient(t, stub)

	if err := pc.SaveConfig(models.ServerConfig{DbPath: "/db"}); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	if stub.lastSave == nil || stub.lastSave.Config == nil || stub.lastSave.Config.DbPath != "/db" {
		t.Fatalf("received save req = %+v, want Config.DbPath /db", stub.lastSave)
	}
}
