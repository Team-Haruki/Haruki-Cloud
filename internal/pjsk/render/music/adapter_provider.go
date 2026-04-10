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
	return a.P.Musics().Search(a.Ctx, query)
}

func (a *ProviderAdapter) GetMusicByID(id int) (*masterdata.Music, error) {
	return a.P.Musics().GetByID(a.Ctx, id)
}

func (a *ProviderAdapter) GetMusicByEventID(eventID int) (*masterdata.Music, error) {
	return a.P.Musics().GetByEventID(a.Ctx, eventID)
}

func (a *ProviderAdapter) GetMusics() []*masterdata.Music {
	return a.P.Musics().GetAll(a.Ctx)
}

func (a *ProviderAdapter) GetBanEvents(charID int) []*masterdata.Event {
	return a.P.Events().GetBanEvents(a.Context(), charID)
}

func (a *ProviderAdapter) GetMusicLocalizedTitles(musicID int) ([]string, error) {
	return a.P.Musics().GetLocalizedTitles(a.Ctx, musicID)
}

func (a *ProviderAdapter) GetMusicDifficulties(musicID int) ([]*masterdata.MusicDifficulty, error) {
	return a.P.Musics().GetDifficulties(a.Ctx, musicID)
}

func (a *ProviderAdapter) GetMusicVocals(musicID int) ([]*masterdata.MusicVocal, error) {
	return a.P.Musics().GetVocals(a.Ctx, musicID)
}

func (a *ProviderAdapter) GetMusicTags(musicID int) ([]string, error) {
	return a.P.Musics().GetTags(a.Ctx, musicID)
}

func (a *ProviderAdapter) GetCharacterByID(id int) (*masterdata.Character, error) {
	return a.P.Characters().GetByID(a.Context(), id)
}

func (a *ProviderAdapter) GetOutsideCharacterByID(id int) (string, error) {
	return a.P.Musics().GetOutsideCharacterByID(a.Ctx, id)
}

func (a *ProviderAdapter) GetPrimaryEventByMusicID(musicID int) (*masterdata.Event, error) {
	return a.P.Musics().GetPrimaryEventByMusicID(a.Ctx, musicID)
}

func (a *ProviderAdapter) GetLimitedTimeMusics(musicID int) []*masterdata.LimitedTimeMusic {
	return a.P.Musics().GetLimitedTimeMusics(a.Ctx, musicID)
}
