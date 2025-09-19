package clips

import (
	"context"
	"encoding/json"
	"fmt"
	"k0pern1cus/app/client/clip_downloader"
	"k0pern1cus/app/client/twitch"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/getsentry/sentry-go"
)

var retryInterval = time.Second
var maxRetries = 3

type ClipHandle struct {
	prepareCalled   atomic.Bool
	prepared        atomic.Bool
	preciseDuration atomic.Pointer[time.Duration]

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

func (h *ClipHandle) download(ctx context.Context, localHub *sentry.Hub) error {
	span := sentry.StartSpan(ctx, "clip_handle.download")
	defer span.Finish()

	span.SetTag("clip_id", h.clip.ID)

	beginTime := time.Now()

	for i := 0; i < maxRetries; i++ {
		if err := h.downloader.DownloadClip(ctx, h.clip.ID, h.getDownloadPath()); err != nil {
			slog.Error("Download clip error",
				slog.String("clip_id", h.clip.ID),
				slog.Any("error", err),
				slog.Int("attempt", i+1),
				slog.Int("max_attempts", maxRetries),
			)

			if i == maxRetries-1 {
				localHub.CaptureException(err)
				return err
			}

			time.Sleep(retryInterval)
			continue
		}

		slog.Debug("Clip download finished",
			slog.String("clip_id", h.clip.ID),
			slog.Duration("exec_time", time.Since(beginTime)),
		)
		return nil
	}

	return fmt.Errorf("unexpected error")
}

func (h *ClipHandle) measurePreciseDuration(ctx context.Context, localHub *sentry.Hub) (time.Duration, error) {
	span := sentry.StartSpan(ctx, "clip_handle.measure_duration")
	defer span.Finish()

	span.SetTag("clip_id", h.clip.ID)

	beginTime := time.Now()

	args := []string{
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		h.getDownloadPath(),
	}

	cmd := exec.CommandContext(ctx, "ffprobe", args...)

	output, err := cmd.Output()
	if err != nil {
		localHub.CaptureException(err)
		return 0, fmt.Errorf("ffprobe failed: %w", err)
	}

	var probeOutput struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err = json.Unmarshal(output, &probeOutput); err != nil {
		localHub.CaptureException(err)
		return 0, fmt.Errorf("parse ffprobe output: %w", err)
	}

	if probeOutput.Format.Duration == "" {
		localHub.CaptureException(err)
		return 0, fmt.Errorf("no duration found in ffprobe output")
	}

	durationSec, err := strconv.ParseFloat(probeOutput.Format.Duration, 64)
	if err != nil {
		localHub.CaptureException(err)
		return 0, fmt.Errorf("parse duration: %w", err)
	}

	slog.Debug("Clip duration measurement finished",
		slog.String("clip_id", h.clip.ID),
		slog.Duration("exec_time", time.Since(beginTime)),
	)

	return time.Duration(durationSec * float64(time.Second)), nil
}

func (h *ClipHandle) prepareAsync(ctx context.Context) {
	localHub := sentry.CurrentHub().Clone()

	defer close(h.readyChan)

	if err := h.download(ctx, localHub); err != nil {
		slog.Error("Max retries exceeded for clip download",
			slog.String("clip_id", h.clip.ID),
			slog.Any("error", err),
		)
		return
	}

	duration, err := h.measurePreciseDuration(ctx, localHub)
	if err != nil {
		slog.Error("Measure precise duration for clip failed",
			slog.String("clip_id", h.clip.ID),
			slog.Any("error", err),
		)
		return
	}

	h.preciseDuration.Store(&duration)
	h.prepared.Store(true)
}

func (h *ClipHandle) getDownloadPath() string {
	return filepath.Join("data", h.clip.ID+".mp4")
}

func (h *ClipHandle) GetDownloadedFile() (string, bool) {
	return h.getDownloadPath(), h.prepared.Load()
}

func (h *ClipHandle) GetPreciseDuration() time.Duration {
	duration := h.preciseDuration.Load()
	if duration == nil {
		return -1
	}

	return *duration
}

func (h *ClipHandle) Clip() twitch.Clip {
	return h.clip
}

func (h *ClipHandle) Release() {
	_ = os.Remove(h.getDownloadPath())
}
