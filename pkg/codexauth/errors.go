package codexauth

import "errors"

var (
	ErrReloginRequired = errors.New("codex: re-login required")
	ErrLoginTimeout    = errors.New("codex: device login timed out")
	ErrNoCredentials   = errors.New("codex: no healthy credentials available")
	ErrInvalidJWT      = errors.New("codex: invalid JWT format")
)
