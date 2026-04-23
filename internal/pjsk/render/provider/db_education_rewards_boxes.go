package provider

import (
	"context"
	json "github.com/bytedance/sonic"
	"slices"
	"strings"

	"haruki-cloud/database/sekai/challengelivehighscorereward"
	"haruki-cloud/database/sekai/resourceboxe"
)

func (p *dbEducationProvider) GetChallengeRewardsByCharacter(ctx context.Context, charID int) []*ChallengeReward {
	if charID <= 0 {
		return nil
	}
	p.init()

	p.rewardMu.RLock()
	if p.rewardsLoaded {
		out := cloneEdChallengeRewards(p.rewardsByChar[charID])
		p.rewardMu.RUnlock()
		return out
	}
	p.rewardMu.RUnlock()

	p.rewardMu.Lock()
	defer p.rewardMu.Unlock()

	if !p.rewardsLoaded {
		items, err := p.client.Challengelivehighscorereward.Query().
			Where(challengelivehighscorereward.ServerRegionEQ(p.region.String())).
			All(ctx)
		if err != nil {
			return nil
		}
		for _, item := range items {
			reward := &ChallengeReward{
				ID:            int(item.GameID),
				CharacterID:   int(item.CharacterID),
				HighScore:     int(item.HighScore),
				ResourceBoxID: int(item.ResourceBoxID),
			}
			p.rewardsByChar[reward.CharacterID] = append(p.rewardsByChar[reward.CharacterID], reward)
		}
		p.rewardsLoaded = true
	}

	return cloneEdChallengeRewards(p.rewardsByChar[charID])
}

func (p *dbEducationProvider) GetResourceBoxByPurpose(ctx context.Context, purpose string, id int) *ResourceBox {
	if id <= 0 {
		return nil
	}
	if !p.ensureResourceBoxesLoaded(ctx) {
		return nil
	}

	p.boxMu.RLock()
	defer p.boxMu.RUnlock()

	if strings.TrimSpace(purpose) == "" {
		return cloneEdResourceBox(p.boxByID[id])
	}
	if purposeMap, ok := p.boxByPurpose[purpose]; ok {
		return cloneEdResourceBox(purposeMap[id])
	}
	return nil
}

func (p *dbEducationProvider) GetResourceBoxesByPurpose(ctx context.Context, purpose string) []*ResourceBox {
	if !p.ensureResourceBoxesLoaded(ctx) {
		return nil
	}

	p.boxMu.RLock()
	defer p.boxMu.RUnlock()

	if strings.TrimSpace(purpose) == "" {
		items := make([]*ResourceBox, 0, len(p.boxByID))
		for _, item := range p.boxByID {
			items = append(items, cloneEdResourceBox(item))
		}
		return items
	}

	purposeMap, ok := p.boxByPurpose[purpose]
	if !ok {
		return nil
	}
	items := make([]*ResourceBox, 0, len(purposeMap))
	for _, item := range purposeMap {
		items = append(items, cloneEdResourceBox(item))
	}
	return items
}

func (p *dbEducationProvider) ensureResourceBoxesLoaded(ctx context.Context) bool {
	p.init()
	p.boxMu.RLock()
	if p.boxesLoaded {
		p.boxMu.RUnlock()
		return true
	}
	p.boxMu.RUnlock()

	p.boxMu.Lock()
	defer p.boxMu.Unlock()

	if p.boxesLoaded {
		return true
	}

	items, err := p.client.Resourceboxe.Query().
		Where(resourceboxe.ServerRegionEQ(p.region.String())).
		All(ctx)
	if err != nil {
		return false
	}
	for _, item := range items {
		box := &ResourceBox{
			ID:                 int(item.GameID),
			ResourceBoxPurpose: item.ResourceBoxPurpose,
			ResourceBoxType:    item.ResourceBoxType,
			Description:        item.Description,
		}
		if len(item.Details) > 0 {
			var details []ResourceBoxDetail
			if err := json.Unmarshal(item.Details, &details); err != nil {
				continue
			}
			box.Details = details
		}
		p.boxByID[box.ID] = box
		if _, ok := p.boxByPurpose[box.ResourceBoxPurpose]; !ok {
			p.boxByPurpose[box.ResourceBoxPurpose] = make(map[int]*ResourceBox)
		}
		p.boxByPurpose[box.ResourceBoxPurpose][box.ID] = box
	}
	supplementResourceBoxDetailsFromStore(p.store, p.boxByPurpose)
	p.boxesLoaded = true
	return true
}

func cloneEdChallengeRewards(source []*ChallengeReward) []*ChallengeReward {
	if len(source) == 0 {
		return nil
	}
	out := make([]*ChallengeReward, 0, len(source))
	for _, item := range source {
		if item == nil {
			continue
		}
		out = append(out, new(*item))
	}
	return out
}

func cloneEdResourceBox(source *ResourceBox) *ResourceBox {
	if source == nil {
		return nil
	}
	c := *source
	c.Details = slices.Clone(source.Details)
	return &c
}
