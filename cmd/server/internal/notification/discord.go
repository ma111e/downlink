package notification

import (
	"bytes"
	"downlink/pkg/models"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	log "github.com/sirupsen/logrus"
)

const discordExecutiveOverviewMaxRunes = 1200

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
	description := executiveOverviewBrief(digest.DigestSummary)
	if description == "" {
		description = fmt.Sprintf("Digest covering the last %s.", formatDuration(digest.TimeWindow))
	}

	mainEmbed := DiscordEmbed{
		Title:       "📰 DOWNLINK Digest",
		Description: truncateString(description, discordExecutiveOverviewMaxRunes),
		Color:       0x5865F2, // Discord blurple
		Timestamp:   digest.CreatedAt.Format(time.RFC3339),
		Footer: &DiscordEmbedFooter{
			Text: fmt.Sprintf("DOWNLINK • %s window", formatDuration(digest.TimeWindow)),
		},
	}

	articleCount := 0
	if digest.ArticleCount != nil {
		articleCount = *digest.ArticleCount
	}
	mainEmbed.Fields = append(mainEmbed.Fields, DiscordEmbedField{
		Name:   "📄 Articles",
		Value:  fmt.Sprintf("%d", articleCount),
		Inline: true,
	})

	mainEmbed.Fields = append(mainEmbed.Fields, DiscordEmbedField{
		Name:   "🤖 Providers",
		Value:  fmt.Sprintf("%d", len(digest.ProviderResults)),
		Inline: true,
	})

	return []DiscordEmbed{mainEmbed}
}

func executiveOverviewBrief(summary string) string {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return ""
	}

	lines := strings.Split(summary, "\n")
	type section struct {
		title string
		lines []string
	}

	sections := []section{{}}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			heading := strings.TrimSpace(strings.TrimPrefix(trimmed, "## "))
			sections = append(sections, section{title: heading})
			continue
		}
		sections[len(sections)-1].lines = append(sections[len(sections)-1].lines, line)
	}

	for _, section := range sections {
		if strings.EqualFold(section.title, "Executive Overview") {
			brief := strings.TrimSpace(strings.Join(section.lines, "\n"))
			if brief != "" {
				return truncateString(brief, discordExecutiveOverviewMaxRunes)
			}
		}
	}
	for _, section := range sections {
		brief := strings.TrimSpace(strings.Join(section.lines, "\n"))
		if brief != "" {
			return truncateString(brief, discordExecutiveOverviewMaxRunes)
		}
	}
	return ""
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
