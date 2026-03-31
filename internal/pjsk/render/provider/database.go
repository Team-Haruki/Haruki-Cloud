package provider

import (
	sekaiDB "haruki-cloud/database/sekai"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

// DatabaseProvider implements MasterDataProvider using a Sekai database client.
// It wraps the ent-generated query layer and caches results in memory.
type DatabaseProvider struct {
	client *sekaiDB.Client
	region renderregion.Value

	cards        *dbCardProvider
	characters   *dbCharacterProvider
	skills       *dbSkillProvider
	events       *dbEventProvider
	musics       *dbMusicProvider
	gachas       *dbGachaProvider
	honors       *dbHonorProvider
	stamps       *dbStampProvider
	vlives       *dbVLiveProvider
	education    *dbEducationProvider
	playerFrames *dbPlayerFrameProvider
	mysekai      *dbMySekaiProvider
}

// NewDatabaseProvider creates a MasterDataProvider backed by the Sekai
// PostgreSQL database. The region determines which server_region rows are
// queried. Pass the default region (typically JP).
func NewDatabaseProvider(client *sekaiDB.Client, region renderregion.Value) *DatabaseProvider {
	if client == nil {
		return nil
	}
	region = renderregion.WithDefault(region)
	p := &DatabaseProvider{
		client: client,
		region: region,
	}
	p.characters = &dbCharacterProvider{client: client, region: region}
	p.skills = &dbSkillProvider{client: client, region: region, characters: p.characters}
	p.cards = &dbCardProvider{client: client, region: region, characters: p.characters, skills: p.skills}
	p.events = &dbEventProvider{client: client, region: region}
	p.musics = &dbMusicProvider{client: client, region: region, events: p.events}
	p.gachas = &dbGachaProvider{client: client, region: region}
	p.honors = &dbHonorProvider{client: client, region: region}
	p.stamps = &dbStampProvider{client: client, region: region}
	p.vlives = &dbVLiveProvider{client: client, region: region}
	p.education = &dbEducationProvider{client: client, region: region}
	p.playerFrames = &dbPlayerFrameProvider{client: client, region: region}
	p.mysekai = &dbMySekaiProvider{client: client, region: region}
	return p
}

func (p *DatabaseProvider) Region() renderregion.Value { return p.region }

func (p *DatabaseProvider) Cards() CardProvider         { return p.cards }
func (p *DatabaseProvider) Characters() CharacterProvider { return p.characters }
func (p *DatabaseProvider) Skills() SkillProvider       { return p.skills }
func (p *DatabaseProvider) Events() EventProvider       { return p.events }
func (p *DatabaseProvider) Musics() MusicProvider       { return p.musics }
func (p *DatabaseProvider) Gachas() GachaProvider       { return p.gachas }
func (p *DatabaseProvider) Honors() HonorProvider       { return p.honors }
func (p *DatabaseProvider) Stamps() StampProvider       { return p.stamps }
func (p *DatabaseProvider) VLives() VLiveProvider       { return p.vlives }
func (p *DatabaseProvider) Education() EducationProvider { return p.education }
func (p *DatabaseProvider) PlayerFrames() PlayerFrameProvider { return p.playerFrames }
func (p *DatabaseProvider) MySekai() MySekaiProvider     { return p.mysekai }
