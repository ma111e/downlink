package downlinkclient

import (
	"context"
	"testing"

	"github.com/ma111e/downlink/pkg/protos"

	"google.golang.org/grpc"
)

type stubCreds struct {
	protos.UnimplementedCredsServiceServer

	lastPoll   *protos.PollCodexLoginRequest
	lastList   *protos.ListCodexCredentialsRequest
	lastRemove *protos.RemoveCodexCredentialRequest

	pollResp   *protos.PollCodexLoginResponse
	listResp   *protos.ListCodexCredentialsResponse
	removeResp *protos.RemoveCodexCredentialResponse
}

func (s *stubCreds) PollCodexLogin(_ context.Context, r *protos.PollCodexLoginRequest) (*protos.PollCodexLoginResponse, error) {
	s.lastPoll = r
	return s.pollResp, nil
}
func (s *stubCreds) ListCodexCredentials(_ context.Context, r *protos.ListCodexCredentialsRequest) (*protos.ListCodexCredentialsResponse, error) {
	s.lastList = r
	return s.listResp, nil
}
func (s *stubCreds) RemoveCodexCredential(_ context.Context, r *protos.RemoveCodexCredentialRequest) (*protos.RemoveCodexCredentialResponse, error) {
	s.lastRemove = r
	return s.removeResp, nil
}

func credsClient(t *testing.T, stub protos.CredsServiceServer) *DownlinkClient {
	return dialStub(t, func(s *grpc.Server) { protos.RegisterCredsServiceServer(s, stub) })
}

func TestPollCodexLoginForwardsSessionAndReturnsStatus(t *testing.T) {
	stub := &stubCreds{pollResp: &protos.PollCodexLoginResponse{
		Status: "approved", CredentialId: "c1", Label: "main",
	}}
	pc := credsClient(t, stub)

	resp, err := pc.PollCodexLogin("sess-1")
	if err != nil {
		t.Fatalf("PollCodexLogin() error = %v", err)
	}
	if stub.lastPoll == nil || stub.lastPoll.SessionId != "sess-1" {
		t.Fatalf("received req = %+v, want SessionId sess-1", stub.lastPoll)
	}
	if resp.Status != "approved" || resp.CredentialId != "c1" || resp.Label != "main" {
		t.Fatalf("resp = %+v, want approved/c1/main passed through", resp)
	}
}

func TestListCodexCredentialsForwardsProvider(t *testing.T) {
	stub := &stubCreds{listResp: &protos.ListCodexCredentialsResponse{
		Credentials: []*protos.CodexCredentialInfo{{Id: "c1", Label: "main"}},
	}}
	pc := credsClient(t, stub)

	resp, err := pc.ListCodexCredentials("codex-sub")
	if err != nil {
		t.Fatalf("ListCodexCredentials() error = %v", err)
	}
	if stub.lastList == nil || stub.lastList.ProviderName != "codex-sub" {
		t.Fatalf("received req = %+v, want ProviderName codex-sub", stub.lastList)
	}
	if len(resp.Credentials) != 1 || resp.Credentials[0].Id != "c1" {
		t.Fatalf("resp = %+v, want one credential c1", resp)
	}
}

func TestRemoveCodexCredentialForwardsArgs(t *testing.T) {
	stub := &stubCreds{removeResp: &protos.RemoveCodexCredentialResponse{Removed: true}}
	pc := credsClient(t, stub)

	resp, err := pc.RemoveCodexCredential("codex-sub", "c1")
	if err != nil {
		t.Fatalf("RemoveCodexCredential() error = %v", err)
	}
	if stub.lastRemove == nil || stub.lastRemove.ProviderName != "codex-sub" || stub.lastRemove.CredentialId != "c1" {
		t.Fatalf("received req = %+v, want provider/credential forwarded", stub.lastRemove)
	}
	if !resp.Removed {
		t.Fatal("Removed = false, want the server's true passed through")
	}
}
