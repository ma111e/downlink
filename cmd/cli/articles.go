package main

import (
	"downlink/pkg/models"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

var (
	// Filter flags for article listing
	unreadOnly     bool
	bookmarkedOnly bool
	categoryName   string
	cliStartDate   string
	cliEndDate     string
	cliBetween     string
)

// Article commands
func createArticleCommands() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "article",
		Short: "Manage articles",
		Long:  `View, filter, and update articles in the feed reader.`,
	}

	// List articles command
	listCmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List articles",
		Long:    `List articles with optional filtering by date, category, read status, and bookmarks.`,
		Run: func(cmd *cobra.Command, args []string) {
			client := getNewDownlinkClient()

			startDate, endDate, err := parseTimeWindow(cliStartDate, cliEndDate, cliBetween, nil)
			if err != nil {
				fmt.Println(err)
				return
			}
			filter := models.ArticleFilter{
				UnreadOnly:     unreadOnly,
				BookmarkedOnly: bookmarkedOnly,
				CategoryName:   categoryName,
				StartDate:      startDate,
				EndDate:        endDate,
			}

			articles, err := client.ListArticles(filter)
			if err != nil {
				fmt.Printf("Failed to list articles: %v\n", err)
				return
			}

			if len(articles) == 0 {
				fmt.Println("No articles found matching your criteria.")
				return
			}

			// Format and display results
			if jsonOutput {
				out, err := json.MarshalIndent(articles, "", "  ")
				if err != nil {
					fmt.Printf("Error marshalling to JSON: %v\n", err)
					return
				}
				fmt.Println(string(out))
			} else {
				printArticleTable(articles)
			}
		},
	}

	// Add flags to list command
	listCmd.Flags().BoolVar(&unreadOnly, "unread", false, "Show only unread articles")
	listCmd.Flags().BoolVar(&bookmarkedOnly, "bookmarked", false, "Show only bookmarked articles")
	listCmd.Flags().StringVar(&categoryName, "category", "", "Filter by category name")
	listCmd.Flags().StringVar(&cliStartDate, "from", "", "Start of time window (e.g., 'now', '2025-01-01', '-7d')")
	listCmd.Flags().StringVar(&cliEndDate, "to", "", "End of time window (e.g., 'now', '2025-01-01', '-1h')")
	listCmd.Flags().StringVar(&cliBetween, "between", "", "Filter articles between two dates/durations (e.g., '-7d,-1d', '2025-01-01,2025-01-07')")

	// Get article command
	getCmd := &cobra.Command{
		Use:   "get [id]",
		Short: "Get article details",
		Long:  `Retrieve detailed information about a specific article by its ID.`,
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			client := getNewDownlinkClient()

			articleId := args[0]
			article, err := client.GetArticle(articleId)
			if err != nil {
				fmt.Printf("Failed to get article: %v\n", err)
				return
			}

			if jsonOutput {
				out, err := json.MarshalIndent(article, "", "  ")
				if err != nil {
					fmt.Printf("Error marshalling to JSON: %v\n", err)
					return
				}
				fmt.Println(string(out))
			} else {
				printArticleDetail(article)
			}
		},
	}

	// Update article command
	updateCmd := &cobra.Command{
		Use:   "update [id]",
		Short: "Update article properties",
		Long:  `Mark articles as read/unread or bookmarked/unbookmarked.`,
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			client := getNewDownlinkClient()

			articleId := args[0]

			// Get flags for updating
			markRead, _ := cmd.Flags().GetBool("read")
			markUnread, _ := cmd.Flags().GetBool("unread")
			bookmark, _ := cmd.Flags().GetBool("bookmark")
			unbookmark, _ := cmd.Flags().GetBool("unbookmark")

			// Check for conflicting flags
			if markRead && markUnread {
				fmt.Println("Error: Cannot mark as both read and unread")
				return
			}

			if bookmark && unbookmark {
				fmt.Println("Error: Cannot both bookmark and unbookmark")
				return
			}

			// Prepare update
			update := models.ArticleUpdate{}

			if markRead {
				readValue := true
				update.Read = &readValue
			} else if markUnread {
				unreadValue := false
				update.Read = &unreadValue
			}

			if bookmark {
				bookmarkValue := true
				update.Bookmarked = &bookmarkValue
			} else if unbookmark {
				unbookmarkValue := false
				update.Bookmarked = &unbookmarkValue
			}

			_, err := client.UpdateArticle(articleId, &update)
			if err != nil {
				fmt.Printf("Failed to update article: %v\n", err)
				return
			}

			fmt.Printf("Article %s updated successfully\n", articleId)
		},
	}

	// Add flags for update command
	updateCmd.Flags().Bool("read", false, "Mark article as read")
	updateCmd.Flags().Bool("unread", false, "Mark article as unread")
	updateCmd.Flags().Bool("bookmark", false, "Bookmark article")
	updateCmd.Flags().Bool("unbookmark", false, "Remove bookmark from article")

	cmd.AddCommand(listCmd, getCmd, updateCmd)
	return cmd
}
