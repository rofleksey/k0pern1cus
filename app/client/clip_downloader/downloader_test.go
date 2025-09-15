package clip_downloader

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDownloadClip(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "clip_download_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	downloader := &Downloader{
		client: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}

	slug := "QuaintAssiduousZebraTakeNRG-XYLPHliuB9eP5MxM"
	outputPath := filepath.Join(tempDir, "test_clip.mp4")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	err = downloader.DownloadClip(ctx, slug, outputPath)
	require.NoError(t, err, "Download should complete without error")

	fileInfo, err := os.Stat(outputPath)
	require.NoError(t, err, "Downloaded file should exist")
	require.NotNil(t, fileInfo, "File info should not be nil")

	if fileInfo != nil {
		require.GreaterOrEqual(t, fileInfo.Size(), int64(1024*1024),
			"Downloaded file should be at least 1MB in size")
	}
}
