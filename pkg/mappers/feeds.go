package mappers

import (
	"errors"
	"fmt"

	"downlink/pkg/models"
	"downlink/pkg/protos"

	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/datatypes"
)

func scraperToProto(scraper map[string]any) (map[string]*anypb.Any, error) {
	if len(scraper) == 0 {
		return nil, nil
	}

	out := make(map[string]*anypb.Any, len(scraper))
	for key, rawValue := range scraper {
		value, err := structpb.NewValue(rawValue)
		if err != nil {
			return nil, fmt.Errorf("scraper key %q: convert to protobuf value: %w", key, err)
		}
		anyValue, err := anypb.New(value)
		if err != nil {
			return nil, fmt.Errorf("scraper key %q: wrap protobuf value: %w", key, err)
		}
		out[key] = anyValue
	}
	return out, nil
}

func scraperFromProto(scraper map[string]*anypb.Any) (map[string]any, error) {
	if len(scraper) == 0 {
		return nil, nil
	}

	out := make(map[string]any, len(scraper))
	for key, anyValue := range scraper {
		if anyValue == nil {
			return nil, fmt.Errorf("scraper key %q: nil protobuf value", key)
		}

		var value structpb.Value
		if err := anyValue.UnmarshalTo(&value); err != nil {
			return nil, fmt.Errorf("scraper key %q: unwrap protobuf value: %w", key, err)
		}
		out[key] = value.AsInterface()
	}
	return out, nil
}

func FeedToProto(feed *models.Feed) (*protos.Feed, error) {
	if feed == nil {
		return nil, nil
	}

	protoFeed := &protos.Feed{
		Id:        feed.Id,
		Url:       feed.URL,
		Type:      feed.Type,
		Title:     feed.Title,
		LastFetch: timestamppb.New(feed.LastFetch),
	}

	// Handle pointer fields that might be nil
	if feed.Enabled != nil {
		protoFeed.Enabled = *feed.Enabled
	}

	if feed.GroupId != nil {
		protoFeed.GroupId = *feed.GroupId
	}

	scraper, err := scraperToProto(feed.Scraper)
	if err != nil {
		return nil, err
	}
	protoFeed.Scraper = scraper

	// Articles are intentionally not mapped (marked with json:"-" in model)

	return protoFeed, nil
}

func FeedToModel(feed *protos.Feed) (*models.Feed, error) {
	if feed == nil {
		return nil, nil
	}

	modelFeed := &models.Feed{
		Id:        feed.Id,
		URL:       feed.Url,
		Type:      feed.Type,
		Title:     feed.Title,
		LastFetch: feed.LastFetch.AsTime(),
	}

	// Handle pointer fields - using pointer values for GORM
	enabled := feed.Enabled
	modelFeed.Enabled = &enabled

	groupId := feed.GroupId
	modelFeed.GroupId = &groupId

	scraper, err := scraperFromProto(feed.Scraper)
	if err != nil {
		return nil, err
	}
	if scraper != nil {
		modelFeed.Scraper = datatypes.JSONMap(scraper)
	}

	// Articles are not mapped from proto

	return modelFeed, nil
}

func AllFeedsToProto(feeds []models.Feed) ([]*protos.Feed, error) {
	return mapValueSliceErr(feeds, FeedToProto)
}

func AllFeedsToModels(feeds []*protos.Feed) ([]models.Feed, error) {
	return mapPointerSliceErr(feeds, FeedToModel)
}

func FeedConfigToProto(config *models.FeedConfig) (*protos.FeedConfig, error) {
	if config == nil {
		return nil, nil
	}

	protoConfig := &protos.FeedConfig{
		Url:      config.URL,
		Title:    config.Title,
		Type:     config.Type,
		Enabled:  config.Enabled,
		Scraping: config.Scraping,
	}

	if config.Selectors != nil {
		protoConfig.Selectors = &protos.Selectors{
			Article:   config.Selectors.Article,
			Cutoff:    config.Selectors.Cutoff,
			Blacklist: config.Selectors.Blacklist,
		}
	}

	scraper, err := scraperToProto(config.Scraper)
	if err != nil {
		return nil, err
	}
	protoConfig.Scraper = scraper

	return protoConfig, nil
}

func FeedConfigToModel(config *protos.FeedConfig) (*models.FeedConfig, error) {
	if config == nil {
		return nil, nil
	}

	modelConfig := &models.FeedConfig{
		URL:      config.Url,
		Title:    config.Title,
		Type:     config.Type,
		Enabled:  config.Enabled,
		Scraping: config.Scraping,
	}

	if config.Selectors != nil {
		modelConfig.Selectors = &models.Selectors{
			Article:   config.Selectors.Article,
			Cutoff:    config.Selectors.Cutoff,
			Blacklist: config.Selectors.Blacklist,
		}
	}

	scraper, err := scraperFromProto(config.Scraper)
	if err != nil {
		return nil, err
	}
	modelConfig.Scraper = scraper

	return modelConfig, nil
}

func FeedItemToProto(item *models.FeedItem) *protos.FeedItem {
	if item == nil {
		return nil
	}

	return &protos.FeedItem{
		Id:          item.Id,
		Title:       item.Title,
		Content:     item.Content,
		Link:        item.Link,
		PublishedAt: timestamppb.New(item.PublishedAt),
		Tags:        item.Tags,
		Category:    item.Category,
		HeroImage:   item.HeroImage,
	}
}

func FeedItemToModel(item *protos.FeedItem) *models.FeedItem {
	if item == nil {
		return nil
	}

	return &models.FeedItem{
		Id:          item.Id,
		Title:       item.Title,
		Content:     item.Content,
		Link:        item.Link,
		PublishedAt: item.PublishedAt.AsTime(),
		Tags:        item.Tags,
		Category:    item.Category,
		HeroImage:   item.HeroImage,
	}
}

func AllFeedItemsToProto(items []models.FeedItem) []*protos.FeedItem {
	return mapValueSlice(items, FeedItemToProto)
}

func AllFeedItemsToModels(items []*protos.FeedItem) []models.FeedItem {
	return mapPointerSlice(items, FeedItemToModel)
}

func FeedResultToProto(result *models.FeedResult) (*protos.FeedResult, error) {
	if result == nil {
		return nil, nil
	}

	feed, err := FeedToProto(&result.Feed)
	if err != nil {
		return nil, err
	}

	protoResult := &protos.FeedResult{
		Feed: feed,
	}

	if len(result.Items) > 0 {
		protoResult.Items = AllFeedItemsToProto(result.Items)
	}

	if result.Error != nil {
		protoResult.Error = result.Error.Error()
	}

	return protoResult, nil
}

func FeedResultToModel(result *protos.FeedResult) (*models.FeedResult, error) {
	if result == nil {
		return nil, nil
	}

	feed, err := FeedToModel(result.Feed)
	if err != nil {
		return nil, err
	}

	modelResult := &models.FeedResult{
		Feed: *feed,
	}

	if len(result.Items) > 0 {
		modelResult.Items = AllFeedItemsToModels(result.Items)
	}

	if result.Error != "" {
		// Note: Cannot recreate the original error type, so using a simple error
		modelResult.Error = errors.New(result.Error)
	}

	return modelResult, nil
}
