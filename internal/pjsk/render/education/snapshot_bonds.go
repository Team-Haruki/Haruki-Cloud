package education

import (
	"fmt"
	"sort"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/drawing"
)

type bondPair struct {
	CharID1 int
	CharID2 int
}

type userBondEntry struct {
	BondsGroupID int
	Rank         int
	Exp          int
}

type bondSnapshotBuilder struct {
	controller        *Controller
	ctx               *resolvedSnapshotContext
	query             BondsQuery
	bondsMaster       []*Bond
	groupToPair       map[int]bondPair
	userBondByGroupID map[int]userBondEntry
	charRankMap       map[int]int
	charStyles        map[int]*GameCharacterStyle
	levelTotalExp     map[int]int
	maxLevel          int
}

func (c *Controller) BuildBondsRequestFromSnapshot(query BondsQuery) (*drawing.BondsRequest, error) {
	finishBuild := commandtrace.MeasureOperation(c.traceContext(), "payload.build")
	defer finishBuild()
	ctx, err := c.resolveSnapshotContext(query.Region, query.Profile, query.Snapshot)
	if err != nil {
		return nil, err
	}

	bondsMaster := ctx.source.GetBonds()
	if len(bondsMaster) == 0 {
		return nil, fmt.Errorf("bond masterdata is not available")
	}

	builder := newBondSnapshotBuilder(c, ctx, query, bondsMaster)
	pairs, states := builder.selectPairs()
	bonds, userMaxLevel := builder.buildBonds(pairs, states)
	if query.Cid > 0 {
		bonds = builder.dedupeBonds(bonds)
	}
	if builder.maxLevel == 0 {
		builder.maxLevel = userMaxLevel
	}
	builder.sortBonds(bonds)
	if query.Cid <= 0 && len(bonds) > maxRenderedBonds {
		bonds = bonds[:maxRenderedBonds]
	}

	return c.BuildBondsRequest(drawing.BondsRequest{
		Profile:  *ctx.profile,
		Bonds:    bonds,
		MaxLevel: builder.maxLevel,
	})
}

func newBondSnapshotBuilder(
	controller *Controller,
	ctx *resolvedSnapshotContext,
	query BondsQuery,
	bondsMaster []*Bond,
) *bondSnapshotBuilder {
	builder := &bondSnapshotBuilder{
		controller:        controller,
		ctx:               ctx,
		query:             query,
		bondsMaster:       bondsMaster,
		groupToPair:       make(map[int]bondPair, len(bondsMaster)),
		userBondByGroupID: make(map[int]userBondEntry, len(ctx.raw.UserBonds)),
		charRankMap:       make(map[int]int, len(ctx.raw.UserCharacters)),
		charStyles:        make(map[int]*GameCharacterStyle),
		levelTotalExp:     make(map[int]int),
	}
	builder.indexBonds()
	builder.indexCharacters()
	builder.indexBondLevels()
	return builder
}

func (b *bondSnapshotBuilder) indexBonds() {
	for _, item := range b.bondsMaster {
		if item == nil || item.GroupID <= 0 {
			continue
		}
		b.groupToPair[item.GroupID] = bondPair{CharID1: item.CharacterID1, CharID2: item.CharacterID2}
	}
	for _, item := range b.ctx.raw.UserBonds {
		b.userBondByGroupID[item.BondsGroupID] = userBondEntry{
			BondsGroupID: item.BondsGroupID,
			Rank:         item.Rank,
			Exp:          item.Exp,
		}
	}
}

func (b *bondSnapshotBuilder) indexCharacters() {
	for _, item := range b.ctx.raw.UserCharacters {
		b.charRankMap[item.CharacterID] = item.CharacterRank
	}
}

func (b *bondSnapshotBuilder) indexBondLevels() {
	for _, item := range b.ctx.source.GetBondLevels() {
		if item == nil || item.Level <= 0 {
			continue
		}
		b.levelTotalExp[item.Level] = item.TotalExp
		b.maxLevel = max(b.maxLevel, item.Level)
	}
}

func (b *bondSnapshotBuilder) getCharacterStyle(gameID int) *GameCharacterStyle {
	if gameID <= 0 {
		return nil
	}
	if style, ok := b.charStyles[gameID]; ok {
		return style
	}
	style := b.ctx.source.GetGameCharacterStyle(gameID)
	b.charStyles[gameID] = style
	return style
}

func (b *bondSnapshotBuilder) baseCharacterID(gameID int) int {
	style := b.getCharacterStyle(gameID)
	if style != nil && style.CharacterID > 0 {
		return style.CharacterID
	}
	return gameID
}

func (b *bondSnapshotBuilder) selectPairs() ([]bondPair, []userBondEntry) {
	if b.query.Cid > 0 {
		return b.selectPairsForCharacter()
	}
	return b.selectUserBondPairs()
}

func (b *bondSnapshotBuilder) selectPairsForCharacter() ([]bondPair, []userBondEntry) {
	pairs := make([]bondPair, 0, len(b.ctx.raw.UserBonds))
	states := make([]userBondEntry, 0, len(b.ctx.raw.UserBonds))
	for _, master := range b.bondsMaster {
		if master == nil {
			continue
		}
		pair := bondPair{CharID1: master.CharacterID1, CharID2: master.CharacterID2}
		leftBaseID := b.baseCharacterID(pair.CharID1)
		rightBaseID := b.baseCharacterID(pair.CharID2)
		if leftBaseID != b.query.Cid && rightBaseID != b.query.Cid {
			continue
		}
		if leftBaseID != b.query.Cid {
			pair.CharID1, pair.CharID2 = pair.CharID2, pair.CharID1
		}
		pairs = append(pairs, pair)
		states = append(states, b.userBondByGroupID[master.GroupID])
	}
	return pairs, states
}

