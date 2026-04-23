package provider

import (
	"context"
	"strings"
)

func (p *localEducationProvider) ensureRewards() error {
	return p.rewards.init(func() (map[int][]*ChallengeReward, error) {
		items, err := loadJSON[ChallengeReward](p.store, "challengeLiveHighScoreRewards.json")
		if err != nil {
			return nil, err
		}
		byChar := make(map[int][]*ChallengeReward)
		for i := range items {
			byChar[items[i].CharacterID] = append(byChar[items[i].CharacterID], &items[i])
		}
		return byChar, nil
	})
}

func (p *localEducationProvider) ensureResourceBoxes() error {
	return p.boxes.init(func() (boxIndex, error) {
		items, err := loadJSON[ResourceBox](p.store, "resourceBoxes.json")
		if err != nil {
			return boxIndex{}, err
		}
		idx := boxIndex{
			byID:      make(map[int]*ResourceBox, len(items)),
			byPurpose: make(map[string]map[int]*ResourceBox),
		}
		for i := range items {
			box := &items[i]
			idx.byID[box.ID] = box
			if _, ok := idx.byPurpose[box.ResourceBoxPurpose]; !ok {
				idx.byPurpose[box.ResourceBoxPurpose] = make(map[int]*ResourceBox)
			}
			idx.byPurpose[box.ResourceBoxPurpose][box.ID] = box
		}
		supplementResourceBoxDetailsFromStore(p.store, idx.byPurpose)
		return idx, nil
	})
}

func (p *localEducationProvider) GetChallengeRewardsByCharacter(_ context.Context, charID int) []*ChallengeReward {
	if charID <= 0 {
		return nil
	}
	if err := p.ensureRewards(); err != nil {
		return nil
	}
	return cloneEdChallengeRewards(p.rewards.v()[charID])
}

func (p *localEducationProvider) GetResourceBoxByPurpose(_ context.Context, purpose string, id int) *ResourceBox {
	if id <= 0 {
		return nil
	}
	if err := p.ensureResourceBoxes(); err != nil {
		return nil
	}
	if strings.TrimSpace(purpose) == "" {
		return cloneEdResourceBox(p.boxes.v().byID[id])
	}
	if purposeMap, ok := p.boxes.v().byPurpose[purpose]; ok {
		return cloneEdResourceBox(purposeMap[id])
	}
	return nil
}

func (p *localEducationProvider) GetResourceBoxesByPurpose(_ context.Context, purpose string) []*ResourceBox {
	if err := p.ensureResourceBoxes(); err != nil {
		return nil
	}
	if strings.TrimSpace(purpose) == "" {
		items := make([]*ResourceBox, 0, len(p.boxes.v().byID))
		for _, item := range p.boxes.v().byID {
			items = append(items, cloneEdResourceBox(item))
		}
		return items
	}
	purposeMap, ok := p.boxes.v().byPurpose[purpose]
	if !ok {
		return nil
	}
	items := make([]*ResourceBox, 0, len(purposeMap))
	for _, item := range purposeMap {
		items = append(items, cloneEdResourceBox(item))
	}
	return items
}
