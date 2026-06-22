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

// FeedServer serves Atom feeds, organized by profile. Everything is reached
// through a profile: `/` lists profiles, `/<slug>/` lists that profile's feeds,
// and `/<slug>/feeds/<name>` is a feed's Atom. The default profile is just a
// profile (`/default/...`); there is no special global route.
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

// Start starts the HTTP server with profile-scoped routes.
func (fs *FeedServer) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", fs.handleProfileIndex)
	mux.HandleFunc("GET /{profile}/{$}", fs.handleProfileFeeds)
	mux.HandleFunc("GET /{profile}/feeds/{name}", fs.handleProfileFeed)

	addr := fmt.Sprintf(":%d", fs.port)
	log.WithField("port", fs.port).Info("Starting Atom feed server")
	return http.ListenAndServe(addr, mux)
}

// handleProfileIndex lists the available profiles, each linking to its feed list.
func (fs *FeedServer) handleProfileIndex(w http.ResponseWriter, _ *http.Request) {
	profiles, err := fs.store.ListProfiles()
	if err != nil {
		log.WithError(err).Error("Failed to list profiles")
		http.Error(w, "Failed to list profiles", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, "<html><head><title>Profiles</title></head><body>")
	fmt.Fprintf(w, "<h1>Profiles</h1><ul>")
	for _, p := range profiles {
		if p.Enabled != nil && !*p.Enabled {
			continue
		}
		link := utils.JoinURL(fs.baseURL, p.Id) + "/"
		name := p.Name
		if name == "" {
			name = p.Id
		}
		fmt.Fprintf(w, "<li><a href=\"%s\">%s</a></li>", html.EscapeString(link), html.EscapeString(name))
	}
	fmt.Fprintf(w, "</ul></body></html>")
}

// handleProfileFeeds lists the feeds in one profile's pool.
func (fs *FeedServer) handleProfileFeeds(w http.ResponseWriter, r *http.Request) {
	profileID := r.PathValue("profile")
	feedList, err := fs.store.ListProfileFeeds(profileID)
	if err != nil {
		log.WithError(err).WithField("profile", profileID).Error("Failed to list profile feeds")
		http.Error(w, "Failed to list feeds", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, "<html><head><title>%s feeds</title></head><body>", html.EscapeString(profileID))
	fmt.Fprintf(w, "<h1>Atom feeds for %s</h1><ul>", html.EscapeString(profileID))
	for _, feed := range feedList {
		normalizedName := utils.NormalizeFeedName(feed.Title)
		feedLink := utils.JoinURL(fs.baseURL, profileID, "feeds", normalizedName)
		fmt.Fprintf(w, "<li><a href=\"%s\">%s</a> - <a href=\"%s\">%s</a></li>",
			html.EscapeString(feedLink), html.EscapeString(feed.Title), html.EscapeString(feedLink), html.EscapeString(feed.URL))
	}
	fmt.Fprintf(w, "</ul></body></html>")
}

// handleProfileFeed serves the Atom feed for one feed within a profile. Articles
// are global; the profile scopes which feeds are discoverable.
func (fs *FeedServer) handleProfileFeed(w http.ResponseWriter, r *http.Request) {
	profileID := r.PathValue("profile")
	normalizedName := r.PathValue("name")

	profileFeeds, err := fs.store.ListProfileFeeds(profileID)
	if err != nil {
		log.WithError(err).WithField("profile", profileID).Error("Failed to list profile feeds")
		http.Error(w, "Failed to fetch feeds", http.StatusInternalServerError)
		return
	}

	var feed models.Feed
	var feedFound bool
	for _, f := range profileFeeds {
		if utils.NormalizeFeedName(f.Title) == normalizedName {
			feed = f
			feedFound = true
			break
		}
	}
	if !feedFound {
		log.WithFields(log.Fields{"profile": profileID, "normalized_name": normalizedName}).Error("Feed not found in profile")
		http.Error(w, "Feed not found", http.StatusNotFound)
		return
	}

	atom, err := fs.buildFeedAtom(feed, utils.JoinURL(fs.baseURL, profileID, "feeds", normalizedName))
	if err != nil {
		log.WithError(err).Error("Failed to generate Atom feed")
		http.Error(w, "Failed to generate feed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/atom+xml; charset=utf-8")
	_, _ = w.Write([]byte(atom))

	log.WithFields(log.Fields{
		"profile":         profileID,
		"feed_id":         feed.Id,
		"feed_title":      feed.Title,
		"normalized_name": normalizedName,
	}).Info("Served Atom feed")
}

// buildFeedAtom renders a feed's articles as an Atom document. selfLink is the
// feed's own URL (absolute when a base URL is configured).
func (fs *FeedServer) buildFeedAtom(feed models.Feed, selfLink string) (string, error) {
	feedArticles, err := fs.store.ListArticles(models.ArticleFilter{FeedId: feed.Id})
	if err != nil {
		return "", fmt.Errorf("list articles: %w", err)
	}

	atomFeed := &feeds.Feed{
		Title:       feed.Title,
		Link:        &feeds.Link{Href: selfLink},
		Description: fmt.Sprintf("Articles from %s", feed.Title),
		Created:     feed.LastFetch,
	}
	for _, article := range feedArticles {
		item := &feeds.Item{
			Title:       article.Title,
			Link:        &feeds.Link{Href: utils.ResolveLink(fs.baseURL, article.Link)},
			Description: article.Content,
			Created:     article.PublishedAt,
			Id:          article.Id,
		}
		if article.HeroImage != "" {
			item.Enclosure = &feeds.Enclosure{Url: article.HeroImage, Type: "image/jpeg"}
		}
		atomFeed.Items = append(atomFeed.Items, item)
	}
	return atomFeed.ToAtom()
}
