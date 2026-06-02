package notification

import (
	"github.com/ma111e/downlink/pkg/models"
)

// Notifier defines the interface for sending digests to notification platforms
type Notifier interface {
	SendDigest(digest models.Digest) error
}
