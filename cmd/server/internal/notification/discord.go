package notification

import (
	"bytes"
	"downlink/pkg/models"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/PuerkitoBio/goquery"
	log "github.com/sirupsen/logrus"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// DiscordNotifier implements the Notifier interface for Discord webhooks
type DiscordNotifier struct {
	webhookURL string
	client     *http.Client
}

// NewDiscordNotifier creates a new Discord notifier
func NewDiscordNotifier(webhookURL string) *DiscordNotifier {
	return &DiscordNotifier{
		webhookURL: webhookURL,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// DiscordWebhookPayload represents the Discord webhook message structure
type DiscordWebhookPayload struct {
	Content string         `json:"content,omitempty"`
	Embeds  []DiscordEmbed `json:"embeds,omitempty"`
}

// DiscordEmbed represents a Discord embed
type DiscordEmbed struct {
	Title       string              `json:"title,omitempty"`
	Description string              `json:"description,omitempty"`
	Color       int                 `json:"color,omitempty"`
	Fields      []DiscordEmbedField `json:"fields,omitempty"`
	Timestamp   string              `json:"timestamp,omitempty"`
	Footer      *DiscordEmbedFooter `json:"footer,omitempty"`
	Image       *DiscordEmbedImage  `json:"image,omitempty"`
}

// DiscordEmbedImage represents an image in a Discord embed
type DiscordEmbedImage struct {
	URL string `json:"url"`
}

// DiscordEmbedField represents a field in a Discord embed
type DiscordEmbedField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline,omitempty"`
}

// DiscordEmbedFooter represents a footer in a Discord embed
type DiscordEmbedFooter struct {
	Text string `json:"text"`
}

// SendDigest sends a digest to Discord
func (d *DiscordNotifier) SendDigest(digest models.Digest) error {
	log.WithField("digestId", digest.Id).Info("Sending digest to Discord")

	embeds := d.buildEmbeds(digest)

	// Discord has a limit of 10 embeds per message
	// If we have more, we'll send multiple messages
	for i := 0; i < len(embeds); i += 10 {
		end := min(i+10, len(embeds))

		payload := DiscordWebhookPayload{
			Embeds: embeds[i:end],
		}

		if err := d.sendWebhook(payload); err != nil {
			return fmt.Errorf("failed to send Discord webhook: %w", err)
		}

		// Add a small delay between messages to avoid rate limiting
		if end < len(embeds) {
			time.Sleep(1 * time.Second)
		}
	}

	log.Info("Digest sent to Discord successfully")
	return nil
}

func (d *DiscordNotifier) buildEmbeds(digest models.Digest) []DiscordEmbed {
	var embeds []DiscordEmbed

	// Main digest embed
	description := digest.DigestSummary
	if description == "" {
		description = fmt.Sprintf("Digest covering the last %s.", formatDuration(digest.TimeWindow))
	}

	mainEmbed := DiscordEmbed{
		Title:       "📰 DOWNLINK Digest",
		Description: truncateString(description, 4096),
		Color:       0x5865F2, // Discord blurple
		Timestamp:   digest.CreatedAt.Format(time.RFC3339),
		Footer: &DiscordEmbedFooter{
			Text: fmt.Sprintf("DOWNLINK • %s window", formatDuration(digest.TimeWindow)),
		},
	}

	mainEmbed.Fields = append(mainEmbed.Fields, DiscordEmbedField{
		Name:   "📄 Articles",
		Value:  fmt.Sprintf("%d", *digest.ArticleCount),
		Inline: true,
	})

	mainEmbed.Fields = append(mainEmbed.Fields, DiscordEmbedField{
		Name:   "🤖 Providers",
		Value:  fmt.Sprintf("%d", len(digest.ProviderResults)),
		Inline: true,
	})

	embeds = append(embeds, mainEmbed)

	// One embed per article — title (linked) and first paragraph
	for _, article := range digest.Articles {
		articleEmbed := d.buildArticleEmbed(article)
		embeds = append(embeds, articleEmbed)
	}

	// One embed per provider — show only StandardSynthesis as the main content
	for _, result := range digest.ProviderResults {
		providerEmbed := d.buildProviderEmbed(result)
		embeds = append(embeds, providerEmbed)
	}

	return embeds
}

func (d *DiscordNotifier) buildArticleEmbed(article models.Article) DiscordEmbed {
	title := article.Title
	if title == "" {
		title = "Untitled"
	}

	// Make the title a clickable hyperlink using Discord's masked link syntax
	// Bold the title for better readability: **[Title](url)**
	embedTitle := fmt.Sprintf("**[%s](%s)**", title, article.Link)

	embed := DiscordEmbed{
		Description: embedTitle,
		Color:       0x36393F, // Discord dark
	}

	paragraph := firstParagraph(article.Content)
	if paragraph != "" {
		embed.Description = fmt.Sprintf("%s\n\n%s", embedTitle, truncateString(paragraph, 3800))
	}

	if source := articleSource(article.Link); source != "" {
		embed.Description = fmt.Sprintf("%s\n\n*%s*", embed.Description, source)
	}

	// Add hero image if available
	if article.HeroImage != "" {
		embed.Image = &DiscordEmbedImage{
			URL: article.HeroImage,
		}
	}

	return embed
}

// articleSource returns the hostname of the article URL to use as a source label.
func articleSource(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return ""
	}
	host := strings.TrimPrefix(u.Host, "www.")
	return host
}

// firstParagraph extracts the plain text of the first non-empty <p> from HTML content.
// Falls back to the first 300 characters of stripped text if no <p> is found.
func firstParagraph(htmlContent string) string {
	if htmlContent == "" {
		return ""
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlContent))
	if err != nil {
		return ""
	}

	var paragraph string
	doc.Find("p").EachWithBreak(func(_ int, s *goquery.Selection) bool {
		text := strings.TrimSpace(s.Text())
		if text != "" {
			paragraph = text
			return false // stop after first non-empty paragraph
		}
		return true
	})

	if paragraph != "" {
		return paragraph
	}

	// Fallback: strip all tags and return the first 300 chars
	plain := strings.TrimSpace(doc.Text())
	return truncateString(plain, 300)
}

