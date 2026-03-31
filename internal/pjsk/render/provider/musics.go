package provider

import "haruki-cloud/internal/pjsk/render/masterdata"

// MusicProvider exposes music-related masterdata queries.
type MusicProvider interface {
	Search(query string) (*masterdata.Music, error)
	GetByID(id int) (*masterdata.Music, error)
	GetByEventID(eventID int) (*masterdata.Music, error)
	GetAll() []*masterdata.Music
	GetLocalizedTitles(musicID int) ([]string, error)
	GetDifficulties(musicID int) ([]*masterdata.MusicDifficulty, error)
	GetVocals(musicID int) ([]*masterdata.MusicVocal, error)
	GetTags(musicID int) ([]string, error)
	GetOutsideCharacterByID(id int) (string, error)
	GetPrimaryEventByMusicID(musicID int) (*masterdata.Event, error)
	GetLimitedTimeMusics(musicID int) []*masterdata.LimitedTimeMusic
}
