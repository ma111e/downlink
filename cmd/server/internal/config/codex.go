package config

import "github.com/ma111e/downlink/pkg/codexauth"

// CodexManager is the singleton credential pool manager for openai-codex providers.
// Set once at server startup by main.go before any requests are served.
var CodexManager *codexauth.Manager
