package downlinkclient

import (
	"github.com/ma111e/downlink/pkg/protos"
)

func (pc *DownlinkClient) StartCodexLogin(providerName, modelName string) (*protos.StartCodexLoginResponse, error) {
	return pc.credsClient.StartCodexLogin(pc.ctx, &protos.StartCodexLoginRequest{
		ProviderName: providerName,
		ModelName:    modelName,
	})
}

func (pc *DownlinkClient) PollCodexLogin(sessionID string) (*protos.PollCodexLoginResponse, error) {
	return pc.credsClient.PollCodexLogin(pc.ctx, &protos.PollCodexLoginRequest{
		SessionId: sessionID,
	})
}

func (pc *DownlinkClient) StartClaudeLogin(providerName, modelName string) (*protos.StartClaudeLoginResponse, error) {
	return pc.credsClient.StartClaudeLogin(pc.ctx, &protos.StartClaudeLoginRequest{
		ProviderName: providerName,
		ModelName:    modelName,
	})
}

func (pc *DownlinkClient) CompleteClaudeLogin(sessionID, code string) (*protos.CompleteClaudeLoginResponse, error) {
	return pc.credsClient.CompleteClaudeLogin(pc.ctx, &protos.CompleteClaudeLoginRequest{
		SessionId: sessionID,
		Code:      code,
	})
}

func (pc *DownlinkClient) ListCodexCredentials(providerName string) (*protos.ListCodexCredentialsResponse, error) {
	return pc.credsClient.ListCodexCredentials(pc.ctx, &protos.ListCodexCredentialsRequest{
		ProviderName: providerName,
	})
}

func (pc *DownlinkClient) RemoveCodexCredential(providerName, credentialID string) (*protos.RemoveCodexCredentialResponse, error) {
	return pc.credsClient.RemoveCodexCredential(pc.ctx, &protos.RemoveCodexCredentialRequest{
		ProviderName: providerName,
		CredentialId: credentialID,
	})
}

func (pc *DownlinkClient) SetCodexCredentialPriority(providerName, credentialID string, priority int32) (*protos.SetCodexCredentialPriorityResponse, error) {
	return pc.credsClient.SetCodexCredentialPriority(pc.ctx, &protos.SetCodexCredentialPriorityRequest{
		ProviderName: providerName,
		CredentialId: credentialID,
		Priority:     priority,
	})
}
