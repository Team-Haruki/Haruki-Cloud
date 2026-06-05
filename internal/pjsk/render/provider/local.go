package provider

import (
	renderregion "haruki-cloud/internal/pjsk/region"
)

// LocalProvider implements MasterDataProvider using local JSON files.
type LocalProvider struct {
	store  *localStore
	region renderregion.Value

	cards        *localCardProvider
	characters   *localCharacterProvider
	skills       *localSkillProvider
	events       *localEventProvider
	musics       *localMusicProvider
	gachas       *localGachaProvider
	costumes     *localCostumeProvider
	honors       *localHonorProvider
	stamps       *localStampProvider
	vlives       *localVLiveProvider
	education    *localEducationProvider
	playerFrames *localPlayerFrameProvider
	mysekai      *localMySekaiProvider
}

// NewLocalProvider creates a MasterDataProvider backed by local JSON files.
// dir is the directory containing the JSON master data files
// (e.g. "data/master/haruki-sekai-master/master").
func NewLocalProvider(dir string, region renderregion.Value) *LocalProvider {
	region = renderregion.WithDefault(region)
	store := newLocalStore(dir)

	p := &LocalProvider{
		store:  store,
		region: region,
	}
	p.characters = &localCharacterProvider{store: store}
	p.skills = &localSkillProvider{store: store, characters: p.characters}
	p.cards = &localCardProvider{store: store, characters: p.characters, skills: p.skills}
	p.events = &localEventProvider{store: store, cards: p.cards}
	p.musics = &localMusicProvider{store: store, events: p.events}
	p.gachas = &localGachaProvider{store: store}
	p.costumes = &localCostumeProvider{store: store}
	p.honors = &localHonorProvider{store: store}
	p.stamps = &localStampProvider{store: store}
	p.vlives = &localVLiveProvider{store: store}
	p.education = &localEducationProvider{store: store}
	p.playerFrames = &localPlayerFrameProvider{store: store}
	p.mysekai = &localMySekaiProvider{store: store}
	return p
}

func (p *LocalProvider) Region() renderregion.Value        { return p.region }
func (p *LocalProvider) Cards() CardProvider               { return p.cards }
func (p *LocalProvider) Characters() CharacterProvider     { return p.characters }
func (p *LocalProvider) Skills() SkillProvider             { return p.skills }
func (p *LocalProvider) Events() EventProvider             { return p.events }
func (p *LocalProvider) Musics() MusicProvider             { return p.musics }
func (p *LocalProvider) Gachas() GachaProvider             { return p.gachas }
func (p *LocalProvider) Costumes() CostumeProvider         { return p.costumes }
func (p *LocalProvider) Honors() HonorProvider             { return p.honors }
func (p *LocalProvider) Stamps() StampProvider             { return p.stamps }
func (p *LocalProvider) VLives() VLiveProvider             { return p.vlives }
func (p *LocalProvider) Education() EducationProvider      { return p.education }
func (p *LocalProvider) PlayerFrames() PlayerFrameProvider { return p.playerFrames }
func (p *LocalProvider) MySekai() MySekaiProvider          { return p.mysekai }
