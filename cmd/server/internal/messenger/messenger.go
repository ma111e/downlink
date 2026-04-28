package messenger

import (
	"downlink/pkg/models"
)

// Messenger defines the interface for sending digests to messaging platforms
type Messenger interface {
	SendDigest(digest models.Digest) error
}
