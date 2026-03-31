package music

import (
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/provider"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

// ProviderAdapter bridges provider.MasterDataProvider to music.DataSource.
type ProviderAdapter struct {
	p provider.MasterDataProvider
}

func NewProviderAdapter(p provider.MasterDataProvider) *ProviderAdapter {
	return &ProviderAdapter{p: p}
}

func (a *ProviderAdapter) DefaultRegion() renderregion.Value { return a.p.Region() }

func (a *ProviderAdapter) SearchMusic(query string) (*masterdata.Music, error) {
	return a.p.Musics().Search(query)
}

func (a *ProviderAdapter) GetMusicByID(id int) (*masterdata.Music, error) {
	return a.p.Musics().GetByID(id)
}

func (a *ProviderAdapter) GetMusicByEventID(eventID int) (*masterdata.Music, error) {
	return a.p.Musics().GetByEventID(eventID)
}

func (a *ProviderAdapter) GetMusics() []*masterdata.Music {
	return a.p.Musics().GetAll()
}

func (a *ProviderAdapter) GetBanEvents(charID int) []*masterdata.Event {
	return a.p.Events().GetBanEvents(charID)
}

func (a *ProviderAdapter) GetMusicLocalizedTitles(musicID int) ([]string, error) {
	return a.p.Musics().GetLocalizedTitles(musicID)
}

func (a *ProviderAdapter) GetMusicDifficulties(musicID int) ([]*masterdata.MusicDifficulty, error) {
	return a.p.Musics().GetDifficulties(musicID)
}

func (a *ProviderAdapter) GetMusicVocals(musicID int) ([]*masterdata.MusicVocal, error) {
	return a.p.Musics().GetVocals(musicID)
}

func (a *ProviderAdapter) GetMusicTags(musicID int) ([]string, error) {
	return a.p.Musics().GetTags(musicID)
}

func (a *ProviderAdapter) GetCharacterByID(id int) (*masterdata.Character, error) {
	return a.p.Characters().GetByID(id)
}

func (a *ProviderAdapter) GetOutsideCharacterByID(id int) (string, error) {
	return a.p.Musics().GetOutsideCharacterByID(id)
}

func (a *ProviderAdapter) GetPrimaryEventByMusicID(musicID int) (*masterdata.Event, error) {
	return a.p.Musics().GetPrimaryEventByMusicID(musicID)
}

func (a *ProviderAdapter) GetLimitedTimeMusics(musicID int) []*masterdata.LimitedTimeMusic {
	return a.p.Musics().GetLimitedTimeMusics(musicID)
}
