package codexauth

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"
)

// jwtClaims holds the fields we care about from a JWT payload.
type jwtClaims struct {
	Exp              int64                  `json:"exp"`
	Email            string                 `json:"email"`
	PreferredUsername string                `json:"preferred_username"`
	UPN              string                 `json:"upn"`
	OpenAIAuth       map[string]interface{} `json:"https://api.openai.com/auth"`
}

// decodeJWTPayload base64url-decodes the payload segment of a JWT and parses
// it as JSON. It does NOT verify the signature — result is for display/expiry
// checks only.
func decodeJWTPayload(token string) (*jwtClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil, ErrInvalidJWT
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}
	var claims jwtClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, err
	}
	return &claims, nil
}

// ExpiresWithin returns true when the access token's exp claim falls within
// skew of now. Returns false (not-expiring) when exp is absent or the token
// cannot be decoded, so callers never panic on a bad token.
func ExpiresWithin(token string, skew time.Duration) bool {
	claims, err := decodeJWTPayload(token)
	if err != nil || claims.Exp == 0 {
		return false
	}
	expiry := time.Unix(claims.Exp, 0)
	return time.Now().Add(skew).After(expiry)
}

// LabelFromJWT extracts a human-readable label from the token payload.
func LabelFromJWT(token string, fallback string) string {
	claims, err := decodeJWTPayload(token)
	if err != nil {
		return fallback
	}
	if claims.Email != "" {
		return claims.Email
	}
	if claims.PreferredUsername != "" {
		return claims.PreferredUsername
	}
	if claims.UPN != "" {
		return claims.UPN
	}
	return fallback
}

// ChatGPTAccountID extracts the chatgpt_account_id from the nested auth claim.
func ChatGPTAccountID(token string) string {
	claims, err := decodeJWTPayload(token)
	if err != nil || claims.OpenAIAuth == nil {
		return ""
	}
	id, _ := claims.OpenAIAuth["chatgpt_account_id"].(string)
	return id
}
