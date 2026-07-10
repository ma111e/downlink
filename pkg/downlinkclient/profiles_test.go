package downlinkclient

import (
	"context"
	"testing"

	"github.com/ma111e/downlink/pkg/protos"

	"google.golang.org/grpc"
)

// stubProfiles is a minimal ProfilesServiceServer that records the last apply
// request and returns canned responses, so the client wrappers can be tested
// for request mapping and response passthrough.
type stubProfiles struct {
	protos.UnimplementedProfilesServiceServer

	lastApply *protos.ApplyProfilesRequest
	applyResp *protos.ApplyProfilesResponse
	listResp  *protos.ListProfilesResponse
}

func (s *stubProfiles) ApplyProfiles(_ context.Context, req *protos.ApplyProfilesRequest) (*protos.ApplyProfilesResponse, error) {
	s.lastApply = req
	return s.applyResp, nil
}

func (s *stubProfiles) ListProfiles(context.Context, *protos.ListProfilesRequest) (*protos.ListProfilesResponse, error) {
	return s.listResp, nil
}

func TestApplyProfilesClient(t *testing.T) {
	stub := &stubProfiles{applyResp: &protos.ApplyProfilesResponse{
		Created:  []string{"neteng"},
		Disabled: []string{"stale"},
		Warnings: []string{"neteng: unknown theme \"neon\"; the template default will be used"},
	}}
	client := dialStub(t, func(srv *grpc.Server) {
		protos.RegisterProfilesServiceServer(srv, stub)
	})

	yaml := []byte("profiles:\n  - slug: neteng\n")
	res, err := client.ApplyProfiles(yaml, true)
	if err != nil {
		t.Fatalf("ApplyProfiles: %v", err)
	}

	if stub.lastApply == nil || stub.lastApply.ProfilesYaml != string(yaml) || !stub.lastApply.DryRun {
		t.Errorf("request not mapped: %+v", stub.lastApply)
	}
	if len(res.Created) != 1 || res.Created[0] != "neteng" {
		t.Errorf("created = %v, want [neteng]", res.Created)
	}
	if len(res.Disabled) != 1 || len(res.Warnings) != 1 {
		t.Errorf("response not passed through: %+v", res)
	}
}

func TestListProfilesClient(t *testing.T) {
	stub := &stubProfiles{listResp: &protos.ListProfilesResponse{Profiles: []*protos.ProfileSummary{
		{Slug: "default", Name: "Default", Enabled: true, FeedCount: 3},
	}}}
	client := dialStub(t, func(srv *grpc.Server) {
		protos.RegisterProfilesServiceServer(srv, stub)
	})

	res, err := client.ListProfiles()
	if err != nil {
		t.Fatalf("ListProfiles: %v", err)
	}
	if len(res.Profiles) != 1 || res.Profiles[0].Slug != "default" || res.Profiles[0].FeedCount != 3 {
		t.Errorf("unexpected profiles: %+v", res.Profiles)
	}
}
