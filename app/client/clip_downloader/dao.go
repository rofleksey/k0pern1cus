package clip_downloader

// Clip represents a Twitch clip
type Clip struct {
	ID             string         `json:"id"`
	Slug           string         `json:"slug"`
	URL            string         `json:"url"`
	VideoQualities []VideoQuality `json:"videoQualities"`
}

// VideoQuality represents a clip video quality option
type VideoQuality struct {
	FrameRate float32 `json:"frameRate"`
	Quality   string  `json:"quality"`
	SourceURL string  `json:"sourceURL"`
}

// Game represents a game on Twitch
type Game struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// User represents a Twitch user
type User struct {
	ID          string `json:"id"`
	Login       string `json:"login"`
	DisplayName string `json:"displayName"`
}

// ClipAccessToken represents an access token for downloading a clip
type ClipAccessToken struct {
	ID                  string `json:"id"`
	PlaybackAccessToken struct {
		Signature string `json:"signature"`
		Value     string `json:"value"`
	} `json:"playbackAccessToken"`
	VideoQualities []VideoQuality `json:"videoQualities"`
}

// DownloadClipOptions contains options for downloading a clip
type DownloadClipOptions struct {
	Quality    string
	Output     string
	Overwrite  bool
	AuthToken  string
	RateLimit  int
	MaxWorkers int
}
