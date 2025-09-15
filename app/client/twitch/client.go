package twitch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"k0pern1cus/pkg/config"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/samber/do"
)

var tokenRefreshInterval = 10 * time.Minute
var baseURL = "https://api.twitch.tv/helix"

type Client struct {
	cfg        *config.Config
	httpClient *http.Client

	mutex       sync.RWMutex
	authToken   string
	tokenExpiry time.Time
}

func NewClient(di *do.Injector) (*Client, error) {
	return &Client{
		cfg:        do.MustInvoke[*config.Config](di),
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func (c *Client) GetClips(ctx context.Context, params *GetClipsParams) (*ClipsResponse, error) {
	if err := c.ensureAuthenticated(ctx); err != nil {
		return nil, fmt.Errorf("authentication failed: %w", err)
	}

	queryParams := url.Values{}

	if params.BroadcasterID != "" {
		queryParams.Add("broadcaster_id", params.BroadcasterID)
	}
	if params.GameID != "" {
		queryParams.Add("game_id", params.GameID)
	}
	if len(params.IDs) > 0 {
		for _, id := range params.IDs {
			queryParams.Add("id", id)
		}
	}
	if params.First > 0 {
		queryParams.Add("first", fmt.Sprintf("%d", params.First))
	}
	if params.After != "" {
		queryParams.Add("after", params.After)
	}
	if params.Before != "" {
		queryParams.Add("before", params.Before)
	}
	if !params.StartedAt.IsZero() {
		queryParams.Add("started_at", params.StartedAt.Format(time.RFC3339))
	}
	if !params.EndedAt.IsZero() {
		queryParams.Add("ended_at", params.EndedAt.Format(time.RFC3339))
	}

	requestURL := fmt.Sprintf("%s/clips?%s", baseURL, queryParams.Encode())
	req, err := http.NewRequestWithContext(ctx, "GET", requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request failed: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.authToken)
	req.Header.Set("Client-Id", c.cfg.Twitch.ClientID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed: status %d, body: %s", resp.StatusCode, string(body))
	}

	var clipsResponse ClipsResponse
	if err := json.NewDecoder(resp.Body).Decode(&clipsResponse); err != nil {
		return nil, fmt.Errorf("decoding response failed: %w", err)
	}

	return &clipsResponse, nil
}

func (c *Client) ensureAuthenticated(ctx context.Context) error {
	c.mutex.RLock()
	if c.authToken != "" && time.Until(c.tokenExpiry) > tokenRefreshInterval {
		c.mutex.RUnlock()
		return nil
	}
	c.mutex.RUnlock()

	c.mutex.Lock()
	defer c.mutex.Unlock()

	token, expiry, err := c.getAccessToken(ctx)
	if err != nil {
		return err
	}

	c.authToken = token
	c.tokenExpiry = expiry
	return nil
}

func (c *Client) getAccessToken(ctx context.Context) (string, time.Time, error) {
	data := url.Values{}
	data.Set("client_id", c.cfg.Twitch.ClientID)
	data.Set("client_secret", c.cfg.Twitch.ClientSecret)
	data.Set("grant_type", "client_credentials")

	req, err := http.NewRequestWithContext(ctx, "POST", "https://id.twitch.tv/oauth2/token", strings.NewReader(data.Encode()))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("creating auth request failed: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("auth request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", time.Time{}, fmt.Errorf("authentication failed: status %d, body: %s", resp.StatusCode, string(body))
	}

	var authResp authResponse
	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		return "", time.Time{}, fmt.Errorf("decoding auth response failed: %w", err)
	}

	expiry := time.Now().Add(time.Duration(authResp.ExpiresIn) * time.Second)

	return authResp.AccessToken, expiry, nil
}
