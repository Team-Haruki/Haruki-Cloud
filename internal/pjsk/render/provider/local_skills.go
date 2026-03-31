package provider

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	"haruki-cloud/internal/pjsk/render/common"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

// ===========================================================================
// localSkillProvider
// ===========================================================================

type localSkillProvider struct {
	store      *localStore
	characters *localCharacterProvider

	once    sync.Once
	byID    map[int]*masterdata.Skill
	loadErr error
}

func (p *localSkillProvider) ensureLoaded() error {
	p.once.Do(func() {
		items, err := loadJSON[masterdata.Skill](p.store, "skills.json")
		if err != nil {
			p.loadErr = err
			return
		}
		p.byID = make(map[int]*masterdata.Skill, len(items))
		for i := range items {
			p.byID[items[i].ID] = &items[i]
		}
	})
	return p.loadErr
}

func (p *localSkillProvider) GetByID(id int) (*masterdata.Skill, error) {
	if id == 0 {
		return nil, nil
	}
	if err := p.ensureLoaded(); err != nil {
		return nil, err
	}
	s, ok := p.byID[id]
	if !ok {
		return nil, fmt.Errorf("skill %d not found", id)
	}
	return common.CloneSkill(s), nil
}

func (p *localSkillProvider) FormatDescription(skillInfo *masterdata.Skill, cardCharacterID int) string {
	if skillInfo == nil {
		return ""
	}
	return skillPlaceholder.ReplaceAllStringFunc(skillInfo.Description, func(match string) string {
		content := match[2 : len(match)-2]
		parts := strings.Split(content, ";")
		if len(parts) != 2 {
			return match
		}
		ids := make([]int, 0, 2)
		for _, rawID := range strings.Split(parts[0], ",") {
			value, err := strconv.Atoi(strings.TrimSpace(rawID))
			if err == nil {
				ids = append(ids, value)
			}
		}
		if len(ids) == 0 {
			return match
		}
		if parts[1] == "c" {
			if p.characters != nil {
				ch, err := p.characters.GetByID(cardCharacterID)
				if err == nil && ch != nil {
					return ch.FirstName + ch.GivenName
				}
			}
			return "???"
		}
		effects := make([]*masterdata.SkillEffect, 0, len(ids))
		for _, effectID := range ids {
			for idx := range skillInfo.SkillEffects {
				if skillInfo.SkillEffects[idx].ID == effectID {
					effects = append(effects, &skillInfo.SkillEffects[idx])
					break
				}
			}
		}
		if len(effects) != len(ids) {
			return "?"
		}
		if len(effects) == 1 {
			return formatSingleEffect(effects[0], parts[1])
		}
		if len(effects) == 2 {
			return formatDualEffects(effects[0], effects[1], parts[1])
		}
		return match
	})
}
