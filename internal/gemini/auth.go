package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type oauthCredentials struct {
	AccessToken  string `json:"access_token"`
	Scope        string `json:"scope"`
	TokenType    string `json:"token_type"`
	IDToken      string `json:"id_token"`
	ExpiryDateMS int64  `json:"expiry_date"`
	RefreshToken string `json:"refresh_token"`
}

type oauthRefreshResponse struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn    int64  `json:"expires_in"`
	Scope        string `json:"scope"`
	TokenType    string `json:"token_type"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	Error        string `json:"error"`
	Description  string `json:"error_description"`
}

func (b *Backend) accessToken(ctx context.Context) (string, error) {
	creds, err := b.loadCredentials()
	if err != nil {
		return "", err
	}

	if strings.TrimSpace(creds.AccessToken) != "" && !credentialsExpired(creds) {
		return strings.TrimSpace(creds.AccessToken), nil
	}
	if strings.TrimSpace(creds.RefreshToken) == "" {
		return "", fmt.Errorf("gemini oauth credentials are expired and contain no refresh token")
	}

	refreshed, err := b.refreshAccessToken(ctx, creds.RefreshToken)
	if err != nil {
		return "", err
	}
	if refreshed.RefreshToken == "" {
		refreshed.RefreshToken = creds.RefreshToken
	}
	if refreshed.Scope == "" {
		refreshed.Scope = creds.Scope
	}
	if refreshed.IDToken == "" {
		refreshed.IDToken = creds.IDToken
	}
	if err := b.saveCredentials(refreshed); err != nil {
		return "", err
	}

	return strings.TrimSpace(refreshed.AccessToken), nil
}

func (b *Backend) loadCredentials() (oauthCredentials, error) {
	if strings.TrimSpace(b.credsPath) == "" {
		return oauthCredentials{}, fmt.Errorf("could not determine ~/.gemini credential path")
	}

	data, err := os.ReadFile(filepath.Clean(b.credsPath))
	if err != nil {
		return oauthCredentials{}, fmt.Errorf("read gemini oauth credentials: %w", err)
	}

	var creds oauthCredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return oauthCredentials{}, fmt.Errorf("parse gemini oauth credentials: %w", err)
	}
	if strings.TrimSpace(creds.AccessToken) == "" && strings.TrimSpace(creds.RefreshToken) == "" {
		return oauthCredentials{}, fmt.Errorf("gemini oauth credentials are empty")
	}

	return creds, nil
}

func (b *Backend) saveCredentials(creds oauthCredentials) error {
	body, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal gemini oauth credentials: %w", err)
	}
	if err := os.WriteFile(filepath.Clean(b.credsPath), body, 0o600); err != nil {
		return fmt.Errorf("write gemini oauth credentials: %w", err)
	}
	return nil
}

func credentialsExpired(creds oauthCredentials) bool {
	if creds.ExpiryDateMS == 0 {
		return true
	}
	expiry := time.UnixMilli(creds.ExpiryDateMS)
	return time.Now().Add(30 * time.Second).After(expiry)
}

func (b *Backend) refreshAccessToken(ctx context.Context, refreshToken string) (oauthCredentials, error) {
	values := url.Values{}
	values.Set("client_id", oauthClientID)
	values.Set("client_secret", oauthClientSecret)
	values.Set("grant_type", "refresh_token")
	values.Set("refresh_token", refreshToken)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.tokenURL, strings.NewReader(values.Encode()))
	if err != nil {
		return oauthCredentials{}, fmt.Errorf("build gemini oauth refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return oauthCredentials{}, fmt.Errorf("refresh gemini oauth token: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return oauthCredentials{}, fmt.Errorf("read gemini oauth refresh response: %w", err)
	}

	var parsed oauthRefreshResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return oauthCredentials{}, fmt.Errorf("parse gemini oauth refresh response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if parsed.Error != "" {
			return oauthCredentials{}, fmt.Errorf("gemini oauth refresh failed: %s (%s)", parsed.Error, parsed.Description)
		}
		return oauthCredentials{}, fmt.Errorf("gemini oauth refresh failed: %s", resp.Status)
	}
	if strings.TrimSpace(parsed.AccessToken) == "" {
		return oauthCredentials{}, fmt.Errorf("gemini oauth refresh returned no access token")
	}

	expiryDateMS := time.Now().Add(time.Duration(parsed.ExpiresIn) * time.Second).UnixMilli()
	if parsed.ExpiresIn == 0 {
		expiryDateMS = time.Now().Add(1 * time.Hour).UnixMilli()
	}

	return oauthCredentials{
		AccessToken:  parsed.AccessToken,
		Scope:        parsed.Scope,
		TokenType:    firstNonEmpty(parsed.TokenType, "Bearer"),
		IDToken:      parsed.IDToken,
		ExpiryDateMS: expiryDateMS,
		RefreshToken: parsed.RefreshToken,
	}, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
