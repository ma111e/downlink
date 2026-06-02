package scrapers

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mmcdole/gofeed"
)

// extractHeroImage extracts a hero image URL from a feed item
func extractHeroImage(item *gofeed.Item) string {
	// Check for image in enclosures (common in RSS feeds)
	if len(item.Enclosures) > 0 {
		for _, enclosure := range item.Enclosures {
			if strings.HasPrefix(enclosure.Type, "image/") && enclosure.URL != "" {
				return enclosure.URL
			}
		}
	}

	// Check for image in media content (common in RSS feeds)
	if item.Extensions != nil {
		if media, ok := item.Extensions["media"]; ok {
			if content, ok := media["content"]; ok && len(content) > 0 {
				for _, elem := range content {
					if url, ok := elem.Attrs["url"]; ok && url != "" {
						// Check if it's an image mime type if the type attribute is available
						if mimeType, ok := elem.Attrs["type"]; ok {
							if strings.HasPrefix(mimeType, "image/") {
								return url
							}
						} else {
							// If no type is specified, check if the URL has an image extension
							ext := strings.ToLower(filepath.Ext(url))
							if ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".gif" || ext == ".webp" {
								return url
							}
						}
					}
				}
			}

			// Check for media:thumbnail
			if thumbnails, ok := media["thumbnail"]; ok && len(thumbnails) > 0 {
				for _, thumb := range thumbnails {
					if url, ok := thumb.Attrs["url"]; ok && url != "" {
						return url
					}
				}
			}
		}
	}

	// Check for Open Graph image or Twitter card in the content
	// This is a simplified approach - for proper parsing, you might want to use a HTML parser
	content := item.Content
	if content == "" {
		content = item.Description
	}

	// Simple regex to find og:image or twitter:image meta tags
	ogImageRegex := regexp.MustCompile(`<meta[^>]*property=["']og:image["'][^>]*content=["']([^"']+)["'][^>]*>`)
	if matches := ogImageRegex.FindStringSubmatch(content); len(matches) > 1 {
		return matches[1]
	}

	twitterImageRegex := regexp.MustCompile(`<meta[^>]*name=["']twitter:image["'][^>]*content=["']([^"']+)["'][^>]*>`)
	if matches := twitterImageRegex.FindStringSubmatch(content); len(matches) > 1 {
		return matches[1]
	}

	// Fallback: Search for the first image in the content
	imgRegex := regexp.MustCompile(`<img[^>]*src=["']([^"']+)["'][^>]*>`)
	if matches := imgRegex.FindStringSubmatch(content); len(matches) > 1 {
		return matches[1]
	}

	// No image found
	return ""
}
