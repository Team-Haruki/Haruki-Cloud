package provider

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/common"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

// ===========================================================================
// localHonorProvider
// ===========================================================================

type localHonorProvider struct {
	store *localStore

	honorOnce sync.Once
	honorByID map[int]*masterdata.Honor
	honorErr  error

	groupOnce sync.Once
	groupByID map[int]*masterdata.HonorGroup
	groupErr  error

	bondsOnce sync.Once
	bondsByID map[int]*masterdata.BondsHonor
	bondsErr  error

	gcuOnce sync.Once
	gcuByID map[int]*masterdata.GameCharacterUnit
	gcuErr  error

	birthdayOnce    sync.Once
	birthdayByGroup map[int]honorBirthdayAssets
	birthdayChars   []localGameCharacterJSON

	eventHonorOnce   sync.Once
	eventByHonorID   map[int]int
	eventHonorLoaded bool
}

func (p *localHonorProvider) ensureHonors() error {
	p.honorOnce.Do(func() {
		items, err := loadJSON[masterdata.Honor](p.store, "honors.json")
		if err != nil {
			p.honorErr = err
			return
		}
		p.honorByID = make(map[int]*masterdata.Honor, len(items))
		for i := range items {
			p.honorByID[items[i].ID] = &items[i]
		}
	})
	return p.honorErr
}

func (p *localHonorProvider) ensureGroups() error {
	p.groupOnce.Do(func() {
		items, err := loadJSON[masterdata.HonorGroup](p.store, "honorGroups.json")
		if err != nil {
			p.groupErr = err
			return
		}
		p.groupByID = make(map[int]*masterdata.HonorGroup, len(items))
		for i := range items {
			p.groupByID[items[i].ID] = &items[i]
		}
	})
	return p.groupErr
}

func (p *localHonorProvider) ensureBonds() error {
	p.bondsOnce.Do(func() {
		items, err := loadJSON[masterdata.BondsHonor](p.store, "bondsHonors.json")
		if err != nil {
			p.bondsErr = err
			return
		}
		p.bondsByID = make(map[int]*masterdata.BondsHonor, len(items))
		for i := range items {
			p.bondsByID[items[i].ID] = &items[i]
		}
	})
	return p.bondsErr
}

func (p *localHonorProvider) ensureGCU() error {
	p.gcuOnce.Do(func() {
		items, err := loadJSON[masterdata.GameCharacterUnit](p.store, "gameCharacterUnits.json")
		if err != nil {
			p.gcuErr = err
			return
		}
		p.gcuByID = make(map[int]*masterdata.GameCharacterUnit, len(items))
		for i := range items {
			p.gcuByID[items[i].ID] = &items[i]
		}
	})
	return p.gcuErr
}

func (p *localHonorProvider) ensureBirthdayChars() {
	p.birthdayOnce.Do(func() {
		p.birthdayByGroup = make(map[int]honorBirthdayAssets)
		chars, err := loadJSON[localGameCharacterJSON](p.store, "gameCharacters.json")
		if err == nil {
			p.birthdayChars = chars
		}
	})
}

func (p *localHonorProvider) ensureEventHonors() {
	p.eventHonorOnce.Do(func() {
		p.eventByHonorID = make(map[int]int)
		items, err := loadJSON[localEventJSON](p.store, "events.json")
		if err != nil {
			return
		}
		for _, item := range items {
			var ranges []honorRewardRange
			if err := json.Unmarshal(item.EventRankingRewardRanges, &ranges); err != nil {
				continue
			}
			for _, rr := range ranges {
				for _, detail := range rr.EventRankingRewardDetails {
					if detail.ResourceType == "honor" && detail.ResourceID > 0 {
						p.eventByHonorID[detail.ResourceID] = item.ID
					}
				}
			}
		}
		p.eventHonorLoaded = true
	})
}

func (p *localHonorProvider) GetByID(id int) (*masterdata.Honor, error) {
	if id == 0 {
		return nil, fmt.Errorf("invalid honor id")
	}
	if err := p.ensureHonors(); err != nil {
		return nil, err
	}
	h, ok := p.honorByID[id]
	if !ok {
		return nil, fmt.Errorf("honor %d not found", id)
	}
	return common.CloneHonor(h), nil
}

