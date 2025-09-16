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

type Service struct {
	cfg        *config.Config
	client     *twitch.Client
	downloader *clip_downloader.Downloader

	m     sync.Mutex
	clips map[string]*ClipHandle
}

func New(di *do.Injector) (*Service, error) {
	return &Service{
		cfg:        do.MustInvoke[*config.Config](di),
		client:     do.MustInvoke[*twitch.Client](di),
		downloader: do.MustInvoke[*clip_downloader.Downloader](di),
	}, nil
}

func (s *Service) Init(ctx context.Context) error {
	slog.Info("Initializing clips...")

	s.m.Lock()
	defer s.m.Unlock()

	clips := make(map[string]*ClipHandle)

	minDate, err := time.Parse("January 2, 2006", s.cfg.Twitch.MinDate)
	if err != nil {
		return fmt.Errorf("could not parse account creation_date: %v", err)
	}

	timeWindow := 24 * time.Hour * 30 * 5 // 5 months, to prevent hitting the 1000 clips per pagination limit

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
					return fmt.Errorf("client.GetClips: %w", err)
				}

				if len(res.Data) == 0 {
					break
				}

				for _, clip := range res.Data {
					if clip.GameID != s.cfg.Twitch.GameID {
						continue
					}

					clips[clip.ID] = &ClipHandle{
						clip:       clip,
						downloader: s.downloader,
						readyChan:  make(chan struct{}),
					}
				}

				if res.Pagination.Cursor == "" {
					break
				}

				after = res.Pagination.Cursor
				time.Sleep(time.Second)
			}

			endedAt = startedAt
			startedAt = endedAt.Add(-timeWindow)

			if endedAt.Before(minDate) {
				break
			}
		}
	}

	if len(clips) == 0 {
		return fmt.Errorf("clips not found")
	}

	slog.Info("Clips initialized",
		slog.Int("count", len(clips)),
	)

	s.clips = clips

	return nil
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
