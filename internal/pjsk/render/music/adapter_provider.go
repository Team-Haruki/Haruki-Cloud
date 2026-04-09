package music

import (
	"context"

	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/provider"
)

// ProviderAdapter bridges provider.MasterDataProvider to music.DataSource.
type ProviderAdapter struct {
	provider.ProviderAdapterBase
}

func NewProviderAdapter(p provider.MasterDataProvider) *ProviderAdapter {
	return &ProviderAdapter{ProviderAdapterBase: provider.NewProviderAdapterBase(p)}
}

func (a *ProviderAdapter) WithContext(ctx context.Context) DataSource {
	if a == nil {
		return nil
	}
	return &ProviderAdapter{ProviderAdapterBase: a.CloneWithContext(ctx)}
}

func (a *ProviderAdapter) SearchMusic(query string) (*masterdata.Music, error) {
	return a.P.Musics().Search(query)
}

func (a *ProviderAdapter) GetMusicByID(id int) (*masterdata.Music, error) {
	return a.P.Musics().GetByID(id)
}

func (a *ProviderAdapter) GetMusicByEventID(eventID int) (*masterdata.Music, error) {
	return a.P.Musics().GetByEventID(eventID)
}

func (a *ProviderAdapter) GetMusics() []*masterdata.Music {
	return a.P.Musics().GetAll()
}

func (a *ProviderAdapter) GetBanEvents(charID int) []*masterdata.Event {
	return a.P.Events().GetBanEvents(charID)
}

func (a *ProviderAdapter) GetMusicLocalizedTitles(musicID int) ([]string, error) {
	return a.P.Musics().GetLocalizedTitles(musicID)
}

func (a *ProviderAdapter) GetMusicDifficulties(musicID int) ([]*masterdata.MusicDifficulty, error) {
	return a.P.Musics().GetDifficulties(musicID)
}

func (a *ProviderAdapter) GetMusicVocals(musicID int) ([]*masterdata.MusicVocal, error) {
	return a.P.Musics().GetVocals(musicID)
}

func (a *ProviderAdapter) GetMusicTags(musicID int) ([]string, error) {
	return a.P.Musics().GetTags(musicID)
}

func (a *ProviderAdapter) GetCharacterByID(id int) (*masterdata.Character, error) {
	return a.P.Characters().GetByID(id)
}

func (a *ProviderAdapter) GetOutsideCharacterByID(id int) (string, error) {
	return a.P.Musics().GetOutsideCharacterByID(id)
}

func (a *ProviderAdapter) GetPrimaryEventByMusicID(musicID int) (*masterdata.Event, error) {
	return a.P.Musics().GetPrimaryEventByMusicID(musicID)
}

func (a *ProviderAdapter) GetLimitedTimeMusics(musicID int) []*masterdata.LimitedTimeMusic {
	return a.P.Musics().GetLimitedTimeMusics(musicID)
}
