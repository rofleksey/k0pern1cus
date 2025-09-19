package streamer

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"k0pern1cus/app/service/clips"
	"k0pern1cus/pkg/config"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/samber/do"
)

var fadeDuration = 0.5
var bufferSizeMB = 10
var preloadCount = 5
var preloadWorkerCount = 1
var artificialOffset = time.Second

type Service struct {
	cfg          *config.Config
	clipsService *clips.Service

	preloadWg   sync.WaitGroup
	preloadChan chan *clips.ClipHandle
}

func New(di *do.Injector) (*Service, error) {
	return &Service{
		cfg:          do.MustInvoke[*config.Config](di),
		clipsService: do.MustInvoke[*clips.Service](di),
		preloadChan:  make(chan *clips.ClipHandle, preloadCount),
	}, nil
}

func (s *Service) startStreamerProcess(ctx context.Context) (io.WriteCloser, *exec.Cmd, error) {
	localHub := sentry.CurrentHub().Clone()

	span := sentry.StartSpan(ctx, "streamer.streamer_process")
	defer span.Finish()
	defer sentry.Recover()

	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-hide_banner",
		"-loglevel", "warning",
		"-re",
		"-f", "mpegts",
		"-i", "pipe:0",
		"-c:v", "copy",
		"-c:a", "copy",
		"-fflags", "+genpts",
		"-copyts",
		"-f", "flv",
		"-flvflags", "no_duration_filesize",
		"-max_delay", "1000000",
		s.cfg.Twitch.RTMPUrl,
	)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		localHub.CaptureException(err)
		return nil, nil, fmt.Errorf("create stdin pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		localHub.CaptureException(err)
		return nil, nil, fmt.Errorf("create stderr pipe: %w", err)
	}

	if err = cmd.Start(); err != nil {
		localHub.CaptureException(err)
		return nil, nil, fmt.Errorf("start ffmpeg: %w", err)
	}

	go s.monitorFFmpegOutput(stderr, "main")

	return stdin, cmd, nil
}

func (s *Service) monitorFFmpegOutput(stderr io.ReadCloser, processType string) {
	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		line := scanner.Text()

		slog.Warn("FFmpeg output",
			slog.String("process", processType),
			slog.String("line", line),
		)
	}
}

func (s *Service) streamVideo(ctx context.Context, clipHandle *clips.ClipHandle, stdin io.WriteCloser, startOffset time.Duration) (time.Duration, error) {
	span := sentry.StartSpan(ctx, "streamer.stream_video")
	defer span.Finish()

	span.SetTag("clip_id", clipHandle.Clip().ID)

	clip := clipHandle.Clip()
	filePath, _ := clipHandle.GetDownloadedFile()

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
		"-i", filePath,
		"-vf", strings.Join(filters, ","),
		"-c:v", "libx264",
		"-preset", "fast",
		"-tune", "zerolatency",
		"-profile:v", "main",
		"-b:v", "6000k",
		"-maxrate", "6000k",
		"-minrate", "6000k",
		"-bufsize", "12000k",
		"-r", "60",
		"-g", "120",
		"-keyint_min", "120",
		"-pix_fmt", "yuv420p",
		"-x264opts", "nal-hrd=cbr:force-cfr=1",
		"-output_ts_offset", fmt.Sprintf("%.6f", startOffset.Seconds()),
		"-c:a", "aac",
		"-b:a", "160k",
		"-ar", "44100",
		"-ac", "2",
		"-f", "mpegts",
		"-mpegts_flags", "initial_discontinuity",
		"-flush_packets", "1",
		"-muxdelay", "0",
		"-muxpreload", "0",
		"-max_delay", "0",
		"-avioflags", "direct",
		"-",
	}

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		sentry.CaptureException(err)
		return 0, fmt.Errorf("create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		sentry.CaptureException(err)
		return 0, fmt.Errorf("create stderr pipe: %w", err)
	}

	go s.monitorFFmpegOutput(stderr, clip.ID)

	if err = cmd.Start(); err != nil {
		sentry.CaptureException(err)
		return 0, fmt.Errorf("start ffmpeg: %w", err)
	}

	buf := make([]byte, bufferSizeMB*1024*1024)
	reader := bufio.NewReaderSize(stdout, len(buf))

	_, err = io.CopyBuffer(stdin, reader, buf)
	if err != nil {
		_ = cmd.Process.Kill()
		sentry.CaptureException(err)
		return 0, fmt.Errorf("copy video data: %w", err)
	}

	if err = cmd.Wait(); err != nil {
		sentry.CaptureException(err)
		return 0, fmt.Errorf("ffmpeg processing: %w", err)
	}

	return startOffset + clipHandle.GetPreciseDuration() + artificialOffset, nil
}

func (s *Service) preloadWorker(ctx context.Context) {
	defer s.preloadWg.Done()

	for {
		clip, ok := s.clipsService.RemoveClip()
		if !ok {
			return
		}

		readyChan := clip.PrepareAsync(ctx)

		select {
		case <-ctx.Done():
			return
		case <-readyChan:
			select {
			case <-ctx.Done():
				return
			case s.preloadChan <- clip:
			}
		}
	}
}

func (s *Service) startPreloadWorkers(ctx context.Context) {
	for i := 0; i < preloadWorkerCount; i++ {
		s.preloadWg.Add(1)
		go s.preloadWorker(ctx)
	}

	go func() {
		s.preloadWg.Wait()
		close(s.preloadChan)
	}()
}

func (s *Service) getNextClip(ctx context.Context) (*clips.ClipHandle, bool) {
	select {
	case <-ctx.Done():
		return nil, false
	case clip, ok := <-s.preloadChan:
		return clip, ok
	}
}

func (s *Service) Run(ctx context.Context) error {
	span := sentry.StartSpan(ctx, "streamer.run")
	defer span.Finish()
	defer sentry.Recover()

	slog.Info("Starting the stream...")

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	stdin, cmd, err := s.startStreamerProcess(ctx)
	if err != nil {
		sentry.CaptureException(err)
		return fmt.Errorf("start streamer process: %w", err)
	}
	defer stdin.Close()

	go func() {
		_ = cmd.Wait()
		cancel()
	}()

	s.startPreloadWorkers(ctx)

	var currentOffset time.Duration

	for {
		clip, ok := s.getNextClip(ctx)
		if !ok {
			return fmt.Errorf("no clips available")
		}

		if _, downloadOk := clip.GetDownloadedFile(); downloadOk {
			slog.Info("Streaming video",
				slog.String("clip_url", clip.Clip().URL),
			)

			newOffset, err := s.streamVideo(ctx, clip, stdin, currentOffset)
			if err != nil {
				sentry.CaptureException(err)
				return fmt.Errorf("stream video: %w", err)
			}

			currentOffset = newOffset
		} else {
			slog.Error("Skipping video due to download failure",
				slog.String("clip_url", clip.Clip().URL),
			)
		}

		clip.Release()
	}
}
