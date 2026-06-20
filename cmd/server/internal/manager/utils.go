package manager

import (
	"crypto/md5"
	"fmt"
	"net/url"

	"golang.org/x/net/publicsuffix"
)

func generateFeedId(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	domain, err := publicsuffix.EffectiveTLDPlusOne(u.Hostname())
	if err != nil {
		return "", err
	}
	hash := md5.Sum([]byte(domain))
	return fmt.Sprintf("%x", hash), nil
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
