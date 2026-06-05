package claudeauth

import "errors"

var (
	ErrReloginRequired = errors.New("claude: re-login required")
	ErrNoCredentials   = errors.New("claude: no healthy credentials available")
	ErrStateMismatch   = errors.New("claude: oauth state mismatch")
)
