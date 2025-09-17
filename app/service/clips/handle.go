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
var maxRetries = 3

type ClipHandle struct {
	prepareCalled atomic.Bool
	downloaded    atomic.Bool

	clip       twitch.Clip
	downloader *clip_downloader.Downloader

	readyChan chan struct{}
}

func (h *ClipHandle) PrepareAsync(ctx context.Context) chan struct{} {
	if !h.prepareCalled.CompareAndSwap(false, true) {
		return h.readyChan
	}

	go h.prepareAsync(ctx)
	return h.readyChan
}

func (h *ClipHandle) prepareAsync(ctx context.Context) {
	defer close(h.readyChan)

	downloadBeginTime := time.Now()
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
		slog.Debug("Clip download finished",
			slog.String("clip_id", h.clip.ID),
			slog.Duration("duration", time.Since(downloadBeginTime)),
		)
		break
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
