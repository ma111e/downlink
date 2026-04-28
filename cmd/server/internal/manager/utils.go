package manager

import (
	"crypto/md5"
	"fmt"
)

// generateFeedId generates a MD5 hash
func generateFeedId(url string) string {
	hash := md5.Sum([]byte(url))
	return fmt.Sprintf("%x", hash)
}

// generateArticleId generates a unique Id for an article
func generateArticleId(feedId, itemId, itemTitle string) string {
	var hash [16]byte

	if itemId != "" {
		// Hash the itemId to ensure the second part is always a hash
		hash = md5.Sum([]byte(itemId))
	} else {
		// If no itemId is provided, generate a unique hash based on its title
		hash = md5.Sum([]byte(itemTitle))
	}

	// Return the feed Id followed by the first 8 bytes of the hash
	return fmt.Sprintf("%s:%x", feedId, hash[:8])
}
