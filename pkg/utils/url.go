package utils

import (
	"net/url"
	"strings"
)

// JoinURL joins a base URL with path segments, trimming slashes and skipping
// empty segments. A trailing empty segment yields the directory URL. When base
// is empty the result is a root-relative path.
func JoinURL(base string, segments ...string) string {
	parts := make([]string, 0, len(segments)+1)
	if trimmed := strings.TrimRight(strings.TrimSpace(base), "/"); trimmed != "" {
		parts = append(parts, trimmed)
	}
	for _, s := range segments {
		s = strings.Trim(strings.TrimSpace(s), "/")
		if s != "" {
			parts = append(parts, s)
		}
	}
	joined := strings.Join(parts, "/")
	if base == "" {
		return "/" + strings.TrimPrefix(joined, "/")
	}
	return joined
}

// ResolveLink returns link unchanged when it is already absolute or when base
// is empty. Otherwise it joins base with the (relative) link.
func ResolveLink(base, link string) string {
	if strings.TrimSpace(base) == "" {
		return link
	}
	if u, err := url.Parse(strings.TrimSpace(link)); err == nil && u.IsAbs() {
		return link
	}
	return JoinURL(base, link)
}