func (b *bondSnapshotBuilder) selectUserBondPairs() ([]bondPair, []userBondEntry) {
	pairs := make([]bondPair, 0, len(b.ctx.raw.UserBonds))
	states := make([]userBondEntry, 0, len(b.ctx.raw.UserBonds))
	for _, item := range b.ctx.raw.UserBonds {
		pair, ok := b.groupToPair[item.BondsGroupID]
		if !ok {
			continue
		}
		pairs = append(pairs, pair)
		states = append(states, b.userBondByGroupID[item.BondsGroupID])
	}
	return pairs, states
}

func (b *bondSnapshotBuilder) buildBonds(
	pairs []bondPair,
	states []userBondEntry,
) ([]drawing.BondInfo, int) {
	bonds := make([]drawing.BondInfo, 0, len(pairs))
	userMaxLevel := 0
	for idx, pair := range pairs {
		state := states[idx]
		userMaxLevel = max(userMaxLevel, state.Rank)
		bonds = append(bonds, b.buildBondInfo(pair, state))
	}
	return bonds, userMaxLevel
}

func (b *bondSnapshotBuilder) buildBondInfo(pair bondPair, state userBondEntry) drawing.BondInfo {
	info := drawing.BondInfo{
		CharaID1:       pair.CharID1,
		CharaID2:       pair.CharID2,
		CharaIconPath1: b.characterIcon(pair.CharID1),
		CharaIconPath2: b.characterIcon(pair.CharID2),
		CharaRank1:     b.charRankMap[b.baseCharacterID(pair.CharID1)],
		CharaRank2:     b.charRankMap[b.baseCharacterID(pair.CharID2)],
		BondLevel:      state.Rank,
		HasBond:        state.BondsGroupID != 0,
		Color1:         defaultBondColor(),
		Color2:         defaultBondColor(),
	}
	if style := b.getCharacterStyle(pair.CharID1); style != nil {
		info.Color1 = parseBondColorCode(style.ColorCode)
	}
	if style := b.getCharacterStyle(pair.CharID2); style != nil {
		info.Color2 = parseBondColorCode(style.ColorCode)
	}
	info.NeedExp = b.needExp(state)
	return info
}

func (b *bondSnapshotBuilder) characterIcon(gameID int) string {
	return b.controller.characterIconPath(b.baseCharacterID(gameID))
}

func (b *bondSnapshotBuilder) needExp(state userBondEntry) *int {
	if state.Rank <= 0 || state.Rank >= b.maxLevel {
		return nil
	}
	currentTotalExp, okCurrent := b.levelTotalExp[state.Rank]
	nextTotalExp, okNext := b.levelTotalExp[state.Rank+1]
	if !okCurrent || !okNext {
		return nil
	}
	return new(max(0, nextTotalExp-currentTotalExp-state.Exp))
}

func (b *bondSnapshotBuilder) dedupeBonds(bonds []drawing.BondInfo) []drawing.BondInfo {
	deduped := make([]drawing.BondInfo, 0, len(bonds))
	indexByDisplayRight := make(map[int]int, len(bonds))
	for _, bond := range bonds {
		displayRight := b.baseCharacterID(bond.CharaID2)
		idx, exists := indexByDisplayRight[displayRight]
		if !exists {
			indexByDisplayRight[displayRight] = len(deduped)
			deduped = append(deduped, bond)
			continue
		}
		if b.betterBondInfo(deduped[idx], bond) {
			deduped[idx] = bond
		}
	}
	return deduped
}

func (b *bondSnapshotBuilder) betterBondInfo(current, candidate drawing.BondInfo) bool {
	if candidate.BondLevel != current.BondLevel {
		return candidate.BondLevel > current.BondLevel
	}
	if candidate.HasBond != current.HasBond {
		return candidate.HasBond
	}
	rightCurrent := b.baseCharacterID(current.CharaID2)
	rightCandidate := b.baseCharacterID(candidate.CharaID2)
	if rightCandidate != rightCurrent {
		return rightCandidate < rightCurrent
	}
	return candidate.CharaID2 < current.CharaID2
}

func (b *bondSnapshotBuilder) sortBonds(bonds []drawing.BondInfo) {
	sort.Slice(bonds, func(i, j int) bool {
		return b.lessBond(bonds[i], bonds[j])
	})
}

func (b *bondSnapshotBuilder) lessBond(left, right drawing.BondInfo) bool {
	if left.BondLevel != right.BondLevel {
		return left.BondLevel > right.BondLevel
	}
	if b.query.Cid > 0 {
		return b.lessCharacterScopedBond(left, right)
	}
	if left.CharaID1 != right.CharaID1 {
		return left.CharaID1 < right.CharaID1
	}
	return left.CharaID2 < right.CharaID2
}

func (b *bondSnapshotBuilder) lessCharacterScopedBond(left, right drawing.BondInfo) bool {
	if left.HasBond != right.HasBond {
		return left.HasBond
	}
	rightLeft := b.baseCharacterID(left.CharaID2)
	rightRight := b.baseCharacterID(right.CharaID2)
	if rightLeft != rightRight {
		return rightLeft < rightRight
	}
	if left.CharaID2 != right.CharaID2 {
		return left.CharaID2 < right.CharaID2
	}
	return left.CharaID1 < right.CharaID1
}
