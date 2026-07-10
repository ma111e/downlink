package downlinkclient

import (
	"github.com/ma111e/downlink/pkg/protos"
)

// ApplyProfiles reconciles the server's profiles to match a raw profiles.yml
// payload: entries are created/updated, and stored profiles absent from it are
// disabled (the default profile is exempt). The server parses the YAML, so the
// file schema stays defined in one place.
func (pc *DownlinkClient) ApplyProfiles(profilesYaml []byte, dryRun bool) (*protos.ApplyProfilesResponse, error) {
	return pc.profilesClient.ApplyProfiles(pc.ctx, &protos.ApplyProfilesRequest{
		ProfilesYaml: string(profilesYaml),
		DryRun:       dryRun,
	})
}

// ListProfiles returns the stored profiles ordered by sort order.
func (pc *DownlinkClient) ListProfiles() (*protos.ListProfilesResponse, error) {
	return pc.profilesClient.ListProfiles(pc.ctx, &protos.ListProfilesRequest{})
}
