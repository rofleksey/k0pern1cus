package streamer

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"k0pern1cus/app/client/twitch"
	"k0pern1cus/app/service/clips"
	"k0pern1cus/pkg/config"
	"log/slog"
	"os/exec"
	"strings"

	"github.com/samber/do"
)

var fadeDuration = 0.5
var bufferSizeMB = 10

type Service struct {
	cfg          *config.Config
	clipsService *clips.Service
}

func New(di *do.Injector) (*Service, error) {
	return &Service{
		cfg:          do.MustInvoke[*config.Config](di),
		clipsService: do.MustInvoke[*clips.Service](di),
	}, nil
}

func (s *Service) startStreamerProcess(ctx context.Context) (io.WriteCloser, error) {
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-re",
		"-i", "pipe:0",
		"-c:v", "libx264",
		"-preset", "veryfast",
		"-maxrate", "3000k",
		"-bufsize", "6000k",
		"-pix_fmt", "yuv420p",
		"-g", "60",
		"-c:a", "aac",
		"-b:a", "128k",
		"-f", "flv",
		s.cfg.Twitch.RTMPUrl,
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("create stdin pipe: %w", err)
	}

	if err = cmd.Start(); err != nil {
		return nil, fmt.Errorf("start ffmpeg: %w", err)
	}

	go func() {
		if err := cmd.Wait(); err != nil {
			slog.Error("FFmpeg streamer process failed", "stderr", stderr.String(), "error", err)
		}
	}()

	return stdin, nil
}

func (s *Service) streamVideo(ctx context.Context, clip twitch.Clip, filePath string, stdin io.WriteCloser) error {
	fadeoutStart := clip.Duration - fadeDuration
	if fadeoutStart < 0 {
		fadeoutStart = 0
	}

	fadeFilterStr := fmt.Sprintf("fade=t=in:st=0:d=%0.2f,fade=t=out:st=%.2f:d=%0.2f",
		fadeDuration, fadeoutStart, fadeDuration)

	scaleFilter := "scale=1920:1080:force_original_aspect_ratio=decrease,pad=1920:1080:(ow-iw)/2:(oh-ih)/2"
	textFilter := fmt.Sprintf("drawtext=text='%s':x=w-text_w-10:y=10:fontsize=24:fontcolor=white:shadowcolor=black:shadowx=2:shadowy=2", clip.Title)

	filters := []string{fadeFilterStr, scaleFilter, textFilter}

	args := []string{
		"-re",
		"-i", filePath,
		"-vf", strings.Join(filters, ","),
		"-c:v", "libx264",
		"-preset", "veryfast",
		"-maxrate", "3000k",
		"-bufsize", "6000k",
		"-pix_fmt", "yuv420p",
		"-g", "60",
		"-c:a", "aac",
		"-b:a", "128k",
		"-f", "mpegts",
		"pipe:1",
	}

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("error creating stdout pipe: %v", err)
	}

	if err = cmd.Start(); err != nil {
		return fmt.Errorf("error starting FFmpeg: %v", err)
	}

	buf := make([]byte, bufferSizeMB*1024*1024)
	reader := bufio.NewReaderSize(stdout, len(buf))

	_, err = io.CopyBuffer(stdin, reader, buf)
	if err != nil {
		_ = cmd.Process.Kill()
		return fmt.Errorf("error copying video data: %v", err)
	}

	if err = cmd.Wait(); err != nil {
		return fmt.Errorf("FFmpeg processing error: %v, stderr: %s", err, stderr.String())
	}

	return nil
}

func (s *Service) Run(ctx context.Context) error {
	slog.Info("Starting the stream...")

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	stdin, err := s.startStreamerProcess(ctx)
	if err != nil {
		return fmt.Errorf("start streamer process: %w", err)
	}

	clip, ok := s.clipsService.RemoveClip()
	if !ok {
		return fmt.Errorf("clips not found")
	}

	clip.PrepareAsync(ctx)
	if err := clip.Join(ctx); err != nil {
		return fmt.Errorf("clip.Join: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err = clip.Join(ctx); err != nil {
			return fmt.Errorf("clip.Join: %w", err)
		}
		videoFile := clip.GetDownloadedFile()

		slog.Info("Streaming video",
			slog.String("clip_id", clip.Clip().ID),
		)

		nextClip, nextOk := s.clipsService.RemoveClip()
		if nextOk {
			nextClip.PrepareAsync(ctx)
		}

		if err = s.streamVideo(ctx, clip.Clip(), videoFile, stdin); err != nil {
			return fmt.Errorf("stream video: %w", err)
		}

		clip.Release()

		if !nextOk {
			return nil
		}
		clip = nextClip
	}
}
