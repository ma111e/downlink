package mappers

import (
	"downlink/pkg/models"
	"downlink/pkg/protos"
)

func FeedGroupToProto(group *models.FeedGroup) (*protos.FeedGroup, error) {
	if group == nil {
		return nil, nil
	}

	protoGroup := &protos.FeedGroup{
		Id:   group.Id,
		Name: group.Name,
		Icon: group.Icon,
	}

	// Handle pointer fields that might be nil
	if group.SortOrder != nil {
		protoGroup.SortOrder = int32(*group.SortOrder)
	}

	// Convert feeds if needed
	if len(group.Feeds) > 0 {
		feeds, err := AllFeedsToProto(group.Feeds)
		if err != nil {
			return nil, err
		}
		protoGroup.Feeds = feeds
	}

	return protoGroup, nil
}

func FeedGroupToModel(group *protos.FeedGroup) (*models.FeedGroup, error) {
	if group == nil {
		return nil, nil
	}

	// GORM needs pointer types for zero values
	sortOrder := int(group.SortOrder)

	modelGroup := &models.FeedGroup{
		Id:        group.Id,
		Name:      group.Name,
		Icon:      group.Icon,
		SortOrder: &sortOrder,
	}

	// Convert feeds if needed
	if len(group.Feeds) > 0 {
		feeds, err := AllFeedsToModels(group.Feeds)
		if err != nil {
			return nil, err
		}
		modelGroup.Feeds = feeds
	}

	return modelGroup, nil
}

func AllFeedGroupsToProto(groups []models.FeedGroup) ([]*protos.FeedGroup, error) {
	return mapValueSliceErr(groups, FeedGroupToProto)
}

func AllFeedGroupsToModels(groups []*protos.FeedGroup) ([]models.FeedGroup, error) {
	return mapPointerSliceErr(groups, FeedGroupToModel)
}