func (d *DiscordNotifier) buildProviderEmbed(result models.DigestProviderResult) DiscordEmbed {
	providerLabel := cases.Title(language.English).String(result.ProviderType)
	embed := DiscordEmbed{
		Title: fmt.Sprintf("%s — %s", providerLabel, result.ModelName),
		Color: d.getProviderColor(result.ProviderType),
	}

	// Surface error and stop — nothing else is meaningful if the provider failed
	if result.Error != "" {
		embed.Fields = append(embed.Fields, DiscordEmbedField{
			Name:  "❌ Error",
			Value: truncateString(result.Error, 1024),
		})
		return embed
	}

	// Use StandardSynthesis as the primary content — it's the right balance for Discord.
	// BriefOverview is too thin; ComprehensiveSynthesis exceeds Discord's embed limits.
	// Fall back to BriefOverview if StandardSynthesis is absent.
	synthesis := result.StandardSynthesis
	if synthesis == "" {
		synthesis = result.BriefOverview
	}

	if synthesis != "" {
		// Discord embed description supports up to 4096 chars, use that instead of a field
		// so the text isn't artificially boxed into a 1024-char field.
		embed.Description = truncateString(synthesis, 4096)
	}

	return embed
}

func (d *DiscordNotifier) getProviderColor(providerType string) int {
	colors := map[string]int{
		"openai":    0x10A37F, // OpenAI green
		"anthropic": 0xD97757, // Anthropic orange
		"ollama":    0x000000, // Black
		"mistral":   0xFF7000, // Mistral orange
	}

	if color, ok := colors[providerType]; ok {
		return color
	}
	return 0x5865F2 // Default Discord blurple
}

// SendHTMLFile uploads a digest HTML file to the Discord webhook as an attachment.
func (d *DiscordNotifier) SendHTMLFile(filename string, data []byte) error {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)

	part, err := w.CreateFormFile("file", filename)
	if err != nil {
		return fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := part.Write(data); err != nil {
		return fmt.Errorf("failed to write file data: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("failed to close multipart writer: %w", err)
	}

	req, err := http.NewRequest("POST", d.webhookURL, &body)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusTooManyRequests {
		retryAfter := 5.0
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if parsed, err := strconv.ParseFloat(ra, 64); err == nil {
				retryAfter = parsed
			}
		}
		log.WithField("retry_after_s", retryAfter).Warn("Discord rate limited on file upload, retrying")
		time.Sleep(time.Duration(retryAfter*1000) * time.Millisecond)
		return d.SendHTMLFile(filename, data)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Discord webhook file upload returned status %d: %s", resp.StatusCode, string(respBody))
	}

	log.WithField("filename", filename).Info("Digest HTML file sent to Discord")
	return nil
}

func (d *DiscordNotifier) sendWebhook(payload DiscordWebhookPayload) error {
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequest("POST", d.webhookURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	// Handle rate limiting with retry
	if resp.StatusCode == http.StatusTooManyRequests {
		retryAfter := 5.0
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if parsed, err := strconv.ParseFloat(ra, 64); err == nil {
				retryAfter = parsed
			}
		}
		log.WithField("retry_after_s", retryAfter).Warn("Discord rate limited, retrying")
		time.Sleep(time.Duration(retryAfter*1000) * time.Millisecond)
		return d.sendWebhook(payload)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Discord webhook returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// SendDiscordMessage posts a plain-text message to a Discord webhook URL.
// Intended for lightweight one-line notifications (e.g. "new page published").
func SendDiscordMessage(webhookURL, content string) error {
	n := NewDiscordNotifier(webhookURL)
	return n.sendWebhook(DiscordWebhookPayload{Content: content})
}

// truncateString truncates a string to maxLen runes, adding "..." if truncated
func truncateString(s string, maxLen int) string {
	if utf8.RuneCountInString(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return string([]rune(s)[:maxLen])
	}
	return string([]rune(s)[:maxLen-3]) + "..."
}

// formatDuration formats a duration in a human-readable way
func formatDuration(d time.Duration) string {
	hours := int(d.Hours())
	if hours >= 24 {
		days := hours / 24
		if days == 1 {
			return "1 day"
		}
		return fmt.Sprintf("%d days", days)
	}
	if hours == 1 {
		return "1 hour"
	}
	return fmt.Sprintf("%d hours", hours)
}
