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

	"github.com/samber/do"
)

var pageSize = 100
var fetchDelay = time.Second

type Service struct {
	cfg        *config.Config
	client     *twitch.Client
	downloader *clip_downloader.Downloader

	m            sync.RWMutex
	clips        map[string]*ClipHandle
	initialized  bool
	initComplete chan struct{}
}

func New(di *do.Injector) (*Service, error) {
	return &Service{
		cfg:          do.MustInvoke[*config.Config](di),
		client:       do.MustInvoke[*twitch.Client](di),
		downloader:   do.MustInvoke[*clip_downloader.Downloader](di),
		clips:        make(map[string]*ClipHandle),
		initComplete: make(chan struct{}),
	}, nil
}

func (s *Service) Init(ctx context.Context) error {
	slog.Info("Initializing clips...")

	minDate, err := time.Parse("January 2, 2006", s.cfg.Twitch.MinDate)
	if err != nil {
		return fmt.Errorf("could not parse account creation_date: %v", err)
	}

	go s.backgroundFetchAllClips(ctx, minDate)

	select {
	case <-s.initComplete:
		slog.Info("Initial clips loaded, continuing with background fetch")
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Service) backgroundFetchAllClips(ctx context.Context, minDate time.Time) {
	timeWindow := 24 * time.Hour * 30 * 5 // 5 months

	rand.Shuffle(len(s.cfg.Twitch.BroadcasterIDs), func(i, j int) {
		s.cfg.Twitch.BroadcasterIDs[i], s.cfg.Twitch.BroadcasterIDs[j] = s.cfg.Twitch.BroadcasterIDs[j], s.cfg.Twitch.BroadcasterIDs[i]
	})

	for _, broadcasterID := range s.cfg.Twitch.BroadcasterIDs {
		endedAt := time.Now()
		startedAt := endedAt.Add(-timeWindow)

		for {
			var after string

			for {
				slog.Info("Getting clips...",
					slog.String("broadcaster_id", broadcasterID),
					slog.Time("started_at", startedAt),
					slog.Time("ended_at", endedAt),
					slog.String("after", after),
				)

				res, err := s.client.GetClips(ctx, &twitch.GetClipsParams{
					BroadcasterID: broadcasterID,
					First:         pageSize,
					StartedAt:     startedAt,
					EndedAt:       endedAt,
					After:         after,
				})
				if err != nil {
					slog.Error("Failed to get clips", slog.String("error", err.Error()))
					time.Sleep(fetchDelay)
					continue
				}

				if len(res.Data) == 0 {
					time.Sleep(fetchDelay)
					break
				}

				// Process clips and add to storage
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
					time.Sleep(fetchDelay)
					break
				}

				after = res.Pagination.Cursor
				time.Sleep(fetchDelay)
			}

			endedAt = startedAt
			startedAt = endedAt.Add(-timeWindow)

			if endedAt.Before(minDate) {
				break
			}
		}
	}

	s.m.Lock()
	if !s.initialized {
		s.initialized = true
		close(s.initComplete)
	}
	s.m.Unlock()
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