func (p *localHonorProvider) GetGroupByID(id int) (*masterdata.HonorGroup, error) {
	if id == 0 {
		return nil, fmt.Errorf("invalid honor group id")
	}
	if err := p.ensureGroups(); err != nil {
		return nil, err
	}
	g, ok := p.groupByID[id]
	if !ok {
		return nil, fmt.Errorf("honor group %d not found", id)
	}
	model := common.CloneHonorGroup(g)

	if model.HonorType == "birthday" && (model.BackgroundAssetBundleName == nil || model.FrameName == nil) {
		if derived, ok := p.deriveBirthdayAssetsForGroup(id, model.Name); ok {
			if model.BackgroundAssetBundleName == nil && derived.background != "" {
				v := derived.background
				model.BackgroundAssetBundleName = &v
			}
			if model.FrameName == nil && derived.frame != "" {
				v := derived.frame
				model.FrameName = &v
			}
			slog.Info("honor birthday derive trace",
				"group_id", id,
				"group_name", model.Name,
				"derived_background_assetbundle_name", derived.background,
				"derived_frame_name", derived.frame)
		}
	}
	return model, nil
}

func (p *localHonorProvider) deriveBirthdayAssetsForGroup(groupID int, groupName string) (honorBirthdayAssets, bool) {
	p.ensureBirthdayChars()

	if cached, ok := p.birthdayByGroup[groupID]; ok {
		return cached, true
	}
	for _, ch := range p.birthdayChars {
		if ch.ID <= 0 {
			continue
		}
		if !localBirthdayGroupMatchesCharacter(groupName, &ch) {
			continue
		}
		suffix := fmt.Sprintf("01_%02d", ch.ID)
		derived := honorBirthdayAssets{
			background: "honor_bg_birthday_" + suffix,
			frame:      "honor_frame_birthday_" + suffix,
		}
		p.birthdayByGroup[groupID] = derived
		return derived, true
	}
	return honorBirthdayAssets{}, false
}

func localBirthdayGroupMatchesCharacter(groupName string, ch *localGameCharacterJSON) bool {
	if ch == nil {
		return false
	}
	name := strings.TrimSpace(groupName)
	if name == "" {
		return false
	}
	candidates := []string{
		strings.TrimSpace(ch.FirstName),
		strings.TrimSpace(ch.GivenName),
		strings.TrimSpace(ch.FirstName + ch.GivenName),
		strings.TrimSpace(ch.FirstNameEnglish),
		strings.TrimSpace(ch.GivenNameEnglish),
		strings.TrimSpace(ch.FirstNameEnglish + ch.GivenNameEnglish),
	}
	for _, c := range candidates {
		if c != "" && strings.Contains(name, c) {
			return true
		}
	}
	if nickname, ok := assets.CharacterIDToNickname[ch.ID]; ok && nickname != "" {
		if strings.Contains(strings.ToLower(name), strings.ToLower(strings.TrimSpace(nickname))) {
			return true
		}
	}
	return false
}

func (p *localHonorProvider) GetBondsHonorByID(id int) (*masterdata.BondsHonor, error) {
	if id == 0 {
		return nil, fmt.Errorf("invalid bonds honor id")
	}
	if err := p.ensureBonds(); err != nil {
		return nil, err
	}
	b, ok := p.bondsByID[id]
	if !ok {
		return nil, fmt.Errorf("bonds honor %d not found", id)
	}
	return common.CloneBondsHonor(b), nil
}

func (p *localHonorProvider) GetGameCharacterUnitByID(id int) (*masterdata.GameCharacterUnit, bool) {
	if id == 0 {
		return nil, false
	}
	if err := p.ensureGCU(); err != nil {
		return nil, false
	}
	u, ok := p.gcuByID[id]
	if !ok {
		return nil, false
	}
	return common.CloneGameCharacterUnit(u), true
}

func (p *localHonorProvider) GetEventIDByHonorID(honorID int) int {
	if honorID == 0 {
		return 0
	}
	p.ensureEventHonors()
	return p.eventByHonorID[honorID]
}
