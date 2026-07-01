package downlinkclient

import (
	"context"
	"testing"

	"github.com/ma111e/downlink/pkg/protos"

	"google.golang.org/grpc"
)

type stubLLMs struct {
	protos.UnimplementedLLMsServiceServer

	lastResolve *protos.ResolveLLMRequest

	providersResp *protos.GetLLMProvidersResponse
	resolveResp   *protos.ResolveLLMResponse
}

func (s *stubLLMs) GetLLMProviders(_ context.Context, _ *protos.GetLLMProvidersRequest) (*protos.GetLLMProvidersResponse, error) {
	return s.providersResp, nil
}
func (s *stubLLMs) ResolveLLM(_ context.Context, r *protos.ResolveLLMRequest) (*protos.ResolveLLMResponse, error) {
	s.lastResolve = r
	return s.resolveResp, nil
}

func llmsClient(t *testing.T, stub protos.LLMsServiceServer) *DownlinkClient {
	return dialStub(t, func(s *grpc.Server) { protos.RegisterLLMsServiceServer(s, stub) })
}

func TestGetLLMProvidersMapsToModels(t *testing.T) {
	stub := &stubLLMs{providersResp: &protos.GetLLMProvidersResponse{
		Providers: []*protos.ProviderConfig{
			{Name: "openai-main", ProviderType: "openai", ModelName: "gpt-4"},
			{Name: "ollama", ProviderType: "ollama"},
		},
	}}
	pc := llmsClient(t, stub)

	got, err := pc.GetLLMProviders()
	if err != nil {
		t.Fatalf("GetLLMProviders() error = %v", err)
	}
	if len(got) != 2 || got[0].Name != "openai-main" || got[0].ModelName != "gpt-4" || got[1].ProviderType != "ollama" {
		t.Fatalf("providers = %+v, want two mapped configs", got)
	}
}

func TestResolveLLMMapsRequestAndResponse(t *testing.T) {
	stub := &stubLLMs{resolveResp: &protos.ResolveLLMResponse{
		ProviderType: "openai",
		ModelName:    "gpt-4o",
	}}
	pc := llmsClient(t, stub)

	pt, mn, err := pc.ResolveLLM("my-openai", "gpt-4o")
	if err != nil {
		t.Fatalf("ResolveLLM() error = %v", err)
	}
	if stub.lastResolve == nil || stub.lastResolve.Provider != "my-openai" || stub.lastResolve.Model != "gpt-4o" {
		t.Fatalf("received req = %+v, want provider/model forwarded", stub.lastResolve)
	}
	if pt != "openai" || mn != "gpt-4o" {
		t.Fatalf("resolved = (%q,%q), want (openai, gpt-4o)", pt, mn)
	}
}
