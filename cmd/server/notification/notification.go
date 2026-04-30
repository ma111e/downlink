// Package notification re-exports the GitHub Pages publisher from the internal
// notification package so that packages outside cmd/server (such as the CLI)
// can use it without violating Go's internal-package import restrictions.
package notification

import (
	internal "downlink/cmd/server/internal/notification"
	"downlink/pkg/models"
)

// GitHubPagesPublisher is a type alias for the internal publisher; the two
// types are identical and their methods are fully interchangeable.
type GitHubPagesPublisher = internal.GitHubPagesPublisher

// NewGitHubPagesPublisher delegates to the internal constructor.
func NewGitHubPagesPublisher(cfg models.GitHubPagesNotificationConfig) *GitHubPagesPublisher {
	return internal.NewGitHubPagesPublisher(cfg)
}
