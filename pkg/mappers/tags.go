package mappers

import (
	"downlink/pkg/models"
	"downlink/pkg/protos"
)

func TagsToProto(tag *models.Tag) *protos.Tag {
	protoTag := protos.Tag{
		Id:   tag.Id,
		Name: tag.Name,
	}

	return &protoTag
}

func TagsToModel(tag *protos.Tag) *models.Tag {
	modelsTag := models.Tag{
		Id:   tag.Id,
		Name: tag.Name,
	}

	return &modelsTag
}

func AllTagsToProto(tags []models.Tag) []*protos.Tag {
	var protoTags []*protos.Tag

	for _, tag := range tags {
		protoTags = append(protoTags, TagsToProto(&tag))
	}

	return protoTags
}

func AllTagsToModels(tags []*protos.Tag) []models.Tag {
	var modelsTags []models.Tag

	for _, tag := range tags {
		modelsTags = append(modelsTags, *TagsToModel(tag))
	}

	return modelsTags
}
