package provider

import (
	"context"
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

type honorBirthdayData struct {
	byGroup map[int]honorBirthdayAssets
	chars   []localGameCharacterJSON
}

type localHonorProvider struct {
	store *localStore

	honors      lazyValue[map[int]*masterdata.Honor]
	groups      lazyValue[map[int]*masterdata.HonorGroup]
	bondsHonors lazyValue[map[int]*masterdata.BondsHonor]
	gcu         lazyValue[map[int]*masterdata.GameCharacterUnit]
	birthday    lazyValue[honorBirthdayData]
	eventHonors lazyValue[map[int]int]

	// birthdayMu guards mutation of birthday.v().byGroup cache
	birthdayMu sync.Mutex
}

func (p *localHonorProvider) ensureHonors() error {
	return p.honors.init(func() (map[int]*masterdata.Honor, error) {
		items, err := loadJSON[masterdata.Honor](p.store, "honors.json")
		if err != nil {
			return nil, err
		}
		byID := make(map[int]*masterdata.Honor, len(items))
		for i := range items {
			byID[items[i].ID] = &items[i]
		}
		return byID, nil
	})
}

func (p *localHonorProvider) ensureGroups() error {
	return p.groups.init(func() (map[int]*masterdata.HonorGroup, error) {
		items, err := loadJSON[masterdata.HonorGroup](p.store, "honorGroups.json")
		if err != nil {
			return nil, err
		}
		byID := make(map[int]*masterdata.HonorGroup, len(items))
		for i := range items {
			byID[items[i].ID] = &items[i]
		}
		return byID, nil
	})
}

func (p *localHonorProvider) ensureBonds() error {
	return p.bondsHonors.init(func() (map[int]*masterdata.BondsHonor, error) {
		items, err := loadJSON[masterdata.BondsHonor](p.store, "bondsHonors.json")
		if err != nil {
			return nil, err
		}
		byID := make(map[int]*masterdata.BondsHonor, len(items))
		for i := range items {
			byID[items[i].ID] = &items[i]
		}
		return byID, nil
	})
}

func (p *localHonorProvider) ensureGCU() error {
	return p.gcu.init(func() (map[int]*masterdata.GameCharacterUnit, error) {
		items, err := loadJSON[masterdata.GameCharacterUnit](p.store, "gameCharacterUnits.json")
		if err != nil {
			return nil, err
		}
		byID := make(map[int]*masterdata.GameCharacterUnit, len(items))
		for i := range items {
			byID[items[i].ID] = &items[i]
		}
		return byID, nil
	})
}

func (p *localHonorProvider) ensureBirthdayChars() {
	_ = p.birthday.init(func() (honorBirthdayData, error) {
		data := honorBirthdayData{byGroup: make(map[int]honorBirthdayAssets)}
		chars, err := loadJSON[localGameCharacterJSON](p.store, "gameCharacters.json")
		if err == nil {
			data.chars = chars
		}
		return data, nil
	})
}

func (p *localHonorProvider) ensureEventHonors() {
	_ = p.eventHonors.init(func() (map[int]int, error) {
		byHonorID := make(map[int]int)
		items, err := loadJSON[localEventJSON](p.store, "events.json")
		if err != nil {
			return byHonorID, nil
		}
		for _, item := range items {
			var ranges []honorRewardRange
			if err := json.Unmarshal(item.EventRankingRewardRanges, &ranges); err != nil {
				continue
			}
			for _, rr := range ranges {
				for _, detail := range rr.EventRankingRewardDetails {
					if detail.ResourceType == "honor" && detail.ResourceID > 0 {
						byHonorID[detail.ResourceID] = item.ID
					}
				}
			}
		}
		return byHonorID, nil
	})
}

func (p *localHonorProvider) GetByID(_ context.Context, id int) (*masterdata.Honor, error) {
	if id == 0 {
		return nil, fmt.Errorf("invalid honor id")
	}
	if err := p.ensureHonors(); err != nil {
		return nil, err
	}
	h, ok := p.honors.v()[id]
	if !ok {
		return nil, fmt.Errorf("honor %d not found", id)
	}
	return common.CloneHonor(h), nil
}

func (p *localHonorProvider) GetGroupByID(_ context.Context, id int) (*masterdata.HonorGroup, error) {
	if id == 0 {
		return nil, fmt.Errorf("invalid honor group id")
	}
	if err := p.ensureGroups(); err != nil {
		return nil, err
	}
	g, ok := p.groups.v()[id]
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

	p.birthdayMu.Lock()
	defer p.birthdayMu.Unlock()

	bd := p.birthday.v()
	if cached, ok := bd.byGroup[groupID]; ok {
		return cached, true
	}
	for _, ch := range bd.chars {
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
		p.birthday.v().byGroup[groupID] = derived
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

func (p *localHonorProvider) GetBondsHonorByID(_ context.Context, id int) (*masterdata.BondsHonor, error) {
	if id == 0 {
		return nil, fmt.Errorf("invalid bonds honor id")
	}
	if err := p.ensureBonds(); err != nil {
		return nil, err
	}
	b, ok := p.bondsHonors.v()[id]
	if !ok {
		return nil, fmt.Errorf("bonds honor %d not found", id)
	}
	return common.CloneBondsHonor(b), nil
}

func (p *localHonorProvider) GetGameCharacterUnitByID(_ context.Context, id int) (*masterdata.GameCharacterUnit, bool) {
	if id == 0 {
		return nil, false
	}
	if err := p.ensureGCU(); err != nil {
		return nil, false
	}
	u, ok := p.gcu.v()[id]
	if !ok {
		return nil, false
	}
	return common.CloneGameCharacterUnit(u), true
}

func (p *localHonorProvider) GetEventIDByHonorID(_ context.Context, honorID int) int {
	if honorID == 0 {
		return 0
	}
	p.ensureEventHonors()
	return p.eventHonors.v()[honorID]
}
