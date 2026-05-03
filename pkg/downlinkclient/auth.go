package downlinkclient

import (
	"downlink/pkg/protos"
)

func (pc *DownlinkClient) StartCodexLogin(providerName, modelName string) (*protos.StartCodexLoginResponse, error) {
	return pc.authClient.StartCodexLogin(pc.ctx, &protos.StartCodexLoginRequest{
		ProviderName: providerName,
		ModelName:    modelName,
	})
}

func (pc *DownlinkClient) PollCodexLogin(sessionID string) (*protos.PollCodexLoginResponse, error) {
	return pc.authClient.PollCodexLogin(pc.ctx, &protos.PollCodexLoginRequest{
		SessionId: sessionID,
	})
}

func (pc *DownlinkClient) ListCodexCredentials(providerName string) (*protos.ListCodexCredentialsResponse, error) {
	return pc.authClient.ListCodexCredentials(pc.ctx, &protos.ListCodexCredentialsRequest{
		ProviderName: providerName,
	})
}

func (pc *DownlinkClient) RemoveCodexCredential(providerName, credentialID string) (*protos.RemoveCodexCredentialResponse, error) {
	return pc.authClient.RemoveCodexCredential(pc.ctx, &protos.RemoveCodexCredentialRequest{
		ProviderName: providerName,
		CredentialId: credentialID,
	})
}

func (pc *DownlinkClient) SetCodexCredentialPriority(providerName, credentialID string, priority int32) (*protos.SetCodexCredentialPriorityResponse, error) {
	return pc.authClient.SetCodexCredentialPriority(pc.ctx, &protos.SetCodexCredentialPriorityRequest{
		ProviderName: providerName,
		CredentialId: credentialID,
		Priority:     priority,
	})
}
