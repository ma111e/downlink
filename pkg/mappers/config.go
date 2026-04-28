package mappers

import (
	"downlink/pkg/models"
	"downlink/pkg/protos"
)

func ServerConfigToProto(config *models.ServerConfig) (*protos.ServerConfig, error) {
	if config == nil {
		return &protos.ServerConfig{
			Feeds:         []*protos.FeedConfig{},
			Providers:     []*protos.ProviderConfig{},
			Analysis:      &protos.AnalysisConfig{},
			Notifications: NotificationsConfigToProto(nil),
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
	protoServerConfig.Notifications = NotificationsConfigToProto(&config.Notifications)

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
	if n := NotificationsConfigToModel(config.Notifications); n != nil {
		servConf.Notifications = *n
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

func NotificationsConfigToProto(config *models.NotificationsConfig) *protos.NotificationsConfig {
	if config == nil {
		return &protos.NotificationsConfig{
			Discord:     &protos.DiscordNotificationConfig{},
			GithubPages: &protos.GitHubPagesNotificationConfig{},
		}
	}

	return &protos.NotificationsConfig{
		Discord: &protos.DiscordNotificationConfig{
			Enabled:    config.Discord.Enabled,
			WebhookUrl: config.Discord.WebhookURL,
		},
		GithubPages: &protos.GitHubPagesNotificationConfig{
			Enabled:           config.GitHubPages.Enabled,
			RepoUrl:           config.GitHubPages.RepoURL,
			Branch:            config.GitHubPages.Branch,
			ConfigurePages:    config.GitHubPages.ConfigurePages,
			Token:             config.GitHubPages.Token,
			OutputDir:         config.GitHubPages.OutputDir,
			BaseUrl:           config.GitHubPages.BaseURL,
			CommitAuthor:      config.GitHubPages.CommitAuthor,
			CommitEmail:       config.GitHubPages.CommitEmail,
			CloneDir:          config.GitHubPages.CloneDir,
			DiscordWebhookUrl: config.GitHubPages.DiscordWebhookURL,
		},
	}
}

func NotificationsConfigToModel(config *protos.NotificationsConfig) *models.NotificationsConfig {
	if config == nil {
		return nil
	}

	out := &models.NotificationsConfig{}
	if config.Discord != nil {
		out.Discord = models.DiscordNotificationConfig{
			Enabled:    config.Discord.Enabled,
			WebhookURL: config.Discord.WebhookUrl,
		}
	}
	if config.GithubPages != nil {
		out.GitHubPages = models.GitHubPagesNotificationConfig{
			Enabled:           config.GithubPages.Enabled,
			RepoURL:           config.GithubPages.RepoUrl,
			Branch:            config.GithubPages.Branch,
			ConfigurePages:    config.GithubPages.ConfigurePages,
			Token:             config.GithubPages.Token,
			OutputDir:         config.GithubPages.OutputDir,
			BaseURL:           config.GithubPages.BaseUrl,
			CommitAuthor:      config.GithubPages.CommitAuthor,
			CommitEmail:       config.GithubPages.CommitEmail,
			CloneDir:          config.GithubPages.CloneDir,
			DiscordWebhookURL: config.GithubPages.DiscordWebhookUrl,
		}
	}

	return out
}
