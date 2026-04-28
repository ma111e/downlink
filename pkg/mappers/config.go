package mappers

import (
	"downlink/pkg/models"
	"downlink/pkg/protos"
)

func ServerConfigToProto(config *models.ServerConfig) (*protos.ServerConfig, error) {
	if config == nil {
		return &protos.ServerConfig{
			Feeds:     []*protos.FeedConfig{},
			Providers: []*protos.ProviderConfig{},
			Analysis:  &protos.AnalysisConfig{},
		}, nil
	}

	emptyAnalysisConfig := protos.AnalysisConfig{}

	protoServerConfig := protos.ServerConfig{
		Feeds:     []*protos.FeedConfig{},
		DbPath:    config.DbPath,
		Providers: []*protos.ProviderConfig{},
		Analysis:  &emptyAnalysisConfig,
	}

	// Convert feeds
	for _, feed := range config.Feeds {
		protoFeed, err := FeedConfigToProto(&feed)
		if err != nil {
			return nil, err
		}
		protoServerConfig.Feeds = append(protoServerConfig.Feeds, protoFeed)
	}

	// Convert providers
	for _, provider := range config.Providers {
		protoServerConfig.Providers = append(protoServerConfig.Providers, ProviderConfigToProto(&provider))
	}

	// Convert analysis
	protoServerConfig.Analysis = AnalysisConfigToProto(&config.Analysis)

	// Convert DefaultSelectors
	if config.DefaultSelectors != nil {
		protoServerConfig.DefaultSelectors = &protos.Selectors{
			Article:   config.DefaultSelectors.Article,
			Cutoff:    config.DefaultSelectors.Cutoff,
			Blacklist: config.DefaultSelectors.Blacklist,
		}
	}

	return &protoServerConfig, nil
}

func ServerConfigToModel(config *protos.ServerConfig) (*models.ServerConfig, error) {
	if config == nil {
		return nil, nil
	}

	servConf := models.ServerConfig{
		Feeds:     []models.FeedConfig{},
		DbPath:    config.DbPath,
		Providers: []models.ProviderConfig{},
		Analysis:  models.AnalysisConfig{},
	}

	// Convert feeds
	for _, feed := range config.Feeds {
		modelFeed, err := FeedConfigToModel(feed)
		if err != nil {
			return nil, err
		}
		if modelFeed != nil {
			servConf.Feeds = append(servConf.Feeds, *modelFeed)
		}
	}

	// Convert providers
	for _, provider := range config.Providers {
		if provider == nil {
			continue
		}
		servConf.Providers = append(servConf.Providers, *ProviderConfigToModel(provider))
	}

	// Convert analysis
	if a := AnalysisConfigToModel(config.Analysis); a != nil {
		servConf.Analysis = *a
	}

	// Convert DefaultSelectors
	if config.DefaultSelectors != nil {
		servConf.DefaultSelectors = &models.Selectors{
			Article:   config.DefaultSelectors.Article,
			Cutoff:    config.DefaultSelectors.Cutoff,
			Blacklist: config.DefaultSelectors.Blacklist,
		}
	}

	return &servConf, nil
}
