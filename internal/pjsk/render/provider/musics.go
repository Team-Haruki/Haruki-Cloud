package provider

import (
	"context"

	"haruki-cloud/internal/pjsk/render/masterdata"
)

// MusicProvider exposes music-related masterdata queries.
type MusicProvider interface {
	Search(ctx context.Context, query string) (*masterdata.Music, error)
	GetByID(ctx context.Context, id int) (*masterdata.Music, error)
	GetByEventID(ctx context.Context, eventID int) (*masterdata.Music, error)
	GetAll(ctx context.Context) []*masterdata.Music
	GetLocalizedTitles(ctx context.Context, musicID int) ([]string, error)
	GetDifficulties(ctx context.Context, musicID int) ([]*masterdata.MusicDifficulty, error)
	GetVocals(ctx context.Context, musicID int) ([]*masterdata.MusicVocal, error)
	GetTags(ctx context.Context, musicID int) ([]string, error)
	GetOutsideCharacterByID(ctx context.Context, id int) (string, error)
	GetPrimaryEventByMusicID(ctx context.Context, musicID int) (*masterdata.Event, error)
	GetLimitedTimeMusics(ctx context.Context, musicID int) []*masterdata.LimitedTimeMusic
}
