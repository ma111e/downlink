package config

import "github.com/ma111e/downlink/pkg/claudeauth"

// ClaudeManager is the singleton credential pool manager for claude-code providers.
// Set once at server startup by main.go before any requests are served.
var ClaudeManager *claudeauth.Manager
