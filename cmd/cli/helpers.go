package main

import (
	"fmt"
	"strings"
	"time"

	"downlink/pkg/downlinkclient"
	"downlink/pkg/models"
	"downlink/pkg/utils"
)

func parseTimeWindow(from, to, between string, defaultFrom *time.Time) (*time.Time, *time.Time, error) {
	var fromTime, toTime *time.Time

	if between != "" {
		parts := strings.SplitN(between, ",", 2)
		if len(parts) != 2 {
			return nil, nil, fmt.Errorf("invalid --between format: must be two values separated by comma (e.g., '-7d,-1d')")
		}

		start, err := utils.ParseTimeString(strings.TrimSpace(parts[0]))
		if err != nil {
			return nil, nil, fmt.Errorf("invalid start time in --between: %w", err)
		}

		end, err := utils.ParseTimeString(strings.TrimSpace(parts[1]))
		if err != nil {
			return nil, nil, fmt.Errorf("invalid end time in --between: %w", err)
		}

		if start.After(end) {
			return nil, nil, fmt.Errorf("invalid --between: start time must be before end time")
		}

		return &start, &end, nil
	}

	if from != "" {
		parsed, err := utils.ParseTimeString(from)
		if err != nil {
			return nil, nil, fmt.Errorf("error parsing --from: %w", err)
		}
		fromTime = &parsed
	} else if defaultFrom != nil {
		value := *defaultFrom
		fromTime = &value
	}

	if to != "" {
		parsed, err := utils.ParseTimeString(to)
		if err != nil {
			return nil, nil, fmt.Errorf("error parsing --to: %w", err)
		}
		toTime = &parsed
	}

	if fromTime != nil && toTime != nil && toTime.Before(*fromTime) {
		return nil, nil, fmt.Errorf("error: --to (%v) cannot be before --from (%v)", toTime, fromTime)
	}

	return fromTime, toTime, nil
}

func findFeedByIDOrNormalizedName(client *downlinkclient.DownlinkClient, input string) (models.Feed, []models.Feed, error) {
	feeds, err := client.ListFeeds()
	if err != nil {
		return models.Feed{}, nil, err
	}

	for _, feed := range feeds {
		if feed.Id == input {
			return feed, feeds, nil
		}
	}

	normalizedInput := utils.NormalizeFeedName(input)
	for _, feed := range feeds {
		if utils.NormalizeFeedName(feed.Title) == normalizedInput {
			return feed, feeds, nil
		}
	}

	return models.Feed{}, feeds, fmt.Errorf("feed not found: %s", input)
}

func printAvailableFeeds(feeds []models.Feed) {
	fmt.Println("\nAvailable feeds:")
	for _, feed := range feeds {
		fmt.Printf("  - %s (ID: %s, normalized: %s)\n", feed.Title, feed.Id, utils.NormalizeFeedName(feed.Title))
	}
}
