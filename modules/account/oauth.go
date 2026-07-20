package account

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Hand-rolled OAuth2 authorization-code flow (spec decision: zero new
// dependencies). Discord's flow is the plain RFC 6749 shape: redirect
// to authorize, POST the code to the token endpoint, GET /users/@me.
const (
	defaultDiscordAuthURL  = "https://discord.com/oauth2/authorize"
	defaultDiscordTokenURL = "https://discord.com/api/oauth2/token"
	defaultDiscordAPIBase  = "https://discord.com/api"
)

type discordClient struct {
	cfg DiscordConfig
	hc  *http.Client
}

func newDiscordClient(cfg DiscordConfig) *discordClient {
	return &discordClient{cfg: cfg, hc: &http.Client{Timeout: 10 * time.Second}}
}

// configured reports whether the operator supplied app credentials;
// unconfigured providers hide their routes (404).
func (d *discordClient) configured() bool {
	return d.cfg.ClientID != "" && d.cfg.ClientSecret != ""
}

func (d *discordClient) authorizeURL(state, redirectURI string) string {
	q := url.Values{
		"client_id":     {d.cfg.ClientID},
		"redirect_uri":  {redirectURI},
		"response_type": {"code"},
		"scope":         {"identify"},
		"state":         {state},
	}
	return cmp.Or(d.cfg.AuthURL, defaultDiscordAuthURL) + "?" + q.Encode()
}

func (d *discordClient) exchangeCode(ctx context.Context, code, redirectURI string) (string, error) {
	form := url.Values{
		"client_id":     {d.cfg.ClientID},
		"client_secret": {d.cfg.ClientSecret},
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		cmp.Or(d.cfg.TokenURL, defaultDiscordTokenURL), strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := d.hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("token exchange: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("token exchange: HTTP %d: %s", resp.StatusCode, b)
	}
	var payload struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil || payload.AccessToken == "" {
		return "", fmt.Errorf("token exchange: bad payload (%v)", err)
	}
	return payload.AccessToken, nil
}

func (d *discordClient) identify(ctx context.Context, accessToken string) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		cmp.Or(d.cfg.APIBase, defaultDiscordAPIBase)+"/users/@me", nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := d.hc.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("identify: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("identify: HTTP %d", resp.StatusCode)
	}
	var payload struct {
		ID       string `json:"id"`
		Username string `json:"username"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil || payload.ID == "" {
		return "", "", fmt.Errorf("identify: bad payload (%v)", err)
	}
	return payload.ID, payload.Username, nil
}
