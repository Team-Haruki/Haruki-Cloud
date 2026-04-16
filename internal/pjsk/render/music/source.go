package music

import (
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/region"
)

type DataSource interface {
	DefaultRegion() renderregion.Value
	SearchMusic(query string) (*masterdata.Music, error)
	GetMusicByID(id int) (*masterdata.Music, error)
	GetMusicByEventID(eventID int) (*masterdata.Music, error)
	GetMusics() []*masterdata.Music
	GetBanEvents(charID int) []*masterdata.Event
	GetMusicLocalizedTitles(musicID int) ([]string, error)
	GetMusicDifficulties(musicID int) ([]*masterdata.MusicDifficulty, error)
	GetMusicVocals(musicID int) ([]*masterdata.MusicVocal, error)
	GetMusicTags(musicID int) ([]string, error)
	GetCharacterByID(id int) (*masterdata.Character, error)
	GetOutsideCharacterByID(id int) (string, error)
	GetPrimaryEventByMusicID(musicID int) (*masterdata.Event, error)
	GetLimitedTimeMusics(musicID int) []*masterdata.LimitedTimeMusic
}
