package provider

import (
	"context"
	"fmt"

	"haruki-cloud/internal/pjsk/render/common"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

// ===========================================================================
// localSkillProvider
// ===========================================================================

type localSkillProvider struct {
	store      *localStore
	characters *localCharacterProvider

	skills lazyValue[map[int]*masterdata.Skill]
}

func (p *localSkillProvider) ensureLoaded() error {
	return p.skills.init(func() (map[int]*masterdata.Skill, error) {
		items, err := p.store.loadJSON[masterdata.Skill]("skills.json")
		if err != nil {
			return nil, err
		}
		byID := make(map[int]*masterdata.Skill, len(items))
		for i := range items {
			byID[items[i].ID] = &items[i]
		}
		return byID, nil
	})
}

func (p *localSkillProvider) GetByID(_ context.Context, id int) (*masterdata.Skill, error) {
	if id == 0 {
		return nil, nil
	}
	if err := p.ensureLoaded(); err != nil {
		return nil, err
	}
	s, ok := p.skills.v()[id]
	if !ok {
		return nil, fmt.Errorf("skill %d not found", id)
	}
	return common.CloneSkill(s), nil
}

func (p *localSkillProvider) FormatDescription(ctx context.Context, skillInfo *masterdata.Skill, cardCharacterID int) string {
	var characterLookup skillCharacterLookup
	if p.characters != nil {
		characterLookup = p.characters.GetByID
	}
	return formatSkillDescription(ctx, skillInfo, cardCharacterID, characterLookup)
}
