package scrapers

import (
	"testing"

	"github.com/mmcdole/gofeed"
	ext "github.com/mmcdole/gofeed/extensions"
)

func TestExtractHeroImageFromImageEnclosure(t *testing.T) {
	item := &gofeed.Item{Enclosures: []*gofeed.Enclosure{
		{URL: "https://x/audio.mp3", Type: "audio/mpeg"}, // skipped, not an image
		{URL: "https://x/pic.jpg", Type: "image/jpeg"},
	}}
	if got := extractHeroImage(item); got != "https://x/pic.jpg" {
		t.Fatalf("got %q, want the image/* enclosure URL", got)
	}
}

func TestExtractHeroImageSkipsNonImageEnclosure(t *testing.T) {
	item := &gofeed.Item{Enclosures: []*gofeed.Enclosure{
		{URL: "https://x/audio.mp3", Type: "audio/mpeg"},
	}}
	if got := extractHeroImage(item); got != "" {
		t.Fatalf("got %q, want empty (no image enclosure, no content)", got)
	}
}

func TestExtractHeroImageFromMediaContent(t *testing.T) {
	item := &gofeed.Item{Extensions: ext.Extensions{
		"media": {
			"content": []ext.Extension{
				{Attrs: map[string]string{"url": "https://x/media.png", "type": "image/png"}},
			},
		},
	}}
	if got := extractHeroImage(item); got != "https://x/media.png" {
		t.Fatalf("got %q, want media:content image URL", got)
	}
}

func TestExtractHeroImageFromMediaThumbnail(t *testing.T) {
	item := &gofeed.Item{Extensions: ext.Extensions{
		"media": {
			"thumbnail": []ext.Extension{
				{Attrs: map[string]string{"url": "https://x/thumb.jpg"}},
			},
		},
	}}
	if got := extractHeroImage(item); got != "https://x/thumb.jpg" {
		t.Fatalf("got %q, want media:thumbnail URL", got)
	}
}

func TestExtractHeroImageFromOGImageMeta(t *testing.T) {
	item := &gofeed.Item{Content: `<meta property="og:image" content="https://x/og.png">`}
	if got := extractHeroImage(item); got != "https://x/og.png" {
		t.Fatalf("got %q, want og:image URL", got)
	}
}

func TestExtractHeroImageFromTwitterImageMeta(t *testing.T) {
	item := &gofeed.Item{Content: `<meta name="twitter:image" content="https://x/tw.png">`}
	if got := extractHeroImage(item); got != "https://x/tw.png" {
		t.Fatalf("got %q, want twitter:image URL", got)
	}
}

func TestExtractHeroImageFallsBackToFirstImg(t *testing.T) {
	// No og/twitter meta; falls back to the first <img src>. Uses Description
	// because Content is empty.
	item := &gofeed.Item{Description: `<p>hi</p><img src="https://x/inline.gif"><img src="https://x/second.png">`}
	if got := extractHeroImage(item); got != "https://x/inline.gif" {
		t.Fatalf("got %q, want the first <img> src", got)
	}
}

func TestExtractHeroImageNoneFound(t *testing.T) {
	item := &gofeed.Item{Content: "<p>just text, no images</p>"}
	if got := extractHeroImage(item); got != "" {
		t.Fatalf("got %q, want empty when no image present", got)
	}
}

// Precedence: an image enclosure wins over content meta tags.
func TestExtractHeroImageEnclosureBeatsContent(t *testing.T) {
	item := &gofeed.Item{
		Enclosures: []*gofeed.Enclosure{{URL: "https://x/enc.jpg", Type: "image/jpeg"}},
		Content:    `<meta property="og:image" content="https://x/og.png">`,
	}
	if got := extractHeroImage(item); got != "https://x/enc.jpg" {
		t.Fatalf("got %q, want enclosure to take precedence over og:image", got)
	}
}
