package clips

import (
	"context"
	"fmt"
	"k0pern1cus/app/client/clip_downloader"
	"k0pern1cus/app/client/twitch"
	"k0pern1cus/pkg/config"
	"log/slog"
	"math/rand"
	"sync"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/samber/do"
)

var pageSize = 100
var rateLimitInterval = 3 * time.Second
var timeWindow = 24 * time.Hour * 30 * 5 // 5 months

type Service struct {
	cfg        *config.Config
	client     *twitch.Client
	downloader *clip_downloader.Downloader

	m            sync.RWMutex
	clips        map[string]*ClipHandle
	initialized  bool
	initComplete chan struct{}

	rateLimiter chan struct{}
}

func New(di *do.Injector) (*Service, error) {
	cfg := do.MustInvoke[*config.Config](di)

	rateLimiter := make(chan struct{}, 1)
	rateLimiter <- struct{}{}

	go func() {
		ticker := time.NewTicker(rateLimitInterval)
		defer ticker.Stop()

		for range ticker.C {
			rateLimiter <- struct{}{}
		}
	}()

	return &Service{
		cfg:          cfg,
		client:       do.MustInvoke[*twitch.Client](di),
		downloader:   do.MustInvoke[*clip_downloader.Downloader](di),
		clips:        make(map[string]*ClipHandle),
		initComplete: make(chan struct{}),
		rateLimiter:  rateLimiter,
	}, nil
}

func (s *Service) Init(ctx context.Context) error {
	span := sentry.StartSpan(ctx, "clips.init")
	defer span.Finish()

	slog.Debug("Initializing clips...")

	minDate, err := time.Parse("January 2, 2006", s.cfg.Twitch.MinDate)
	if err != nil {
		sentry.CaptureException(err)
		return fmt.Errorf("could not parse account creation_date: %v", err)
	}

	go s.backgroundFetchAllClips(ctx, minDate)

	select {
	case <-s.initComplete:
		slog.Debug("Initial clips loaded, continuing with background fetch")
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Service) backgroundFetchAllClips(ctx context.Context, minDate time.Time) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	rand.Shuffle(len(s.cfg.Twitch.BroadcasterIDs), func(i, j int) {
		s.cfg.Twitch.BroadcasterIDs[i], s.cfg.Twitch.BroadcasterIDs[j] = s.cfg.Twitch.BroadcasterIDs[j], s.cfg.Twitch.BroadcasterIDs[i]
	})

	var wg sync.WaitGroup
	broadcasterChan := make(chan string, len(s.cfg.Twitch.BroadcasterIDs))

	for i := 0; i < len(s.cfg.Twitch.BroadcasterIDs); i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for broadcasterID := range broadcasterChan {
				s.fetchBroadcasterClips(ctx, broadcasterID, minDate, workerID)
			}
		}(i)
	}

	for _, broadcasterID := range s.cfg.Twitch.BroadcasterIDs {
		broadcasterChan <- broadcasterID
	}
	close(broadcasterChan)

	wg.Wait()

	s.m.Lock()
	if !s.initialized {
		s.initialized = true
		close(s.initComplete)
	}

	totalDuration := 0.0

	for _, clip := range s.clips {
		totalDuration += clip.Clip().Duration
	}

	slog.Info("Initialized clips successfully",
		slog.Int("count", len(s.clips)),
		slog.Float64("duration", totalDuration),
	)
	s.m.Unlock()
}

func (s *Service) fetchBroadcasterClips(ctx context.Context, broadcasterID string, minDate time.Time, workerID int) {
	localHub := sentry.CurrentHub().Clone()

	span := sentry.StartSpan(ctx, "clips.fetch_broadcaster")
	defer span.Finish()
	defer sentry.Recover()

	span.SetTag("broadcaster_id", broadcasterID)
	span.SetTag("worker_id", fmt.Sprintf("%d", workerID))

	endedAt := time.Now()
	startedAt := endedAt.Add(-timeWindow)

	for {
		var after string

		for {
			select {
			case <-s.rateLimiter:
			case <-ctx.Done():
				return
			}

			slog.Debug("Getting clips...",
				slog.String("broadcaster_id", broadcasterID),
				slog.Time("started_at", startedAt),
				slog.Time("ended_at", endedAt),
				slog.String("after", after),
				slog.Int("worker_id", workerID),
			)

			res, err := s.client.GetClips(ctx, &twitch.GetClipsParams{
				BroadcasterID: broadcasterID,
				First:         pageSize,
				StartedAt:     startedAt,
				EndedAt:       endedAt,
				After:         after,
			})
			if err != nil {
				localHub.CaptureException(err)
				slog.Error("Failed to get clips",
					slog.String("error", err.Error()),
					slog.String("broadcaster_id", broadcasterID),
					slog.Int("worker_id", workerID),
				)
				continue
			}

			if len(res.Data) == 0 {
				break
			}

			newClips := make([]*ClipHandle, 0)
			for _, clip := range res.Data {
				if clip.GameID != s.cfg.Twitch.GameID {
					continue
				}

				clipHandle := &ClipHandle{
					clip:       clip,
					downloader: s.downloader,
					readyChan:  make(chan struct{}),
				}
				newClips = append(newClips, clipHandle)
			}

			if len(newClips) > 0 {
				s.m.Lock()
				for _, clipHandle := range newClips {
					s.clips[clipHandle.clip.ID] = clipHandle
				}

				if !s.initialized {
					s.initialized = true
					close(s.initComplete)
				}
				s.m.Unlock()
			}

			if res.Pagination.Cursor == "" {
				break
			}

			after = res.Pagination.Cursor
		}

		endedAt = startedAt
		startedAt = endedAt.Add(-timeWindow)

		if endedAt.Before(minDate) {
			break
		}
	}
}

func (s *Service) RemoveClip() (*ClipHandle, bool) {
	s.m.Lock()
	defer s.m.Unlock()

	if len(s.clips) == 0 {
		return nil, false
	}

	keys := make([]string, 0, len(s.clips))
	for key := range s.clips {
		keys = append(keys, key)
	}

	randomKey := keys[rand.Intn(len(keys))]

	clip := s.clips[randomKey]
	delete(s.clips, randomKey)

	return clip, true
}
