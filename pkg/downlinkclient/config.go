package downlinkclient

import (
	"downlink/pkg/mappers"
	"downlink/pkg/models"
	"downlink/pkg/protos"
)

// GetConfig returns the current application configuration
func (pc *DownlinkClient) GetConfig() (models.ServerConfig, error) {
	config, err := pc.serverConfigClient.GetConfig(pc.ctx, &protos.GetConfigRequest{})
	if err != nil {
		return models.ServerConfig{}, err
	}

	serverConfig, err := mappers.ServerConfigToModel(config.Config)
	if err != nil {
		return models.ServerConfig{}, err
	}
	if serverConfig == nil {
		return models.ServerConfig{}, nil
	}
	return *serverConfig, nil
}

// SaveConfig updates the application configuration
func (pc *DownlinkClient) SaveConfig(config models.ServerConfig) error {
	protoConfig, err := mappers.ServerConfigToProto(&config)
	if err != nil {
		return err
	}

	_, err = pc.serverConfigClient.SaveConfig(pc.ctx, &protos.SaveConfigRequest{Config: protoConfig})

	return err
}
