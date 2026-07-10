package services

import (
	"context"
	"fmt"

	"github.com/ma111e/downlink/cmd/server/internal/manager"
	"github.com/ma111e/downlink/cmd/server/internal/store"
	"github.com/ma111e/downlink/pkg/models"
	"github.com/ma111e/downlink/pkg/protos"
	"gopkg.in/yaml.v3"
)

// ProfilesServer implements the ProfilesService gRPC service.
type ProfilesServer struct {
	protos.UnimplementedProfilesServiceServer
}

// NewProfilesServer creates a new Profiles server instance.
func NewProfilesServer() *ProfilesServer {
	return &ProfilesServer{}
}

// ApplyProfiles reconciles the stored profiles against the raw profiles.yml
// payload, the runtime counterpart of the startup apply: same YAML schema, same
// manager reconcile (upsert + feed-pool resolution + disable-absent).
func (s *ProfilesServer) ApplyProfiles(_ context.Context, req *protos.ApplyProfilesRequest) (*protos.ApplyProfilesResponse, error) {
	var pf models.ProfilesFile
	if err := yaml.Unmarshal([]byte(req.ProfilesYaml), &pf); err != nil {
		return nil, fmt.Errorf("failed to parse profiles YAML: %w", err)
	}

	res, err := manager.Manager.ApplyProfiles(&pf, req.DryRun)
	if err != nil {
		return nil, err
	}

	return &protos.ApplyProfilesResponse{
		Created:  res.Created,
		Updated:  res.Updated,
		Disabled: res.Disabled,
		Skipped:  res.Skipped,
		Warnings: res.Warnings,
	}, nil
}

// ListProfiles returns the stored profiles ordered by sort order.
func (s *ProfilesServer) ListProfiles(_ context.Context, _ *protos.ListProfilesRequest) (*protos.ListProfilesResponse, error) {
	profiles, err := store.Db.ListProfiles()
	if err != nil {
		return nil, err
	}

	out := make([]*protos.ProfileSummary, 0, len(profiles))
	for _, p := range profiles {
		enabled := p.Enabled == nil || *p.Enabled
		sortOrder := 0
		if p.SortOrder != nil {
			sortOrder = *p.SortOrder
		}
		out = append(out, &protos.ProfileSummary{
			Slug:         p.Id,
			Name:         p.Name,
			Description:  p.Description,
			Icon:         p.Icon,
			Enabled:      enabled,
			SortOrder:    int32(sortOrder),
			OutputSubdir: p.OutputSubdir,
			Layout:       p.Layout,
			Theme:        p.Theme,
			FeedCount:    int32(len(p.Feeds)),
		})
	}
	return &protos.ListProfilesResponse{Profiles: out}, nil
}
