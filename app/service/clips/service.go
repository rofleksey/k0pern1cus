package clips

import (
	"context"
	"fmt"
	"k0pern1cus/app/client/clip_downloader"
	"k0pern1cus/app/client/twitch"
	"log/slog"
	"math/rand"
	"sync"
	"time"

	"github.com/samber/do"
)

var accountCreationDateStr = "June 20, 2017"
var broadcasterID = "160864129" // k0per1s
var gameID = "491487"           // dead by daylight
var pageSize = 100

type Service struct {
	client     *twitch.Client
	downloader *clip_downloader.Downloader

	m     sync.Mutex
	clips map[string]*ClipHandle
}

func New(di *do.Injector) (*Service, error) {
	return &Service{
		client:     do.MustInvoke[*twitch.Client](di),
		downloader: do.MustInvoke[*clip_downloader.Downloader](di),
	}, nil
}

func (s *Service) Init(ctx context.Context) error {
	slog.Info("Initializing clips...")

	s.m.Lock()
	defer s.m.Unlock()

	clips := make(map[string]*ClipHandle)

	accountCreationDate, err := time.Parse("January 2, 2006", accountCreationDateStr)
	if err != nil {
		return fmt.Errorf("could not parse account creation_date: %v", err)
	}

	timeWindow := 24 * time.Hour * 30 * 5 // 5 months, to prevent hitting the 1000 clips per pagination limit

	endedAt := time.Now()
	startedAt := endedAt.Add(timeWindow)

	for {
		var after string

		batchCnt := 0
		for {
			slog.Info("Getting clips...",
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
				if clip.GameID != gameID {
					continue
				}

				clips[clip.ID] = &ClipHandle{
					clip:       clip,
					downloader: s.downloader,
					readyChan:  make(chan struct{}),
				}
				batchCnt++
			}

			if res.Pagination.Cursor == "" {
				break
			}

			after = res.Pagination.Cursor
			time.Sleep(time.Second)
		}

		endedAt = startedAt
		startedAt = endedAt.Add(-timeWindow)

		if endedAt.Before(accountCreationDate) {
			break
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
