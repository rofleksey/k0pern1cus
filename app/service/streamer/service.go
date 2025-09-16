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
	"sync"

	"github.com/samber/do"
)

var fadeDuration = 0.5
var bufferSizeMB = 10
var preloadCount = 3

type Service struct {
	cfg          *config.Config
	clipsService *clips.Service

	preloadMutex sync.Mutex
	preloaded    []*clips.ClipHandle
}

func New(di *do.Injector) (*Service, error) {
	return &Service{
		cfg:          do.MustInvoke[*config.Config](di),
		clipsService: do.MustInvoke[*clips.Service](di),
		preloaded:    make([]*clips.ClipHandle, 0, preloadCount),
	}, nil
}

func (s *Service) startStreamerProcess(ctx context.Context) (io.WriteCloser, error) {
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-re",
		"-i", "pipe:0",
		"-c:v", "libx264",
		"-preset", "medium",
		"-tune", "film",
		"-profile:v", "high",
		"-level", "4.0",
		"-crf", "18",
		"-maxrate", "6000k",
		"-bufsize", "12000k",
		"-pix_fmt", "yuv420p",
		"-g", "60",
		"-keyint_min", "60",
		"-sc_threshold", "0",
		"-c:a", "aac",
		"-b:a", "160k",
		"-ar", "48000",
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

	filters := []string{
		fmt.Sprintf("fade=t=in:st=0:d=%.2f", fadeDuration),
		fmt.Sprintf("fade=t=out:st=%.2f:d=%.2f", fadeoutStart, fadeDuration),
		"scale=1920:1080:flags=lanczos:force_original_aspect_ratio=decrease",
		"pad=1920:1080:(ow-iw)/2:(oh-ih)/2:color=black",
	}

	escapedTitle := strings.ReplaceAll(clip.Title, "'", "'\\''")
	escapedTitle = strings.ReplaceAll(escapedTitle, ":", "\\:")
	filters = append(filters, fmt.Sprintf("drawtext=text='%s':fontfile=/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf:x=w-text_w-20:y=20:fontsize=28:fontcolor=white:shadowcolor=black:shadowx=2:shadowy=2", escapedTitle))

	args := []string{
		"-re",
		"-i", filePath,
		"-vf", strings.Join(filters, ","),
		"-c:v", "libx264",
		"-preset", "medium",
		"-tune", "film",
		"-profile:v", "high",
		"-level", "4.0",
		"-crf", "18",
		"-maxrate", "6000k",
		"-bufsize", "12000k",
		"-pix_fmt", "yuv420p",
		"-g", "60",
		"-keyint_min", "60",
		"-sc_threshold", "0",
		"-c:a", "aac",
		"-b:a", "160k",
		"-ar", "48000",
		"-f", "mpegts",
		"-movflags", "+faststart",
		"-",
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

func (s *Service) preloadClips(ctx context.Context) {
	s.preloadMutex.Lock()
	defer s.preloadMutex.Unlock()

	for len(s.preloaded) < preloadCount {
		clip, ok := s.clipsService.RemoveClip()
		if !ok {
			break
		}
		clip.PrepareAsync(ctx)
		s.preloaded = append(s.preloaded, clip)
	}
}

func (s *Service) getNextClip(ctx context.Context) (*clips.ClipHandle, bool) {
	s.preloadMutex.Lock()
	defer s.preloadMutex.Unlock()

	if len(s.preloaded) == 0 {
		return nil, false
	}

	nextClip := s.preloaded[0]
	s.preloaded = s.preloaded[1:]

	go s.preloadClips(ctx)

	return nextClip, true
}

func (s *Service) Run(ctx context.Context) error {
	slog.Info("Starting the stream...")

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	stdin, err := s.startStreamerProcess(ctx)
	if err != nil {
		return fmt.Errorf("start streamer process: %w", err)
	}

	go s.preloadClips(ctx)

	clip, ok := s.getNextClip(ctx)
	if !ok {
		return fmt.Errorf("no clips available")
	}

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

		nextClip, nextOk := s.getNextClip(ctx)

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
