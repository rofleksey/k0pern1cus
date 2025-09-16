package streamer

import (
	"bufio"
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

func (s *Service) startStreamerProcess(ctx context.Context) (io.WriteCloser, *exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-hide_banner",
		"-loglevel", "warning",
		"-stats",
		"-i", "pipe:0",
		"-fflags", "+genpts+igndts+flush_packets",
		"-avoid_negative_ts", "make_non_negative",
		"-c:v", "copy",
		"-c:a", "copy",
		"-flvflags", "no_duration_filesize+autoresume",
		"-f", "flv",
		"-flush_packets", "1",
		"-movflags", "+faststart",
		"-rtmp_buffer", "5000",
		s.cfg.Twitch.RTMPUrl,
	)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("create stdin pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("create stderr pipe: %w", err)
	}

	if err = cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("start ffmpeg: %w", err)
	}

	go s.monitorFFmpegOutput(stderr, "main")

	return stdin, cmd, nil
}

func (s *Service) monitorFFmpegOutput(stderr io.ReadCloser, processType string) {
	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		line := scanner.Text()
		slog.Debug("FFmpeg output", "process", processType, "line", line)
	}
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

	escapedTitle := fmt.Sprintf("%s - %s", clip.BroadcasterName, clip.Title)
	escapedTitle = strings.ReplaceAll(escapedTitle, "'", "'\\''")
	escapedTitle = strings.ReplaceAll(escapedTitle, ":", "\\:")
	filters = append(filters, fmt.Sprintf("drawtext=text='%s':fontfile=/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf:x=w-text_w-20:y=20:fontsize=28:fontcolor=white:shadowcolor=black:shadowx=2:shadowy=2", escapedTitle))

	args := []string{
		"-hide_banner",
		"-loglevel", "warning",
		"-re",
		"-i", filePath,
		"-vf", strings.Join(filters, ","),
		"-c:v", "libx264",
		"-preset", "faster",
		"-tune", "zerolatency",
		"-b:v", "6000k",
		"-maxrate", "6000k",
		"-minrate", "6000k",
		"-bufsize", "12000k",
		"-r", "60",
		"-g", "120",
		"-keyint_min", "120",
		"-pix_fmt", "yuv420p",
		"-x264opts", "nal-hrd=cbr",
		"-c:a", "aac",
		"-b:a", "160k",
		"-ar", "48000",
		"-ac", "2",
		"-f", "mpegts",
		"-muxdelay", "0",
		"-muxpreload", "0",
		"-",
	}

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("create stdout pipe: %w", err)
	}

	if err = cmd.Start(); err != nil {
		return fmt.Errorf("start ffmpeg: %w", err)
	}

	buf := make([]byte, bufferSizeMB*1024*1024)
	reader := bufio.NewReaderSize(stdout, len(buf))

	_, err = io.CopyBuffer(stdin, reader, buf)
	if err != nil {
		_ = cmd.Process.Kill()
		return fmt.Errorf("copy video data: %w", err)
	}

	if err = cmd.Wait(); err != nil {
		return fmt.Errorf("ffmpeg processing: %w", err)
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

	stdin, cmd, err := s.startStreamerProcess(ctx)
	if err != nil {
		return fmt.Errorf("start streamer process: %w", err)
	}
	defer stdin.Close()

	go func() {
		_ = cmd.Wait()
		cancel()
	}()

	s.preloadClips(ctx)

	clip, ok := s.getNextClip(ctx)
	if !ok {
		return fmt.Errorf("no clips available")
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err = clip.Join(ctx); err != nil {
			return fmt.Errorf("clip join: %w", err)
		}
		videoFile, downloadOk := clip.GetDownloadedFile()

		nextClip, nextOk := s.getNextClip(ctx)

		if downloadOk {
			slog.Debug("Streaming video",
				slog.String("clip_url", clip.Clip().URL),
			)

			if err = s.streamVideo(ctx, clip.Clip(), videoFile, stdin); err != nil {
				return fmt.Errorf("stream video: %w", err)
			}
		}

		clip.Release()

		if !nextOk {
			return nil
		}
		clip = nextClip
	}
}
