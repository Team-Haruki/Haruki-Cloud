package provider

import (
	"context"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

type ContextualMasterDataProvider interface {
	MasterDataProvider
	WithContext(ctx context.Context) MasterDataProvider
}

func WithContext(p MasterDataProvider, ctx context.Context) MasterDataProvider {
	if p == nil {
		return nil
	}
	if contextual, ok := p.(ContextualMasterDataProvider); ok {
		return contextual.WithContext(ctx)
	}
	return p
}

func (p *LocalProvider) WithContext(context.Context) MasterDataProvider {
	return p
}

func (p *DatabaseProvider) WithContext(ctx context.Context) MasterDataProvider {
	if p == nil || ctx == nil {
		return p
	}
	return &contextualDatabaseProvider{
		base: p,
		ctx:  ctx,
	}
}

type contextualDatabaseProvider struct {
	base *DatabaseProvider
	ctx  context.Context
}

func (p *contextualDatabaseProvider) Region() renderregion.Value {
	return p.base.Region()
}

func (p *contextualDatabaseProvider) Cards() CardProvider {
	return &contextualCardProvider{base: p.base.cards, ctx: p.ctx}
}

func (p *contextualDatabaseProvider) Characters() CharacterProvider {
	return &contextualCharacterProvider{base: p.base.characters, ctx: p.ctx}
}

func (p *contextualDatabaseProvider) Skills() SkillProvider {
	return &contextualSkillProvider{base: p.base.skills, ctx: p.ctx}
}

func (p *contextualDatabaseProvider) Events() EventProvider {
	return &contextualEventProvider{base: p.base.events, ctx: p.ctx}
}

func (p *contextualDatabaseProvider) Musics() MusicProvider {
	return &contextualMusicProvider{base: p.base.musics, ctx: p.ctx}
}

func (p *contextualDatabaseProvider) Gachas() GachaProvider {
	return &contextualGachaProvider{base: p.base.gachas, ctx: p.ctx}
}

func (p *contextualDatabaseProvider) Honors() HonorProvider {
	return &contextualHonorProvider{base: p.base.honors, ctx: p.ctx}
}

func (p *contextualDatabaseProvider) Stamps() StampProvider {
	return &contextualStampProvider{base: p.base.stamps, ctx: p.ctx}
}

func (p *contextualDatabaseProvider) VLives() VLiveProvider {
	return &contextualVLiveProvider{base: p.base.vlives, ctx: p.ctx}
}

func (p *contextualDatabaseProvider) Education() EducationProvider {
	return &contextualEducationProvider{base: p.base.education, ctx: p.ctx}
}

func (p *contextualDatabaseProvider) PlayerFrames() PlayerFrameProvider {
	return &contextualPlayerFrameProvider{base: p.base.playerFrames, ctx: p.ctx}
}

func (p *contextualDatabaseProvider) MySekai() MySekaiProvider {
	return p.base.mysekai
}
