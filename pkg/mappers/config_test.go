package mappers

import (
	"testing"

	"downlink/pkg/models"
)

func TestServerConfigRoundTripsNotifications(t *testing.T) {
	input := &models.ServerConfig{
		DbPath: "downlink.db",
		Notifications: models.NotificationsConfig{
			Discord: models.DiscordNotificationConfig{
				Enabled:    true,
				WebhookURL: "https://discord.example/webhook",
			},
			GitHubPages: models.GitHubPagesNotificationConfig{
				Enabled:           true,
				RepoURL:           "https://github.com/owner/repo.git",
				Branch:            "pages",
				ConfigurePages:    true,
				Token:             "token",
				OutputDir:         "digests",
				BaseURL:           "https://owner.github.io/repo",
				CommitAuthor:      "downlink",
				CommitEmail:       "downlink@example.com",
				CloneDir:          "/tmp/downlink-pages",
				DiscordWebhookURL: "https://discord.example/pages",
			},
		},
	}

	protoConfig, err := ServerConfigToProto(input)
	if err != nil {
		t.Fatalf("ServerConfigToProto() error = %v", err)
	}

	output, err := ServerConfigToModel(protoConfig)
	if err != nil {
		t.Fatalf("ServerConfigToModel() error = %v", err)
	}
	if output == nil {
		t.Fatalf("ServerConfigToModel() returned nil")
	}

	if output.Notifications.Discord != input.Notifications.Discord {
		t.Fatalf("discord config = %+v, want %+v", output.Notifications.Discord, input.Notifications.Discord)
	}
	if output.Notifications.GitHubPages != input.Notifications.GitHubPages {
		t.Fatalf("github pages config = %+v, want %+v", output.Notifications.GitHubPages, input.Notifications.GitHubPages)
	}
}
