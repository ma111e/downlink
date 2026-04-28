package mappers

import (
	"downlink/pkg/models"
	"downlink/pkg/protos"
)

func CategoryToProto(category *models.Category) *protos.Category {
	protoCategory := protos.Category{
		Name:  category.Name,
		Color: category.Color,
		Icon:  category.Icon,
	}

	return &protoCategory
}

func CategoryToModel(category *protos.Category) *models.Category {
	modelsCategory := models.Category{
		Name:  category.Name,
		Color: category.Color,
		Icon:  category.Icon,
	}

	return &modelsCategory
}

func AllCategoriesToProto(categories []models.Category) []*protos.Category {
	var protoCategories []*protos.Category

	for _, category := range categories {
		protoCategories = append(protoCategories, CategoryToProto(&category))
	}

	return protoCategories
}

func AllCategoriesToModels(categories []*protos.Category) []models.Category {
	var modelsCategories []models.Category

	for _, category := range categories {
		modelsCategories = append(modelsCategories, *CategoryToModel(category))
	}

	return modelsCategories
}
