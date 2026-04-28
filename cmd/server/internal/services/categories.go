package services

import (
	"context"
	"fmt"
	"downlink/cmd/server/internal/store"
	"downlink/pkg/mappers"
	"downlink/pkg/models"
	"downlink/pkg/protos"

	log "github.com/sirupsen/logrus"
)

// CategoriesServer implements the CategoriesService gRPC service
type CategoriesServer struct {
	protos.UnimplementedCategoriesServiceServer
}

// NewCategoriesServer creates a new Categories server instance
func NewCategoriesServer() *CategoriesServer {
	return &CategoriesServer{}
}

// GetOrCreateCategory implements the CategoriesService gRPC service
func (s *CategoriesServer) GetOrCreateCategory(_ context.Context, req *protos.GetOrCreateCategoryRequest) (*protos.GetOrCreateCategoryResponse, error) {
	// Check if category exists by name
	categories, err := store.Db.GetCategories()
	if err != nil {
		return nil, fmt.Errorf("failed to get categories: %w", err)
	}

	// Look for an existing category with this name
	for _, cat := range categories {
		if cat.Name == req.Name {
			return &protos.GetOrCreateCategoryResponse{
				Category: mappers.CategoryToProto(&cat),
			}, nil
		}
	}

	// Category doesn't exist, create it
	newCategory := models.Category{
		Name:  req.Name,
		Color: "#808080",  // Default color
		Icon:  "category", // Default icon
	}

	// Save the new category
	err = store.Db.SaveCategory(newCategory)
	if err != nil {
		return nil, fmt.Errorf("failed to create category: %w", err)
	}

	log.WithFields(log.Fields{
		"name": newCategory.Name,
	}).Info("Created new category")

	return &protos.GetOrCreateCategoryResponse{
		Category: mappers.CategoryToProto(&newCategory),
	}, nil
}
