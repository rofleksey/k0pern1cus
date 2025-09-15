package clip_downloader

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/samber/do"
)

// Code taken from https://github.com/ihabunek/twitch-dl

const (
	gqlURL   = "https://gql.twitch.tv/gql"
	clientID = "kd1unb4b3q4t58fwlpcbzcbnm76a8fp"
)

type Downloader struct {
	client *http.Client
}

func New(di *do.Injector) (*Downloader, error) {
	return &Downloader{
		client: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}, nil
}

func (d *Downloader) DownloadClip(ctx context.Context, slug, outputPath string) error {
	downloadURL, err := d.getClipAuthenticatedURL(ctx, slug)
	if err != nil {
		return fmt.Errorf("could not get clip authenticated url: %w", err)
	}

	if err = d.downloadFile(ctx, downloadURL, outputPath); err != nil {
		return fmt.Errorf("could not download clip: %w", err)
	}

	return nil
}

func (d *Downloader) getClipAccessToken(ctx context.Context, slug string) (*ClipAccessToken, error) {
	query := map[string]any{
		"operationName": "VideoAccessToken_Clip",
		"variables":     map[string]string{"slug": slug},
		"extensions": map[string]any{
			"persistedQuery": map[string]any{
				"version":    1,
				"sha256Hash": "36b89d2507fce29e5ca551df756d27c1cfe079e2609642b4390aa4c35796eb11",
			},
		},
	}

	queryBytes, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("could not marshal query: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, gqlURL, strings.NewReader(string(queryBytes)))
	if err != nil {
		return nil, fmt.Errorf("could not create request: %w", err)
	}

	req.Header.Set("Client-ID", clientID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status: %s", resp.Status)
	}

	var response struct {
		Data struct {
			Clip *ClipAccessToken `json:"clip"`
		} `json:"data"`
		Errors []any `json:"errors"`
	}

	if err = json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response body: %w", err)
	}

	if response.Data.Clip == nil {
		return nil, fmt.Errorf("access token not found for clip: %s", slug)
	}

	return response.Data.Clip, nil
}

func (d *Downloader) getClipAuthenticatedURL(ctx context.Context, slug string) (string, error) {
	accessToken, err := d.getClipAccessToken(ctx, slug)
	if err != nil {
		return "", fmt.Errorf("could not get clip access token: %w", err)
	}

	if len(accessToken.VideoQualities) == 0 {
		return "", fmt.Errorf("no video qualities available for clip: %s", slug)
	}

	selectedURL := accessToken.VideoQualities[0].SourceURL

	params := url.Values{}
	params.Add("sig", accessToken.PlaybackAccessToken.Signature)
	params.Add("token", accessToken.PlaybackAccessToken.Value)

	return fmt.Sprintf("%s?%s", selectedURL, params.Encode()), nil
}

func (d *Downloader) downloadFile(ctx context.Context, url, filepath string) error {
	out, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("could not create file: %w", err)
	}
	defer out.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("could not create request: %w", err)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("could not execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status: %s", resp.Status)
	}

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("could not copy file: %w", err)
	}

	return nil
}
