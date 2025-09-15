package twitch

import "time"

// GetClipsParams represents the parameters for getting clips
type GetClipsParams struct {
	BroadcasterID string
	GameID        string
	IDs           []string
	First         int
	After         string
	Before        string
	StartedAt     time.Time
	EndedAt       time.Time
}

// Clip represents a Twitch clip
type Clip struct {
	ID              string    `json:"id"`
	URL             string    `json:"url"`
	EmbedURL        string    `json:"embed_url"`
	BroadcasterID   string    `json:"broadcaster_id"`
	BroadcasterName string    `json:"broadcaster_name"`
	CreatorID       string    `json:"creator_id"`
	CreatorName     string    `json:"creator_name"`
	VideoID         string    `json:"video_id"`
	GameID          string    `json:"game_id"`
	Language        string    `json:"language"`
	Title           string    `json:"title"`
	ViewCount       int       `json:"view_count"`
	CreatedAt       time.Time `json:"created_at"`
	ThumbnailURL    string    `json:"thumbnail_url"`
	Duration        float64   `json:"duration"`
	VodOffset       int       `json:"vod_offset"`
	IsFeatured      bool      `json:"is_featured"`
}

// ClipsResponse represents the response from the clips endpoint
type ClipsResponse struct {
	Data       []Clip      `json:"data"`
	Pagination *Pagination `json:"pagination,omitempty"`
}

// Pagination represents pagination information
type Pagination struct {
	Cursor string `json:"cursor,omitempty"`
}

// authResponse represents the response from the OAuth token endpoint
type authResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}
