package provider

import (
	"context"

	"haruki-cloud/internal/pjsk/render/masterdata"
)

type contextualEventProvider struct {
	base *dbEventProvider
	ctx  context.Context
}

func (p *contextualEventProvider) GetByID(id int) (*masterdata.Event, error) {
	return p.base.getByID(p.ctx, id)
}

func (p *contextualEventProvider) GetByCardID(cardID int) (*masterdata.Event, error) {
	return p.base.getByCardID(p.ctx, cardID)
}

func (p *contextualEventProvider) GetAll() []*masterdata.Event {
	return p.base.getAll(p.ctx)
}

func (p *contextualEventProvider) GetCards(eventID int) ([]*masterdata.Card, error) {
	return p.base.getCards(p.ctx, eventID)
}

func (p *contextualEventProvider) GetBannerCharacterID(eventID int) (int, error) {
	return p.base.getBannerCharacterID(p.ctx, eventID)
}

func (p *contextualEventProvider) GetDeckBonuses(eventID int) ([]*masterdata.EventDeckBonus, error) {
	return p.base.getDeckBonuses(p.ctx, eventID)
}

func (p *contextualEventProvider) GetBanEvents(charID int) []*masterdata.Event {
	return p.base.getBanEvents(p.ctx, charID)
}

func (p *contextualEventProvider) GetWorldBloomChapters(eventID int) []*masterdata.WorldBloom {
	return p.base.getWorldBloomChapters(p.ctx, eventID)
}

type contextualMusicProvider struct {
	base *dbMusicProvider
	ctx  context.Context
}

func (p *contextualMusicProvider) Search(query string) (*masterdata.Music, error) {
	return p.base.search(p.ctx, query)
}

func (p *contextualMusicProvider) GetByID(id int) (*masterdata.Music, error) {
	return p.base.getByID(p.ctx, id)
}

func (p *contextualMusicProvider) GetByEventID(eventID int) (*masterdata.Music, error) {
	return p.base.getByEventID(p.ctx, eventID)
}

func (p *contextualMusicProvider) GetAll() []*masterdata.Music {
	return p.base.getAll(p.ctx)
}

func (p *contextualMusicProvider) GetLocalizedTitles(musicID int) ([]string, error) {
	return p.base.getLocalizedTitles(p.ctx, musicID)
}

func (p *contextualMusicProvider) GetDifficulties(musicID int) ([]*masterdata.MusicDifficulty, error) {
	return p.base.getDifficulties(p.ctx, musicID)
}

func (p *contextualMusicProvider) GetVocals(musicID int) ([]*masterdata.MusicVocal, error) {
	return p.base.getVocals(p.ctx, musicID)
}

func (p *contextualMusicProvider) GetTags(musicID int) ([]string, error) {
	return p.base.getTags(p.ctx, musicID)
}

func (p *contextualMusicProvider) GetOutsideCharacterByID(id int) (string, error) {
	return p.base.getOutsideCharacterByID(p.ctx, id)
}

func (p *contextualMusicProvider) GetPrimaryEventByMusicID(musicID int) (*masterdata.Event, error) {
	return p.base.getPrimaryEventByMusicID(p.ctx, musicID)
}

func (p *contextualMusicProvider) GetLimitedTimeMusics(musicID int) []*masterdata.LimitedTimeMusic {
	return p.base.getLimitedTimeMusics(p.ctx, musicID)
}

type contextualGachaProvider struct {
	base *dbGachaProvider
	ctx  context.Context
}

func (p *contextualGachaProvider) GetByID(id int) (*masterdata.Gacha, error) {
	return p.base.getByID(p.ctx, id)
}

func (p *contextualGachaProvider) GetAll() []*masterdata.Gacha {
	return p.base.getAll(p.ctx)
}

func (p *contextualGachaProvider) GetCardByID(id int) (*masterdata.Card, error) {
	return p.base.getCardByID(p.ctx, id)
}
