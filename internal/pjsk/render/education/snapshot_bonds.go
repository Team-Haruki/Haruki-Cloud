package education

import (
	"fmt"
	"sort"

	"haruki-cloud/utils/drawing"
)

func (c *Controller) BuildBondsRequestFromSnapshot(query BondsQuery) (*drawing.BondsRequest, error) {
	ctx, err := c.resolveSnapshotContext(query.Region, query.Profile, query.Snapshot)
	if err != nil {
		return nil, err
	}

	bondsMaster := ctx.source.GetBonds()
	if len(bondsMaster) == 0 {
		return nil, fmt.Errorf("bond masterdata is not available")
	}

	type bondPair struct {
		CharID1 int
		CharID2 int
	}
	groupToPair := make(map[int]bondPair, len(bondsMaster))
	for _, item := range bondsMaster {
		if item == nil || item.GroupID <= 0 {
			continue
		}
		groupToPair[item.GroupID] = bondPair{CharID1: item.CharacterID1, CharID2: item.CharacterID2}
	}

	type userBondEntry struct {
		BondsGroupID int
		Rank         int
		Exp          int
	}
	userBondByGroupID := make(map[int]userBondEntry, len(ctx.raw.UserBonds))
	for _, item := range ctx.raw.UserBonds {
		userBondByGroupID[item.BondsGroupID] = userBondEntry{
			BondsGroupID: item.BondsGroupID,
			Rank:         item.Rank,
			Exp:          item.Exp,
		}
	}

	charRankMap := make(map[int]int, len(ctx.raw.UserCharacters))
	for _, item := range ctx.raw.UserCharacters {
		charRankMap[item.CharacterID] = item.CharacterRank
	}

	levelTotalExp := make(map[int]int)
	maxLevel := 0
	for _, item := range ctx.source.GetBondLevels() {
		if item == nil || item.Level <= 0 {
			continue
		}
		levelTotalExp[item.Level] = item.TotalExp
		if item.Level > maxLevel {
			maxLevel = item.Level
		}
	}

	selectedPairs := make([]bondPair, 0, len(ctx.raw.UserBonds))
	selectedState := make([]userBondEntry, 0, len(ctx.raw.UserBonds))
	requiredCharIDs := make(map[int]struct{}, len(ctx.raw.UserBonds)*2)
	if query.Cid > 0 {
		for _, master := range bondsMaster {
			if master == nil {
				continue
			}
			pair := bondPair{CharID1: master.CharacterID1, CharID2: master.CharacterID2}
			if pair.CharID1 != query.Cid && pair.CharID2 != query.Cid {
				continue
			}
			if pair.CharID1 != query.Cid {
				pair.CharID1, pair.CharID2 = pair.CharID2, pair.CharID1
			}
			selectedPairs = append(selectedPairs, pair)
			selectedState = append(selectedState, userBondByGroupID[master.GroupID])
			requiredCharIDs[pair.CharID1] = struct{}{}
			requiredCharIDs[pair.CharID2] = struct{}{}
		}
	} else {
		for _, item := range ctx.raw.UserBonds {
			pair, ok := groupToPair[item.BondsGroupID]
			if !ok {
				continue
			}
			selectedPairs = append(selectedPairs, pair)
			selectedState = append(selectedState, userBondByGroupID[item.BondsGroupID])
			requiredCharIDs[pair.CharID1] = struct{}{}
			requiredCharIDs[pair.CharID2] = struct{}{}
		}
	}

	charStyles := make(map[int]*GameCharacterStyle, len(requiredCharIDs))
	for charID := range requiredCharIDs {
		if style := ctx.source.GetGameCharacterStyle(charID); style != nil {
			charStyles[charID] = style
		}
	}

	resolveCharIcon := func(gameID int) string {
		if style, ok := charStyles[gameID]; ok && style.CharacterID > 0 {
			return c.characterIconPath(style.CharacterID)
		}
		return c.characterIconPath(gameID)
	}
	resolveBondSortCharacterID := func(gameID int) int {
		if style, ok := charStyles[gameID]; ok && style.CharacterID > 0 {
			return style.CharacterID
		}
		return gameID
	}

	bonds := make([]drawing.BondInfo, 0, len(selectedPairs))
	userMaxLevel := 0
	for idx, pair := range selectedPairs {
		state := selectedState[idx]
		if state.Rank > userMaxLevel {
			userMaxLevel = state.Rank
		}

		info := drawing.BondInfo{
			CharaID1:       pair.CharID1,
			CharaID2:       pair.CharID2,
			CharaIconPath1: resolveCharIcon(pair.CharID1),
			CharaIconPath2: resolveCharIcon(pair.CharID2),
			CharaRank1:     charRankMap[pair.CharID1],
			CharaRank2:     charRankMap[pair.CharID2],
			BondLevel:      state.Rank,
			HasBond:        state.BondsGroupID != 0,
			Color1:         defaultBondColor(),
			Color2:         defaultBondColor(),
		}
		if style, ok := charStyles[pair.CharID1]; ok {
			info.Color1 = parseBondColorCode(style.ColorCode)
		}
		if style, ok := charStyles[pair.CharID2]; ok {
			info.Color2 = parseBondColorCode(style.ColorCode)
		}
		if state.Rank > 0 && state.Rank < maxLevel {
			currentTotalExp, okCurrent := levelTotalExp[state.Rank]
			nextTotalExp, okNext := levelTotalExp[state.Rank+1]
			if okCurrent && okNext {
				needExp := nextTotalExp - currentTotalExp - state.Exp
				if needExp < 0 {
					needExp = 0
				}
				info.NeedExp = &needExp
			}
		}
		bonds = append(bonds, info)
	}

	if query.Cid > 0 {
		deduped := make([]drawing.BondInfo, 0, len(bonds))
		indexByDisplayRight := make(map[int]int, len(bonds))
		betterBondInfo := func(current, candidate drawing.BondInfo) bool {
			if candidate.BondLevel != current.BondLevel {
				return candidate.BondLevel > current.BondLevel
			}
			if candidate.HasBond != current.HasBond {
				return candidate.HasBond
			}
			rightCurrent := resolveBondSortCharacterID(current.CharaID2)
			rightCandidate := resolveBondSortCharacterID(candidate.CharaID2)
			if rightCandidate != rightCurrent {
				return rightCandidate < rightCurrent
			}
			return candidate.CharaID2 < current.CharaID2
		}
		for _, bond := range bonds {
			displayRight := resolveBondSortCharacterID(bond.CharaID2)
			if idx, ok := indexByDisplayRight[displayRight]; ok {
				if betterBondInfo(deduped[idx], bond) {
					deduped[idx] = bond
				}
				continue
			}
			indexByDisplayRight[displayRight] = len(deduped)
			deduped = append(deduped, bond)
		}
		bonds = deduped
	}
	if maxLevel == 0 {
		maxLevel = userMaxLevel
	}

	sort.Slice(bonds, func(i, j int) bool {
		if query.Cid > 0 {
			if bonds[i].BondLevel != bonds[j].BondLevel {
				return bonds[i].BondLevel > bonds[j].BondLevel
			}
			if bonds[i].HasBond != bonds[j].HasBond {
				return bonds[i].HasBond
			}
			rightI := resolveBondSortCharacterID(bonds[i].CharaID2)
			rightJ := resolveBondSortCharacterID(bonds[j].CharaID2)
			if rightI != rightJ {
				return rightI < rightJ
			}
			if bonds[i].CharaID2 != bonds[j].CharaID2 {
				return bonds[i].CharaID2 < bonds[j].CharaID2
			}
			return bonds[i].CharaID1 < bonds[j].CharaID1
		}
		if bonds[i].BondLevel != bonds[j].BondLevel {
			return bonds[i].BondLevel > bonds[j].BondLevel
		}
		if bonds[i].CharaID1 != bonds[j].CharaID1 {
			return bonds[i].CharaID1 < bonds[j].CharaID1
		}
		return bonds[i].CharaID2 < bonds[j].CharaID2
	})

	return c.BuildBondsRequest(drawing.BondsRequest{
		Profile:  *ctx.profile,
		Bonds:    bonds,
		MaxLevel: maxLevel,
	})
}
