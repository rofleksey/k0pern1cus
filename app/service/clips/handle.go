package clips

import (
	"context"
	"k0pern1cus/app/client/clip_downloader"
	"k0pern1cus/app/client/twitch"
	"log/slog"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"
)

var retryInterval = time.Second

type ClipHandle struct {
	downloaded atomic.Bool
	clip       twitch.Clip
	downloader *clip_downloader.Downloader

	readyChan chan struct{}
}

func (h *ClipHandle) PrepareAsync(ctx context.Context) {
	go h.prepareAsync(ctx)
}

func (h *ClipHandle) prepareAsync(ctx context.Context) {
	maxRetries := 3

	for i := 0; i < maxRetries; i++ {
		if err := h.downloader.DownloadClip(ctx, h.clip.ID, h.getDownloadPath()); err != nil {
			slog.Error("Download clip error",
				slog.String("clip_id", h.clip.ID),
				slog.Any("error", err),
				slog.Int("attempt", i+1),
				slog.Int("max_attempts", maxRetries),
			)

			if i == maxRetries-1 {
				slog.Error("Max retries exceeded for clip download",
					slog.String("clip_id", h.clip.ID),
				)
				break
			}

			time.Sleep(retryInterval)
			continue
		}

		h.downloaded.Store(true)
		break
	}

	close(h.readyChan)
}

func (h *ClipHandle) Join(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-h.readyChan:
		return nil
	}
}

func (h *ClipHandle) getDownloadPath() string {
	return filepath.Join("data", h.clip.ID+".mp4")
}

func (h *ClipHandle) GetDownloadedFile() (string, bool) {
	return h.getDownloadPath(), h.downloaded.Load()
}

func (h *ClipHandle) Clip() twitch.Clip {
	return h.clip
}

func (h *ClipHandle) Release() {
	_ = os.Remove(h.getDownloadPath())
}
