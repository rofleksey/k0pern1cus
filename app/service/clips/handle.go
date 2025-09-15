package clips

import (
	"context"
	"k0pern1cus/app/client/clip_downloader"
	"k0pern1cus/app/client/twitch"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

var retryInterval = time.Second

type ClipHandle struct {
	clip       twitch.Clip
	downloader *clip_downloader.Downloader

	readyChan chan struct{}
}

func (h *ClipHandle) PrepareAsync(ctx context.Context) {
	go h.prepareAsync(ctx)
}

func (h *ClipHandle) prepareAsync(ctx context.Context) {
	for {
		if err := h.downloader.DownloadClip(ctx, h.clip.ID, h.GetDownloadedFile()); err != nil {
			slog.Error("Download clip error",
				slog.String("clip_id", h.clip.ID),
				slog.Any("error", err),
			)

			time.Sleep(retryInterval)
			continue
		}

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

func (h *ClipHandle) GetDownloadedFile() string {
	return filepath.Join("data", h.clip.ID+".mp4")
}

func (h *ClipHandle) Clip() twitch.Clip {
	return h.clip
}

func (h *ClipHandle) Release() {
	_ = os.Remove(h.GetDownloadedFile())
}
