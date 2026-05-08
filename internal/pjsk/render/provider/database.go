package provider

import (
	"path/filepath"
	"strings"

	sekaiDB "haruki-cloud/database/sekai"
	renderregion "haruki-cloud/internal/pjsk/region"
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

func (p *DatabaseProvider) SetLocalMasterdataDir(root string, allowLeaks bool) {
	if p == nil {
		return
	}

	root = strings.TrimSpace(root)
	if root == "" {
		if p.education != nil {
			p.education.store = nil
		}
		if p.musics != nil {
			p.musics.local = nil
		}
		if p.events != nil {
			p.events.local = nil
		}
		if p.mysekai != nil {
			p.mysekai.local = nil
		}
		return
	}

	dirs := localMasterdataCandidateDirs(root, p.region)
	if len(dirs) == 0 {
		if p.education != nil {
			p.education.store = nil
		}
		if p.musics != nil {
			p.musics.local = nil
		}
		if p.events != nil {
			p.events.local = nil
		}
		if p.mysekai != nil {
			p.mysekai.local = nil
		}
		return
	}
	store := newLocalStore(dirs...)
	if p.education != nil {
		p.education.store = store
	}
	if p.musics != nil {
		p.musics.local = &localMusicProvider{store: store, events: p.events}
	}
	if p.mysekai != nil {
		p.mysekai.local = &localMySekaiProvider{store: store}
	}

	if !allowLeaks {
		if p.events != nil {
			p.events.local = nil
		}
		return
	}

	localCharacters := &localCharacterProvider{store: store}
	localSkills := &localSkillProvider{store: store, characters: localCharacters}
	localCards := &localCardProvider{store: store, characters: localCharacters, skills: localSkills}
	if p.events != nil {
		p.events.local = &localEventProvider{store: store, cards: localCards}
	}
}

func (p *DatabaseProvider) Region() renderregion.Value { return p.region }

func (p *DatabaseProvider) Cards() CardProvider               { return p.cards }
func (p *DatabaseProvider) Characters() CharacterProvider     { return p.characters }
func (p *DatabaseProvider) Skills() SkillProvider             { return p.skills }
func (p *DatabaseProvider) Events() EventProvider             { return p.events }
func (p *DatabaseProvider) Musics() MusicProvider             { return p.musics }
func (p *DatabaseProvider) Gachas() GachaProvider             { return p.gachas }
func (p *DatabaseProvider) Honors() HonorProvider             { return p.honors }
func (p *DatabaseProvider) Stamps() StampProvider             { return p.stamps }
func (p *DatabaseProvider) VLives() VLiveProvider             { return p.vlives }
func (p *DatabaseProvider) Education() EducationProvider      { return p.education }
func (p *DatabaseProvider) PlayerFrames() PlayerFrameProvider { return p.playerFrames }
func (p *DatabaseProvider) MySekai() MySekaiProvider          { return p.mysekai }

var localMasterdataRepoDirs = map[renderregion.Value]string{
	renderregion.JP: "haruki-sekai-master",
	renderregion.CN: "haruki-sekai-sc-master",
	renderregion.TW: "haruki-sekai-tc-master",
	renderregion.KR: "haruki-sekai-kr-master",
	renderregion.EN: "haruki-sekai-en-master",
}

func localMasterdataCandidateDirs(root string, region renderregion.Value) []string {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" {
		return nil
	}

	region = renderregion.WithDefault(region)
	targetRegion := region.String()
	repoDir := localMasterdataRepoDirs[region]

	result := make([]string, 0, 12)
	seen := make(map[string]struct{}, 12)
	add := func(dir string) {
		dir = strings.TrimSpace(dir)
		if dir == "" || dir == "." || dir == "/" {
			return
		}
		dir = filepath.Clean(dir)
		if _, ok := seen[dir]; ok {
			return
		}
		seen[dir] = struct{}{}
		result = append(result, dir)
	}

	add(filepath.Join(root, targetRegion))
	if repoDir != "" {
		add(filepath.Join(root, repoDir, "master"))
	}
	if inferred, ok := inferLocalMasterdataDirRegion(root); !ok || inferred == region {
		add(root)
	}

	current := root
	for depth := 0; depth < 4; depth++ {
		parent := filepath.Dir(current)
		if parent == "" || parent == "." || parent == "/" || parent == current {
			break
		}
		current = parent
		add(filepath.Join(current, targetRegion))
		if repoDir != "" {
			add(filepath.Join(current, repoDir, "master"))
		}
	}

	return result
}

func inferLocalMasterdataDirRegion(dir string) (renderregion.Value, bool) {
	base := strings.ToLower(strings.TrimSpace(filepath.Base(dir)))
	if region, ok := parseMasterdataRegion(base); ok {
		return region, true
	}
	if region, ok := regionForLocalMasterdataRepo(base); ok {
		return region, true
	}
	if base == "master" {
		parent := strings.ToLower(strings.TrimSpace(filepath.Base(filepath.Dir(dir))))
		if region, ok := regionForLocalMasterdataRepo(parent); ok {
			return region, true
		}
	}
	return renderregion.Unknown, false
}

func parseMasterdataRegion(value string) (renderregion.Value, bool) {
	switch renderregion.Normalize(value) {
	case renderregion.JP:
		return renderregion.JP, true
	case renderregion.CN:
		return renderregion.CN, true
	case renderregion.TW:
		return renderregion.TW, true
	case renderregion.KR:
		return renderregion.KR, true
	case renderregion.EN:
		return renderregion.EN, true
	default:
		return renderregion.Unknown, false
	}
}

func regionForLocalMasterdataRepo(name string) (renderregion.Value, bool) {
	for region, repoDir := range localMasterdataRepoDirs {
		if strings.EqualFold(strings.TrimSpace(name), repoDir) {
			return region, true
		}
	}
	return renderregion.Unknown, false
}
