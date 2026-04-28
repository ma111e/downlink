package services

import (
	"context"
	"downlink/cmd/server/internal/config"
	"downlink/pkg/mappers"
	"downlink/pkg/protos"

	"google.golang.org/protobuf/types/known/emptypb"
)

// ServerConfigServer implements the ServerConfigService gRPC service
type ServerConfigServer struct {
	protos.UnimplementedServerConfigServiceServer
}

// NewServerConfigServer creates a new ServerConfig server instance
func NewServerConfigServer() *ServerConfigServer {
	return &ServerConfigServer{}
}

// ServerConfigServer implements the ServerConfigService gRPC service
func (s *ServerConfigServer) GetConfig(ctx context.Context, req *protos.GetConfigRequest) (*protos.GetConfigResponse, error) {
	protoConfig, err := mappers.ServerConfigToProto(config.Config)
	if err != nil {
		return nil, err
	}

	return &protos.GetConfigResponse{
		Config: protoConfig,
	}, nil
}

// ServerConfigServer implements the ServerConfigService gRPC service
func (s *ServerConfigServer) SaveConfig(ctx context.Context, req *protos.SaveConfigRequest) (*emptypb.Empty, error) {
	newConfig, err := mappers.ServerConfigToModel(req.Config)
	if err != nil {
		return nil, err
	}
	config.Config = newConfig

	if err := config.Config.Save(config.ConfigPath); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

// UpdateAnalysisConfig updates the analysis configuration
func (s *ServerConfigServer) UpdateAnalysisConfig(_ context.Context, req *protos.UpdateAnalysisConfigRequest) (*emptypb.Empty, error) {
	config.Config.Analysis = *mappers.AnalysisConfigToModel(req.AnalysisConfig)
	if err := config.Config.Save(config.ConfigPath); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}
