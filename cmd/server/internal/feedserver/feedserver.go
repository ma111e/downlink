package feedserver

import (
	"fmt"
	"html"
	"net/http"

	"github.com/ma111e/downlink/cmd/server/internal/store"
	"github.com/ma111e/downlink/pkg/models"
	"github.com/ma111e/downlink/pkg/utils"

	"github.com/gorilla/feeds"
	log "github.com/sirupsen/logrus"
)

// FeedServer handles HTTP requests for Atom feeds
type FeedServer struct {
	store   store.Store
	port    int
	baseURL string
}

// NewFeedServer creates a new feed server instance. baseURL, when set, is used
// to build absolute links for the served feeds (empty keeps links relative).
func NewFeedServer(store store.Store, port int, baseURL string) *FeedServer {
	return &FeedServer{
		store:   store,
		port:    port,
		baseURL: baseURL,
	}
}

// Start starts the HTTP server
func (fs *FeedServer) Start() error {
	http.HandleFunc("/feeds/", fs.handleFeedRequest)
	http.HandleFunc("/", fs.handleIndex)

	addr := fmt.Sprintf(":%d", fs.port)
	log.WithField("port", fs.port).Info("Starting Atom feed server")

	return http.ListenAndServe(addr, nil)
}

// handleIndex lists all available feeds
func (fs *FeedServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	availableFeeds, err := fs.store.ListFeeds()
	if err != nil {
		log.WithError(err).Error("Failed to list feeds")
		http.Error(w, "Failed to list feeds", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, "<html><head><title>Available Feeds</title></head><body>")
	fmt.Fprintf(w, "<h1>Available Atom Feeds</h1><ul>")

	for _, feed := range availableFeeds {
		normalizedName := utils.NormalizeFeedName(feed.Title)
		feedLink := utils.JoinURL(fs.baseURL, "feeds", normalizedName)
		fmt.Fprintf(w, "<li><a href=\"%s\">%s</a> - <a href=\"%s\">%s</a></li>",
			html.EscapeString(feedLink), html.EscapeString(feed.Title), html.EscapeString(feedLink), html.EscapeString(feed.URL))
	}

	fmt.Fprintf(w, "</ul></body></html>")
}

// handleFeedRequest handles requests for individual feed Atom feeds
func (fs *FeedServer) handleFeedRequest(w http.ResponseWriter, r *http.Request) {
	normalizedName := r.URL.Path[len("/feeds/"):]
	if normalizedName == "" {
		http.Error(w, "Feed name required", http.StatusBadRequest)
		return
	}

	// Get all feeds and find the one matching the normalized name
	availableFeeds, err := fs.store.ListFeeds()
	if err != nil {
		log.WithError(err).Error("Failed to list feeds")
		http.Error(w, "Failed to fetch feeds", http.StatusInternalServerError)
		return
	}

	var feed models.Feed
	var feedFound bool
	for _, f := range availableFeeds {
		if utils.NormalizeFeedName(f.Title) == normalizedName {
			feed = f
			feedFound = true
			break
		}
	}

	if !feedFound {
		log.WithField("normalized_name", normalizedName).Error("Feed not found")
		http.Error(w, "Feed not found", http.StatusNotFound)
		return
	}

	// Get articles for this specific feed
	feedArticles, err := fs.store.ListArticles(models.ArticleFilter{FeedId: feed.Id})
	if err != nil {
		log.WithError(err).Error("Failed to list articles")
		http.Error(w, "Failed to fetch articles", http.StatusInternalServerError)
		return
	}

	// Create Atom feed
	atomFeed := &feeds.Feed{
		Title:       feed.Title,
		Link:        &feeds.Link{Href: utils.JoinURL(fs.baseURL, "feeds", normalizedName)},
		Description: fmt.Sprintf("Articles from %s", feed.Title),
		Created:     feed.LastFetch,
	}

	// Add articles to feed
	for _, article := range feedArticles {
		item := &feeds.Item{
			Title:       article.Title,
			Link:        &feeds.Link{Href: utils.ResolveLink(fs.baseURL, article.Link)},
			Description: article.Content,
			Created:     article.PublishedAt,
			Id:          article.Id,
		}

		// Add hero image if available
		if article.HeroImage != "" {
			item.Enclosure = &feeds.Enclosure{
				Url:  article.HeroImage,
				Type: "image/jpeg",
			}
		}

		atomFeed.Items = append(atomFeed.Items, item)
	}

	// Generate Atom XML
	atom, err := atomFeed.ToAtom()
	if err != nil {
		log.WithError(err).Error("Failed to generate Atom feed")
		http.Error(w, "Failed to generate feed", http.StatusInternalServerError)
		return
	}

	// Set headers and write response
	w.Header().Set("Content-Type", "application/atom+xml; charset=utf-8")
	w.Write([]byte(atom))

	log.WithFields(log.Fields{
		"feed_id":         feed.Id,
		"feed_title":      feed.Title,
		"normalized_name": normalizedName,
		"article_count":   len(feedArticles),
	}).Info("Served Atom feed")
}
