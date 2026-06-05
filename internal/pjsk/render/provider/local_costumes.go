package provider

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"haruki-cloud/internal/pjsk/render/common"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

type costumeIndex struct {
	all         []*masterdata.Costume3d
	byID        map[int]*masterdata.Costume3d
	sourceCards map[int][]int
}

type localCostumeProvider struct {
	store    *localStore
	costumes lazyValue[costumeIndex]
}

func (p *localCostumeProvider) ensureCostumes() error {
	return p.costumes.init(func() (costumeIndex, error) {
		raw, err := loadJSON[localCostume3dJSON](p.store, "costume3ds.json")
		if err != nil {
			return costumeIndex{}, err
		}
		idx := costumeIndex{
			all:         make([]*masterdata.Costume3d, 0, len(raw)),
			byID:        make(map[int]*masterdata.Costume3d, len(raw)),
			sourceCards: make(map[int][]int),
		}
		for i := range raw {
			model := raw[i].toModel()
			if model == nil || model.ID <= 0 {
				continue
			}
			idx.byID[model.ID] = model
			idx.all = append(idx.all, model)
		}
		sortCostumesForList(idx.all)

		links, err := loadJSON[localCardCostume3dJSON](p.store, "cardCostume3ds.json")
		if err == nil {
			for _, link := range links {
				if link.Costume3dID <= 0 || link.CardID <= 0 {
					continue
				}
				idx.sourceCards[link.Costume3dID] = append(idx.sourceCards[link.Costume3dID], link.CardID)
			}
			for costumeID := range idx.sourceCards {
				sort.Ints(idx.sourceCards[costumeID])
			}
		}
		return idx, nil
	})
}

func (p *localCostumeProvider) GetByID(_ context.Context, id int) (*masterdata.Costume3d, error) {
	if id == 0 {
		return nil, fmt.Errorf("costume id is required")
	}
	if err := p.ensureCostumes(); err != nil {
		return nil, err
	}
	costume, ok := p.costumes.v().byID[id]
	if !ok {
		return nil, fmt.Errorf("costume %d not found", id)
	}
	return common.CloneCostume(costume), nil
}

func (p *localCostumeProvider) Filter(_ context.Context, filter *CostumeFilter) ([]*masterdata.Costume3d, error) {
	if filter == nil {
		filter = &CostumeFilter{}
	}
	if err := p.ensureCostumes(); err != nil {
		return nil, err
	}
	items := make([]*masterdata.Costume3d, 0)
	for _, costume := range p.costumes.v().all {
		if !localCostumeMatchesFilter(costume, filter) {
			continue
		}
		items = append(items, costume)
	}
	start := filter.Offset
	if start < 0 {
		start = 0
	}
	if start >= len(items) {
		return nil, nil
	}
	end := len(items)
	if filter.Limit > 0 && start+filter.Limit < end {
		end = start + filter.Limit
	}
	return common.CloneCostumes(items[start:end]), nil
}

func (p *localCostumeProvider) GetVariants(_ context.Context, groupID int, partType string, characterID int) ([]*masterdata.Costume3d, error) {
	if groupID == 0 {
		return nil, fmt.Errorf("costume group id is required")
	}
	if err := p.ensureCostumes(); err != nil {
		return nil, err
	}
	partType = strings.TrimSpace(partType)
	result := make([]*masterdata.Costume3d, 0)
	for _, costume := range p.costumes.v().all {
		if costume.GroupID != groupID {
			continue
		}
		if partType != "" && costume.PartType != partType {
			continue
		}
		if characterID > 0 && costume.CharacterID != characterID {
			continue
		}
		result = append(result, costume)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ColorID == result[j].ColorID {
			return result[i].ID < result[j].ID
		}
		return result[i].ColorID < result[j].ColorID
	})
	return common.CloneCostumes(result), nil
}

func (p *localCostumeProvider) GetSourceCardIDs(_ context.Context, costumeIDs []int) (map[int][]int, error) {
	if err := p.ensureCostumes(); err != nil {
		return nil, err
	}
	result := make(map[int][]int, len(costumeIDs))
	for _, costumeID := range costumeIDs {
		if costumeID <= 0 {
			continue
		}
		result[costumeID] = append([]int(nil), p.costumes.v().sourceCards[costumeID]...)
	}
	return result, nil
}

func localCostumeMatchesFilter(costume *masterdata.Costume3d, filter *CostumeFilter) bool {
	if costume == nil {
		return false
	}
	if partType := strings.TrimSpace(filter.PartType); partType != "" && costume.PartType != partType {
		return false
	}
	if costumeType := strings.TrimSpace(filter.CostumeType); costumeType != "" && costume.Costume3DType != costumeType {
		return false
	}
	if filter.CharacterID > 0 && costume.CharacterID != filter.CharacterID {
		return false
	}
	if len(filter.CharacterIDs) > 0 {
		matches := false
		for _, id := range filter.CharacterIDs {
			if id > 0 && costume.CharacterID == id {
				matches = true
				break
			}
		}
		if !matches {
			return false
		}
	}
	if filter.ColorID > 0 && costume.ColorID != filter.ColorID {
		return false
	}
	if keyword := strings.TrimSpace(filter.Keyword); keyword != "" && !localCostumeContainsKeyword(costume, keyword) {
		return false
	}
	return true
}

func localCostumeContainsKeyword(costume *masterdata.Costume3d, keyword string) bool {
	needle := strings.ToLower(keyword)
	values := []string{
		costume.Name,
		costume.ColorName,
		costume.HowToObtain,
		costume.Designer,
		costume.AssetBundleName,
	}
	for _, value := range values {
		if strings.Contains(strings.ToLower(value), needle) {
			return true
		}
	}
	return false
}

func sortCostumesForList(items []*masterdata.Costume3d) {
	sort.Slice(items, func(i, j int) bool {
		if costumeListTime(items[i]) == costumeListTime(items[j]) {
			if items[i].Seq == items[j].Seq {
				return items[i].ID > items[j].ID
			}
			return items[i].Seq > items[j].Seq
		}
		return costumeListTime(items[i]) > costumeListTime(items[j])
	})
}

func costumeListTime(costume *masterdata.Costume3d) int64 {
	if costume == nil {
		return 0
	}
	if costume.PublishedAt > 0 {
		return costume.PublishedAt
	}
	return costume.ArchivePublishedAt
}
