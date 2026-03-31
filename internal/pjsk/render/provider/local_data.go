package provider

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/common"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

// ===========================================================================
// localCardProvider
// ===========================================================================

type localCardProvider struct {
	store      *localStore
	characters *localCharacterProvider
	skills     *localSkillProvider

	cardsOnce sync.Once
	cardAll   []*masterdata.Card
	cardByID  map[int]*masterdata.Card
	cardsErr  error

	supplyOnce sync.Once
	supplyByID map[int]string
	supplyErr  error

	gachaOnce sync.Once
	gachas    []*masterdata.Gacha
	gachaErr  error

	costumeOnce   sync.Once
	costumeByCard map[int][]*masterdata.Costume3d
	costumeErr    error

	eventCardOnce sync.Once
	cardsByEvent  map[int][]int
	eventByCard   map[int]int
	eventCardErr  error
}

func (p *localCardProvider) ensureCards() error {
	p.cardsOnce.Do(func() {
		items, err := loadJSON[masterdata.Card](p.store, "cards.json")
		if err != nil {
			p.cardsErr = err
			return
		}
		p.cardByID = make(map[int]*masterdata.Card, len(items))
		p.cardAll = make([]*masterdata.Card, 0, len(items))
		for i := range items {
			c := &items[i]
			p.cardByID[c.ID] = c
			p.cardAll = append(p.cardAll, c)
		}
		sort.Slice(p.cardAll, func(i, j int) bool {
			return p.cardAll[i].ReleaseAt < p.cardAll[j].ReleaseAt
		})
	})
	return p.cardsErr
}

func (p *localCardProvider) ensureSupplies() error {
	p.supplyOnce.Do(func() {
		items, err := loadJSON[localCardSupplyJSON](p.store, "cardSupplies.json")
		if err != nil {
			p.supplyErr = err
			return
		}
		p.supplyByID = make(map[int]string, len(items))
		for _, item := range items {
			p.supplyByID[item.ID] = cardNormalizeSupplyType(item.CardSupplyType)
		}
	})
	return p.supplyErr
}

func (p *localCardProvider) ensureGachas() error {
	p.gachaOnce.Do(func() {
		items, err := loadJSON[masterdata.Gacha](p.store, "gachas.json")
		if err != nil {
			p.gachaErr = err
			return
		}
		p.gachas = make([]*masterdata.Gacha, 0, len(items))
		for i := range items {
			p.gachas = append(p.gachas, &items[i])
		}
		sort.Slice(p.gachas, func(i, j int) bool {
			if p.gachas[i].StartAt == p.gachas[j].StartAt {
				return p.gachas[i].ID > p.gachas[j].ID
			}
			return p.gachas[i].StartAt > p.gachas[j].StartAt
		})
	})
	return p.gachaErr
}

func (p *localCardProvider) ensureCostumes() error {
	p.costumeOnce.Do(func() {
		links, err := loadJSON[localCardCostume3dJSON](p.store, "cardCostume3ds.json")
		if err != nil {
			p.costumeErr = err
			return
		}
		raw, err := loadJSON[localCostume3dJSON](p.store, "costume3ds.json")
		if err != nil {
			p.costumeErr = err
			return
		}
		costumeByID := make(map[int]*masterdata.Costume3d, len(raw))
		for i := range raw {
			costumeByID[raw[i].ID] = raw[i].toModel()
		}
		p.costumeByCard = make(map[int][]*masterdata.Costume3d)
		for _, link := range links {
			if c, ok := costumeByID[link.Costume3dID]; ok {
				p.costumeByCard[link.CardID] = append(p.costumeByCard[link.CardID], c)
			}
		}
	})
	return p.costumeErr
}

func (p *localCardProvider) ensureEventCards() error {
	p.eventCardOnce.Do(func() {
		items, err := loadJSON[localEventCardJSON](p.store, "eventCards.json")
		if err != nil {
			p.eventCardErr = err
			return
		}
		p.cardsByEvent = make(map[int][]int)
		p.eventByCard = make(map[int]int)
		for _, item := range items {
			p.cardsByEvent[item.EventID] = append(p.cardsByEvent[item.EventID], item.CardID)
			if _, ok := p.eventByCard[item.CardID]; !ok {
				p.eventByCard[item.CardID] = item.EventID
			}
		}
	})
	return p.eventCardErr
}

func (p *localCardProvider) GetByID(id int) (*masterdata.Card, error) {
	if id == 0 {
		return nil, fmt.Errorf("invalid card id")
	}
	if err := p.ensureCards(); err != nil {
		return nil, err
	}
	card, ok := p.cardByID[id]
	if !ok {
		return nil, fmt.Errorf("card %d not found", id)
	}
	return common.CloneCard(card), nil
}

func (p *localCardProvider) GetByCharacterAndSeq(characterID, seq int) (*masterdata.Card, error) {
	if characterID == 0 {
		return nil, fmt.Errorf("character id is required")
	}
	if err := p.ensureCards(); err != nil {
		return nil, err
	}

	var cards []*masterdata.Card
	for _, c := range p.cardAll {
		if c.CharacterID == characterID {
			cards = append(cards, c)
		}
	}
	if len(cards) == 0 {
		return nil, fmt.Errorf("no cards found for character %d", characterID)
	}

	var card *masterdata.Card
	if seq < 0 {
		index := len(cards) + seq
		if index < 0 || index >= len(cards) {
			return nil, fmt.Errorf("card sequence out of range: %d (total: %d)", seq, len(cards))
		}
		card = cards[index]
	} else {
		if seq < 1 || seq > len(cards) {
			return nil, fmt.Errorf("card sequence out of range: %d (total: %d)", seq, len(cards))
		}
		card = cards[seq-1]
	}
	return common.CloneCard(card), nil
}

func (p *localCardProvider) Filter(filter *CardFilter) ([]*masterdata.Card, error) {
	if filter == nil {
		return nil, fmt.Errorf("filter is required")
	}
	if err := p.ensureCards(); err != nil {
		return nil, err
	}

	var allowedIDs map[int]struct{}
	if filter.EventID != 0 {
		if err := p.ensureEventCards(); err != nil {
			return nil, err
		}
		cardIDs, ok := p.cardsByEvent[filter.EventID]
		if !ok || len(cardIDs) == 0 {
			return nil, nil
		}
		allowedIDs = make(map[int]struct{}, len(cardIDs))
		for _, id := range cardIDs {
			allowedIDs[id] = struct{}{}
		}
	}

	results := make([]*masterdata.Card, 0)
	for _, card := range p.cardAll {
		if allowedIDs != nil {
			if _, ok := allowedIDs[card.ID]; !ok {
				continue
			}
		}
		if filter.CharacterID != 0 && card.CharacterID != filter.CharacterID {
			continue
		}
		if filter.Rarity != "" && card.CardRarityType != filter.Rarity {
			continue
		}
		if filter.Attr != "" && card.Attr != filter.Attr {
			continue
		}
		if filter.Year != 0 {
			start := time.Date(filter.Year, time.January, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
			end := time.Date(filter.Year+1, time.January, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
			if card.ReleaseAt < start || card.ReleaseAt >= end {
				continue
			}
		}
		if filter.Unit != "" || filter.SupportUnit != "" {
			if !p.matchesUnitFilter(filter, card) {
				continue
			}
		}
		if filter.SkillType != "" {
			if p.skills != nil {
				skill, sErr := p.skills.GetByID(card.SkillID)
				if sErr != nil || skill == nil || skill.DescriptionSpriteName != filter.SkillType {
					continue
				}
			} else {
				continue
			}
		}
		if filter.SupplyType != "" && !cardMatchesSupplyFilter(filter.SupplyType, p.GetSupplyType(card)) {
			continue
		}
		results = append(results, common.CloneCard(card))
		if filter.Limit > 0 && len(results) >= filter.Limit {
			break
		}
	}
	return results, nil
}

func (p *localCardProvider) matchesUnitFilter(filter *CardFilter, card *masterdata.Card) bool {
	if filter == nil || card == nil {
		return false
	}
	if filter.Unit == "" && filter.SupportUnit == "" {
		return true
	}
	if p.characters == nil {
		return false
	}
	character, err := p.characters.GetByID(card.CharacterID)
	if err != nil || character == nil {
		return false
	}
	mainUnit := cardNormalizeUnit(character.Unit)
	supportUnit := cardNormalizeSupportUnit(card.SupportUnit)

	if filter.Unit != "" && filter.Unit != mainUnit && filter.Unit != supportUnit {
		return false
	}
	if filter.SupportUnit != "" && filter.SupportUnit != supportUnit {
		return false
	}
	return true
}

func (p *localCardProvider) GetSupplyType(cardInfo *masterdata.Card) string {
	if cardInfo == nil {
		return cardNormalizeSupplyType("")
	}
	if cardInfo.CardRarityType == "rarity_birthday" {
		return cardNormalizeSupplyType("birthday")
	}
	if cardInfo.CardSupplyID == 0 {
		return cardNormalizeSupplyType("")
	}
	if err := p.ensureSupplies(); err != nil {
		return cardNormalizeSupplyType("")
	}
	if v, ok := p.supplyByID[cardInfo.CardSupplyID]; ok {
		return v
	}
	return cardNormalizeSupplyType("")
}

func (p *localCardProvider) GetGachaByCardID(cardID int) (*masterdata.Gacha, error) {
	if cardID == 0 {
		return nil, fmt.Errorf("invalid card id")
	}
	if err := p.ensureGachas(); err != nil {
		return nil, err
	}
	for _, g := range p.gachas {
		if cardContainsPickup(g, cardID) {
			return common.CloneGacha(g), nil
		}
	}
	return nil, fmt.Errorf("gacha not found for card: %d", cardID)
}

func (p *localCardProvider) GetCostume3dsByCardID(cardID int) ([]*masterdata.Costume3d, error) {
	if cardID == 0 {
		return nil, nil
	}
	if err := p.ensureCostumes(); err != nil {
		return nil, err
	}
	costumes, ok := p.costumeByCard[cardID]
	if !ok || len(costumes) == 0 {
		return nil, nil
	}
	return common.CloneCostumes(costumes), nil
}

func (p *localCardProvider) GetUnitByCardID(cardID int) (string, error) {
	card, err := p.GetByID(cardID)
	if err != nil {
		return "", err
	}
	if p.characters != nil {
		character, cErr := p.characters.GetByID(card.CharacterID)
		if cErr == nil && character != nil {
			if character.Unit != "" && character.Unit != "piapro" {
				return character.Unit, nil
			}
			if card.SupportUnit != "" && card.SupportUnit != "none" {
				return card.SupportUnit, nil
			}
			return "piapro", nil
		}
	}
	return "", fmt.Errorf("character not found for card %d", cardID)
}

// ===========================================================================
// localCharacterProvider
// ===========================================================================

type localCharacterProvider struct {
	store *localStore

	charOnce  sync.Once
	charByID  map[int]*masterdata.Character
	charErr   error

	unitOnce  sync.Once
	unitByID  map[int]*masterdata.GameCharacterUnit
	colorByID map[int]string
	unitErr   error
}

func (p *localCharacterProvider) ensureCharacters() error {
	p.charOnce.Do(func() {
		items, err := loadJSON[localGameCharacterJSON](p.store, "gameCharacters.json")
		if err != nil {
			p.charErr = err
			return
		}
		p.charByID = make(map[int]*masterdata.Character, len(items))
		for _, item := range items {
			p.charByID[item.ID] = &masterdata.Character{
				ID:        item.ID,
				FirstName: item.FirstName,
				GivenName: item.GivenName,
				Unit:      item.Unit,
			}
		}
	})
	return p.charErr
}

func (p *localCharacterProvider) ensureUnits() error {
	p.unitOnce.Do(func() {
		items, err := loadJSON[masterdata.GameCharacterUnit](p.store, "gameCharacterUnits.json")
		if err != nil {
			p.unitErr = err
			return
		}
		p.unitByID = make(map[int]*masterdata.GameCharacterUnit, len(items))
		p.colorByID = make(map[int]string, len(items))
		for i := range items {
			p.unitByID[items[i].ID] = &items[i]
			p.colorByID[items[i].ID] = strings.TrimSpace(items[i].ColorCode)
		}
	})
	return p.unitErr
}

func (p *localCharacterProvider) GetByID(id int) (*masterdata.Character, error) {
	if id == 0 {
		return nil, fmt.Errorf("character id is required")
	}
	if err := p.ensureCharacters(); err != nil {
		return nil, err
	}
	ch, ok := p.charByID[id]
	if !ok {
		return nil, fmt.Errorf("character %d not found", id)
	}
	return common.CloneCharacter(ch), nil
}

func (p *localCharacterProvider) GetColorCode(id int) (string, bool) {
	if id == 0 {
		return "", false
	}
	if err := p.ensureUnits(); err != nil {
		return "", false
	}
	v, ok := p.colorByID[id]
	return v, ok && v != ""
}

func (p *localCharacterProvider) GetGameCharacterUnit(id int) (*masterdata.GameCharacterUnit, error) {
	if id == 0 {
		return nil, fmt.Errorf("game character unit id is required")
	}
	if err := p.ensureUnits(); err != nil {
		return nil, err
	}
	u, ok := p.unitByID[id]
	if !ok {
		return nil, fmt.Errorf("game character unit %d not found", id)
	}
	return common.CloneGameCharacterUnit(u), nil
}

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

// ===========================================================================
// localEventProvider
// ===========================================================================

type localEventProvider struct {
	store *localStore
	cards *localCardProvider

	eventsOnce sync.Once
	eventAll   []*masterdata.Event
	eventByID  map[int]*masterdata.Event
	eventsErr  error

	eventCardOnce sync.Once
	eventByCard   map[int]int
	cardsByEvent  map[int][]int
	eventCardErr  error

	deckBonusOnce    sync.Once
	deckBonusByEvent map[int][]*masterdata.EventDeckBonus
	deckBonusErr     error

	worldBloomOnce    sync.Once
	worldBloomByEvent map[int][]*masterdata.WorldBloom
	worldBloomErr     error
}

func (p *localEventProvider) ensureEvents() error {
	p.eventsOnce.Do(func() {
		items, err := loadJSON[localEventJSON](p.store, "events.json")
		if err != nil {
			p.eventsErr = err
			return
		}
		p.eventByID = make(map[int]*masterdata.Event, len(items))
		p.eventAll = make([]*masterdata.Event, 0, len(items))
		for i := range items {
			m := items[i].toModel()
			p.eventByID[m.ID] = m
			p.eventAll = append(p.eventAll, m)
		}
		sort.Slice(p.eventAll, func(i, j int) bool {
			return p.eventAll[i].StartAt < p.eventAll[j].StartAt
		})
	})
	return p.eventsErr
}

func (p *localEventProvider) ensureEventCards() error {
	p.eventCardOnce.Do(func() {
		items, err := loadJSON[localEventCardJSON](p.store, "eventCards.json")
		if err != nil {
			p.eventCardErr = err
			return
		}
		p.eventByCard = make(map[int]int)
		p.cardsByEvent = make(map[int][]int)
		for _, item := range items {
			p.cardsByEvent[item.EventID] = append(p.cardsByEvent[item.EventID], item.CardID)
			if _, ok := p.eventByCard[item.CardID]; !ok {
				p.eventByCard[item.CardID] = item.EventID
			}
		}
	})
	return p.eventCardErr
}

func (p *localEventProvider) ensureDeckBonuses() error {
	p.deckBonusOnce.Do(func() {
		items, err := loadJSON[masterdata.EventDeckBonus](p.store, "eventDeckBonuses.json")
		if err != nil {
			p.deckBonusErr = err
			return
		}
		p.deckBonusByEvent = make(map[int][]*masterdata.EventDeckBonus)
		for i := range items {
			p.deckBonusByEvent[items[i].EventID] = append(
				p.deckBonusByEvent[items[i].EventID], &items[i])
		}
	})
	return p.deckBonusErr
}

func (p *localEventProvider) ensureWorldBlooms() error {
	p.worldBloomOnce.Do(func() {
		items, err := loadJSON[localWorldBloomJSON](p.store, "worldBlooms.json")
		if err != nil {
			p.worldBloomErr = err
			return
		}
		p.worldBloomByEvent = make(map[int][]*masterdata.WorldBloom)
		for i := range items {
			m := items[i].toModel()
			p.worldBloomByEvent[m.EventID] = append(p.worldBloomByEvent[m.EventID], m)
		}
		for _, wbs := range p.worldBloomByEvent {
			sort.Slice(wbs, func(i, j int) bool {
				return wbs[i].ChapterStartAt < wbs[j].ChapterStartAt
			})
		}
	})
	return p.worldBloomErr
}

func (p *localEventProvider) GetByID(id int) (*masterdata.Event, error) {
	if id == 0 {
		return nil, fmt.Errorf("event id is required")
	}
	if err := p.ensureEvents(); err != nil {
		return nil, err
	}
	ev, ok := p.eventByID[id]
	if !ok {
		return nil, fmt.Errorf("event %d not found", id)
	}
	return common.CloneEvent(ev), nil
}

func (p *localEventProvider) GetByCardID(cardID int) (*masterdata.Event, error) {
	if err := p.ensureEventCards(); err != nil {
		return nil, err
	}
	eventID, ok := p.eventByCard[cardID]
	if !ok {
		return nil, fmt.Errorf("no event found for card %d", cardID)
	}
	return p.GetByID(eventID)
}

func (p *localEventProvider) GetAll() []*masterdata.Event {
	if err := p.ensureEvents(); err != nil {
		return nil
	}
	result := make([]*masterdata.Event, 0, len(p.eventAll))
	for _, ev := range p.eventAll {
		result = append(result, common.CloneEvent(ev))
	}
	return result
}

func (p *localEventProvider) GetCards(eventID int) ([]*masterdata.Card, error) {
	if err := p.ensureEventCards(); err != nil {
		return nil, err
	}
	cardIDs, ok := p.cardsByEvent[eventID]
	if !ok || len(cardIDs) == 0 {
		return nil, fmt.Errorf("no cards found for event %d", eventID)
	}
	result := make([]*masterdata.Card, 0, len(cardIDs))
	for _, id := range cardIDs {
		card, err := p.cards.GetByID(id)
		if err != nil {
			return nil, err
		}
		result = append(result, card)
	}
	return result, nil
}

func (p *localEventProvider) GetBannerCharacterID(eventID int) (int, error) {
	cards, err := p.GetCards(eventID)
	if err != nil {
		return 0, err
	}
	minCardID := -1
	var selected *masterdata.Card
	for _, cardInfo := range cards {
		supplyType := p.cards.GetSupplyType(cardInfo)
		if strings.Contains(supplyType, "festival") {
			continue
		}
		if minCardID == -1 || cardInfo.ID < minCardID {
			minCardID = cardInfo.ID
			selected = cardInfo
		}
	}
	if selected == nil {
		return 0, fmt.Errorf("no valid banner card found for event %d", eventID)
	}
	return selected.CharacterID, nil
}

func (p *localEventProvider) GetDeckBonuses(eventID int) ([]*masterdata.EventDeckBonus, error) {
	if err := p.ensureDeckBonuses(); err != nil {
		return nil, err
	}
	bonuses, ok := p.deckBonusByEvent[eventID]
	if !ok {
		return nil, nil
	}
	result := make([]*masterdata.EventDeckBonus, 0, len(bonuses))
	for _, b := range bonuses {
		c := *b
		result = append(result, &c)
	}
	return result, nil
}

func (p *localEventProvider) GetBanEvents(charID int) []*masterdata.Event {
	if err := p.ensureEvents(); err != nil {
		return nil
	}
	result := make([]*masterdata.Event, 0)
	for _, ev := range p.eventAll {
		if ev.EventType != "marathon" && ev.EventType != "cheerful_carnival" {
			continue
		}
		bannerCID, err := p.GetBannerCharacterID(ev.ID)
		if err != nil || bannerCID != charID {
			continue
		}
		result = append(result, common.CloneEvent(ev))
	}
	return result
}

func (p *localEventProvider) GetWorldBloomChapters(eventID int) []*masterdata.WorldBloom {
	if err := p.ensureWorldBlooms(); err != nil {
		return nil
	}
	wbs, ok := p.worldBloomByEvent[eventID]
	if !ok {
		return nil
	}
	result := make([]*masterdata.WorldBloom, 0, len(wbs))
	for _, wb := range wbs {
		c := *wb
		result = append(result, &c)
	}
	return result
}

// ===========================================================================
// localMusicProvider
// ===========================================================================

type localMusicProvider struct {
	store  *localStore
	events EventProvider

	musicOnce sync.Once
	musicAll  []*masterdata.Music
	musicByID map[int]*masterdata.Music
	musicErr  error

	diffOnce    sync.Once
	diffByMusic map[int][]*masterdata.MusicDifficulty
	diffErr     error

	vocalOnce    sync.Once
	vocalByMusic map[int][]*masterdata.MusicVocal
	vocalErr     error

	tagOnce    sync.Once
	tagByMusic map[int][]string
	tagErr     error

	outsideOnce  sync.Once
	outsideByID  map[int]string
	outsideErr   error

	eventMusicOnce    sync.Once
	musicIDByEvent    map[int]int
	eventIDsByMusic   map[int][]int
	eventMusicErr     error

	limitedOnce    sync.Once
	limitedByMusic map[int][]*masterdata.LimitedTimeMusic
	limitedErr     error
}

func (p *localMusicProvider) ensureMusics() error {
	p.musicOnce.Do(func() {
		items, err := loadJSON[localMusicJSON](p.store, "musics.json")
		if err != nil {
			p.musicErr = err
			return
		}
		p.musicByID = make(map[int]*masterdata.Music, len(items))
		p.musicAll = make([]*masterdata.Music, 0, len(items))
		for i := range items {
			m := items[i].toModel()
			p.musicByID[m.ID] = m
			p.musicAll = append(p.musicAll, m)
		}
		sort.Slice(p.musicAll, func(i, j int) bool {
			if p.musicAll[i].PublishedAt == p.musicAll[j].PublishedAt {
				return p.musicAll[i].ID < p.musicAll[j].ID
			}
			return p.musicAll[i].PublishedAt < p.musicAll[j].PublishedAt
		})
	})
	return p.musicErr
}

func (p *localMusicProvider) ensureDifficulties() error {
	p.diffOnce.Do(func() {
		items, err := loadJSON[masterdata.MusicDifficulty](p.store, "musicDifficulties.json")
		if err != nil {
			p.diffErr = err
			return
		}
		p.diffByMusic = make(map[int][]*masterdata.MusicDifficulty)
		for i := range items {
			p.diffByMusic[items[i].MusicID] = append(p.diffByMusic[items[i].MusicID], &items[i])
		}
	})
	return p.diffErr
}

func (p *localMusicProvider) ensureVocals() error {
	p.vocalOnce.Do(func() {
		items, err := loadJSON[localMusicVocalJSON](p.store, "musicVocals.json")
		if err != nil {
			p.vocalErr = err
			return
		}
		p.vocalByMusic = make(map[int][]*masterdata.MusicVocal)
		for i := range items {
			m := items[i].toModel()
			p.vocalByMusic[m.MusicID] = append(p.vocalByMusic[m.MusicID], m)
		}
	})
	return p.vocalErr
}

func (p *localMusicProvider) ensureTags() error {
	p.tagOnce.Do(func() {
		items, err := loadJSON[localMusicTagJSON](p.store, "musicTags.json")
		if err != nil {
			p.tagErr = err
			return
		}
		p.tagByMusic = make(map[int][]string)
		for _, item := range items {
			tag := strings.TrimSpace(item.MusicTag)
			if tag != "" {
				p.tagByMusic[item.MusicID] = append(p.tagByMusic[item.MusicID], tag)
			}
		}
	})
	return p.tagErr
}

func (p *localMusicProvider) ensureOutsideCharacters() error {
	p.outsideOnce.Do(func() {
		items, err := loadJSON[localOutsideCharacterJSON](p.store, "outsideCharacters.json")
		if err != nil {
			p.outsideErr = err
			return
		}
		p.outsideByID = make(map[int]string, len(items))
		for _, item := range items {
			p.outsideByID[item.ID] = strings.TrimSpace(item.Name)
		}
	})
	return p.outsideErr
}

func (p *localMusicProvider) ensureEventMusics() error {
	p.eventMusicOnce.Do(func() {
		items, err := loadJSON[localEventMusicJSON](p.store, "eventMusics.json")
		if err != nil {
			p.eventMusicErr = err
			return
		}
		p.musicIDByEvent = make(map[int]int)
		p.eventIDsByMusic = make(map[int][]int)
		sort.Slice(items, func(i, j int) bool {
			return items[i].Seq < items[j].Seq
		})
		for _, item := range items {
			if _, ok := p.musicIDByEvent[item.EventID]; !ok {
				p.musicIDByEvent[item.EventID] = item.MusicID
			}
			p.eventIDsByMusic[item.MusicID] = append(p.eventIDsByMusic[item.MusicID], item.EventID)
		}
	})
	return p.eventMusicErr
}

func (p *localMusicProvider) ensureLimitedTimeMusics() error {
	p.limitedOnce.Do(func() {
		items, err := loadJSON[masterdata.LimitedTimeMusic](p.store, "limitedTimeMusics.json")
		if err != nil {
			p.limitedErr = err
			return
		}
		p.limitedByMusic = make(map[int][]*masterdata.LimitedTimeMusic)
		for i := range items {
			p.limitedByMusic[items[i].MusicID] = append(p.limitedByMusic[items[i].MusicID], &items[i])
		}
	})
	return p.limitedErr
}

func (p *localMusicProvider) Search(query string) (*masterdata.Music, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("music not found: empty query")
	}
	if id, err := strconv.Atoi(query); err == nil {
		return p.GetByID(id)
	}
	all := p.GetAll()
	lowerQuery := strings.ToLower(query)
	for _, m := range all {
		if strings.Contains(strings.ToLower(m.Title), lowerQuery) {
			return common.CloneMusic(m), nil
		}
		if strings.Contains(strings.ToLower(m.Pronunciation), lowerQuery) {
			return common.CloneMusic(m), nil
		}
	}
	return nil, fmt.Errorf("music not found: %s", query)
}

func (p *localMusicProvider) GetByID(id int) (*masterdata.Music, error) {
	if id <= 0 {
		return nil, fmt.Errorf("invalid music id: %d", id)
	}
	if err := p.ensureMusics(); err != nil {
		return nil, err
	}
	m, ok := p.musicByID[id]
	if !ok {
		return nil, fmt.Errorf("music %d not found", id)
	}
	return common.CloneMusic(m), nil
}

func (p *localMusicProvider) GetByEventID(eventID int) (*masterdata.Music, error) {
	if err := p.ensureEventMusics(); err != nil {
		return nil, err
	}
	musicID, ok := p.musicIDByEvent[eventID]
	if !ok {
		return nil, fmt.Errorf("no music found for event %d", eventID)
	}
	return p.GetByID(musicID)
}

func (p *localMusicProvider) GetAll() []*masterdata.Music {
	if err := p.ensureMusics(); err != nil {
		return nil
	}
	return common.CloneMusicList(p.musicAll)
}

func (p *localMusicProvider) GetLocalizedTitles(musicID int) ([]string, error) {
	if musicID <= 0 {
		return nil, fmt.Errorf("invalid music id: %d", musicID)
	}
	if err := p.ensureMusics(); err != nil {
		return nil, err
	}
	m, ok := p.musicByID[musicID]
	if !ok {
		return nil, fmt.Errorf("music %d not found", musicID)
	}
	unique := make(map[string]struct{}, 2)
	titles := make([]string, 0, 2)
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		key := strings.ToLower(s)
		if _, ok := unique[key]; ok {
			return
		}
		unique[key] = struct{}{}
		titles = append(titles, s)
	}
	add(m.Title)
	add(m.Pronunciation)
	return titles, nil
}

func (p *localMusicProvider) GetDifficulties(musicID int) ([]*masterdata.MusicDifficulty, error) {
	if err := p.ensureDifficulties(); err != nil {
		return nil, err
	}
	diffs, ok := p.diffByMusic[musicID]
	if !ok || len(diffs) == 0 {
		return nil, fmt.Errorf("no difficulties found for music %d", musicID)
	}
	result := make([]*masterdata.MusicDifficulty, 0, len(diffs))
	for _, d := range diffs {
		c := *d
		result = append(result, &c)
	}
	return result, nil
}

func (p *localMusicProvider) GetVocals(musicID int) ([]*masterdata.MusicVocal, error) {
	if err := p.ensureVocals(); err != nil {
		return nil, err
	}
	vocals, ok := p.vocalByMusic[musicID]
	if !ok || len(vocals) == 0 {
		return nil, fmt.Errorf("no vocals found for music %d", musicID)
	}
	result := make([]*masterdata.MusicVocal, 0, len(vocals))
	for _, v := range vocals {
		c := *v
		if v.Characters != nil {
			c.Characters = append([]masterdata.MusicVocalCharacter(nil), v.Characters...)
		}
		result = append(result, &c)
	}
	return result, nil
}

func (p *localMusicProvider) GetTags(musicID int) ([]string, error) {
	if err := p.ensureTags(); err != nil {
		return nil, err
	}
	tags := p.tagByMusic[musicID]
	return append([]string(nil), tags...), nil
}

func (p *localMusicProvider) GetOutsideCharacterByID(id int) (string, error) {
	if id <= 0 {
		return "", fmt.Errorf("invalid outside character id: %d", id)
	}
	if err := p.ensureOutsideCharacters(); err != nil {
		return "", err
	}
	name, ok := p.outsideByID[id]
	if !ok {
		return "", fmt.Errorf("outside character %d not found", id)
	}
	return name, nil
}

func (p *localMusicProvider) GetPrimaryEventByMusicID(musicID int) (*masterdata.Event, error) {
	if err := p.ensureEventMusics(); err != nil {
		return nil, err
	}
	eventIDs, ok := p.eventIDsByMusic[musicID]
	if !ok || len(eventIDs) == 0 {
		return nil, fmt.Errorf("no events found for music %d", musicID)
	}
	if p.events == nil {
		return nil, fmt.Errorf("event provider not configured")
	}
	var earliest *masterdata.Event
	for _, eid := range eventIDs {
		ev, err := p.events.GetByID(eid)
		if err != nil {
			continue
		}
		if earliest == nil || ev.StartAt < earliest.StartAt {
			earliest = ev
		}
	}
	if earliest == nil {
		return nil, fmt.Errorf("no events found for music %d", musicID)
	}
	return earliest, nil
}

func (p *localMusicProvider) GetLimitedTimeMusics(musicID int) []*masterdata.LimitedTimeMusic {
	if err := p.ensureLimitedTimeMusics(); err != nil {
		return nil
	}
	items, ok := p.limitedByMusic[musicID]
	if !ok {
		return nil
	}
	result := make([]*masterdata.LimitedTimeMusic, 0, len(items))
	for _, item := range items {
		c := *item
		result = append(result, &c)
	}
	return result
}

// ===========================================================================
// localGachaProvider
// ===========================================================================

type localGachaProvider struct {
	store *localStore

	gachaOnce sync.Once
	gachaAll  []*masterdata.Gacha
	gachaByID map[int]*masterdata.Gacha
	gachaErr  error

	cardOnce  sync.Once
	cardByID  map[int]*masterdata.Card
	cardErr   error
}

func (p *localGachaProvider) ensureGachas() error {
	p.gachaOnce.Do(func() {
		items, err := loadJSON[masterdata.Gacha](p.store, "gachas.json")
		if err != nil {
			p.gachaErr = err
			return
		}
		p.gachaByID = make(map[int]*masterdata.Gacha, len(items))
		p.gachaAll = make([]*masterdata.Gacha, 0, len(items))
		for i := range items {
			g := &items[i]
			p.gachaByID[g.ID] = g
			p.gachaAll = append(p.gachaAll, g)
		}
		sort.Slice(p.gachaAll, func(i, j int) bool {
			if p.gachaAll[i].StartAt == p.gachaAll[j].StartAt {
				return p.gachaAll[i].ID > p.gachaAll[j].ID
			}
			return p.gachaAll[i].StartAt > p.gachaAll[j].StartAt
		})
	})
	return p.gachaErr
}

func (p *localGachaProvider) ensureCards() error {
	p.cardOnce.Do(func() {
		items, err := loadJSON[masterdata.Card](p.store, "cards.json")
		if err != nil {
			p.cardErr = err
			return
		}
		p.cardByID = make(map[int]*masterdata.Card, len(items))
		for i := range items {
			p.cardByID[items[i].ID] = &items[i]
		}
	})
	return p.cardErr
}

func (p *localGachaProvider) GetByID(id int) (*masterdata.Gacha, error) {
	if id == 0 {
		return nil, fmt.Errorf("gacha id is required")
	}
	if err := p.ensureGachas(); err != nil {
		return nil, err
	}
	g, ok := p.gachaByID[id]
	if !ok {
		return nil, fmt.Errorf("gacha %d not found", id)
	}
	return common.CloneGacha(g), nil
}

func (p *localGachaProvider) GetAll() []*masterdata.Gacha {
	if err := p.ensureGachas(); err != nil {
		return nil
	}
	return common.CloneGachaList(p.gachaAll)
}

func (p *localGachaProvider) GetCardByID(id int) (*masterdata.Card, error) {
	if id == 0 {
		return nil, fmt.Errorf("invalid card id")
	}
	if err := p.ensureCards(); err != nil {
		return nil, err
	}
	card, ok := p.cardByID[id]
	if !ok {
		return nil, fmt.Errorf("card %d not found", id)
	}
	c := *card
	return &c, nil
}

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

// ===========================================================================
// localStampProvider
// ===========================================================================

type localStampProvider struct {
	store *localStore

	once   sync.Once
	stamps []masterdata.Stamp
	err    error
}

func (p *localStampProvider) GetAll() ([]masterdata.Stamp, error) {
	p.once.Do(func() {
		items, err := loadJSON[masterdata.Stamp](p.store, "stamps.json")
		if err != nil {
			p.err = err
			return
		}
		p.stamps = items
	})
	if p.err != nil {
		return nil, p.err
	}
	return append([]masterdata.Stamp(nil), p.stamps...), nil
}

// ===========================================================================
// localVLiveProvider
// ===========================================================================

type localVLiveProvider struct {
	store *localStore

	once  sync.Once
	lives []*VLive
	err   error
}

func (p *localVLiveProvider) ensureLoaded() error {
	p.once.Do(func() {
		items, err := loadJSON[localVirtualLiveJSON](p.store, "virtualLives.json")
		if err != nil {
			p.err = err
			return
		}
		p.lives = make([]*VLive, 0, len(items))
		for _, item := range items {
			live := &VLive{
				ID:      item.ID,
				Name:    item.Name,
				StartAt: item.StartAt,
				EndAt:   item.EndAt,
			}
			var schedules []map[string]interface{}
			if len(item.VirtualLiveSchedules) > 0 {
				_ = json.Unmarshal(item.VirtualLiveSchedules, &schedules)
			}
			for _, s := range schedules {
				startAt := vliveInt64Number(s["startAt"])
				endAt := vliveInt64Number(s["endAt"])
				if startAt <= 0 || endAt <= 0 {
					continue
				}
				live.Schedules = append(live.Schedules, VLiveSchedule{
					StartAt: startAt,
					EndAt:   endAt,
				})
			}
			p.lives = append(p.lives, live)
		}
		sort.Slice(p.lives, func(i, j int) bool {
			if p.lives[i].StartAt == p.lives[j].StartAt {
				return p.lives[i].ID < p.lives[j].ID
			}
			return p.lives[i].StartAt < p.lives[j].StartAt
		})
	})
	return p.err
}

func (p *localVLiveProvider) GetLives(_ renderregion.Value) ([]*VLive, error) {
	if err := p.ensureLoaded(); err != nil {
		return nil, err
	}
	result := make([]*VLive, 0, len(p.lives))
	for _, live := range p.lives {
		c := *live
		c.Schedules = append([]VLiveSchedule(nil), live.Schedules...)
		result = append(result, &c)
	}
	return result, nil
}

// ===========================================================================
// localEducationProvider
// ===========================================================================

type localEducationProvider struct {
	store *localStore

	rewardOnce      sync.Once
	rewardsByChar   map[int][]*ChallengeReward
	rewardErr       error

	boxOnce        sync.Once
	boxByID        map[int]*ResourceBox
	boxByPurpose   map[string]map[int]*ResourceBox
	boxErr         error

	areaOnce           sync.Once
	areaByID           map[int]*AreaItem
	areaLevelsByItem   map[int][]*AreaItemLevel
	areaLevelByItem    map[int]map[int]*AreaItemLevel
	areaErr            error

	rankOnce       sync.Once
	rankByChar     map[int]map[int]*CharacterRank
	rankErr        error

	gateOnce       sync.Once
	gateByID       map[int]map[int]*MysekaiGateLevel
	gateErr        error

	shopOnce       sync.Once
	shopByBoxID    map[int]*ShopItem
	shopErr        error
}

func (p *localEducationProvider) ensureRewards() error {
	p.rewardOnce.Do(func() {
		items, err := loadJSON[ChallengeReward](p.store, "challengeLiveHighScoreRewards.json")
		if err != nil {
			p.rewardErr = err
			return
		}
		p.rewardsByChar = make(map[int][]*ChallengeReward)
		for i := range items {
			p.rewardsByChar[items[i].CharacterID] = append(
				p.rewardsByChar[items[i].CharacterID], &items[i])
		}
	})
	return p.rewardErr
}

func (p *localEducationProvider) ensureResourceBoxes() error {
	p.boxOnce.Do(func() {
		items, err := loadJSON[ResourceBox](p.store, "resourceBoxes.json")
		if err != nil {
			p.boxErr = err
			return
		}
		p.boxByID = make(map[int]*ResourceBox, len(items))
		p.boxByPurpose = make(map[string]map[int]*ResourceBox)
		for i := range items {
			box := &items[i]
			p.boxByID[box.ID] = box
			if _, ok := p.boxByPurpose[box.ResourceBoxPurpose]; !ok {
				p.boxByPurpose[box.ResourceBoxPurpose] = make(map[int]*ResourceBox)
			}
			p.boxByPurpose[box.ResourceBoxPurpose][box.ID] = box
		}
	})
	return p.boxErr
}

func (p *localEducationProvider) ensureAreaItems() error {
	p.areaOnce.Do(func() {
		items, err := loadJSON[AreaItem](p.store, "areaItems.json")
		if err != nil {
			p.areaErr = err
			return
		}
		p.areaByID = make(map[int]*AreaItem, len(items))
		for i := range items {
			p.areaByID[items[i].ID] = &items[i]
		}

		levels, err := loadJSON[AreaItemLevel](p.store, "areaItemLevels.json")
		if err != nil {
			p.areaErr = err
			return
		}
		p.areaLevelsByItem = make(map[int][]*AreaItemLevel)
		p.areaLevelByItem = make(map[int]map[int]*AreaItemLevel)
		for i := range levels {
			lv := &levels[i]
			p.areaLevelsByItem[lv.AreaItemID] = append(p.areaLevelsByItem[lv.AreaItemID], lv)
			if _, ok := p.areaLevelByItem[lv.AreaItemID]; !ok {
				p.areaLevelByItem[lv.AreaItemID] = make(map[int]*AreaItemLevel)
			}
			p.areaLevelByItem[lv.AreaItemID][lv.Level] = lv
		}
	})
	return p.areaErr
}

func (p *localEducationProvider) ensureCharacterRanks() error {
	p.rankOnce.Do(func() {
		items, err := loadJSON[localCharacterRankJSON](p.store, "characterRanks.json")
		if err != nil {
			p.rankErr = err
			return
		}
		p.rankByChar = make(map[int]map[int]*CharacterRank)
		for _, item := range items {
			rank := &CharacterRank{
				CharacterID:     item.CharacterID,
				Rank:            item.CharacterRank,
				Power1BonusRate: item.Power1BonusRate,
			}
			if _, ok := p.rankByChar[rank.CharacterID]; !ok {
				p.rankByChar[rank.CharacterID] = make(map[int]*CharacterRank)
			}
			p.rankByChar[rank.CharacterID][rank.Rank] = rank
		}
	})
	return p.rankErr
}

func (p *localEducationProvider) ensureGateLevels() error {
	p.gateOnce.Do(func() {
		items, err := loadJSON[localMysekaiGateLevelJSON](p.store, "mysekaiGateLevels.json")
		if err != nil {
			p.gateErr = err
			return
		}
		p.gateByID = make(map[int]map[int]*MysekaiGateLevel)
		for _, item := range items {
			level := &MysekaiGateLevel{
				GateID:         item.MysekaiGateID,
				Level:          item.Level,
				PowerBonusRate: item.PowerBonusRate,
			}
			if _, ok := p.gateByID[level.GateID]; !ok {
				p.gateByID[level.GateID] = make(map[int]*MysekaiGateLevel)
			}
			p.gateByID[level.GateID][level.Level] = level
		}
	})
	return p.gateErr
}

func (p *localEducationProvider) ensureShopItems() error {
	p.shopOnce.Do(func() {
		items, err := loadJSON[localShopItemJSON](p.store, "shopItems.json")
		if err != nil {
			p.shopErr = err
			return
		}
		p.shopByBoxID = make(map[int]*ShopItem, len(items))
		for _, item := range items {
			entry := &ShopItem{
				ID:            item.ID,
				ResourceBoxID: item.ResourceBoxID,
			}
			if len(item.Costs) > 0 {
				var rawCosts []struct {
					Cost ShopItemCost `json:"cost"`
				}
				if err := json.Unmarshal(item.Costs, &rawCosts); err == nil {
					entry.Costs = make([]ShopItemCost, 0, len(rawCosts))
					for _, raw := range rawCosts {
						entry.Costs = append(entry.Costs, raw.Cost)
					}
				}
			}
			p.shopByBoxID[entry.ResourceBoxID] = entry
		}
	})
	return p.shopErr
}

func (p *localEducationProvider) GetChallengeRewardsByCharacter(charID int) []*ChallengeReward {
	if charID <= 0 {
		return nil
	}
	if err := p.ensureRewards(); err != nil {
		return nil
	}
	return cloneEdChallengeRewards(p.rewardsByChar[charID])
}

func (p *localEducationProvider) GetResourceBoxByPurpose(purpose string, id int) *ResourceBox {
	if id <= 0 {
		return nil
	}
	if err := p.ensureResourceBoxes(); err != nil {
		return nil
	}
	if strings.TrimSpace(purpose) == "" {
		return cloneEdResourceBox(p.boxByID[id])
	}
	if purposeMap, ok := p.boxByPurpose[purpose]; ok {
		return cloneEdResourceBox(purposeMap[id])
	}
	return nil
}

func (p *localEducationProvider) GetResourceBoxesByPurpose(purpose string) []*ResourceBox {
	if err := p.ensureResourceBoxes(); err != nil {
		return nil
	}
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

func (p *localEducationProvider) GetAreaItems() []*AreaItem {
	if err := p.ensureAreaItems(); err != nil {
		return nil
	}
	items := make([]*AreaItem, 0, len(p.areaByID))
	for _, item := range p.areaByID {
		items = append(items, cloneEdAreaItem(item))
	}
	return items
}

func (p *localEducationProvider) GetAreaItem(id int) *AreaItem {
	if id <= 0 {
		return nil
	}
	if err := p.ensureAreaItems(); err != nil {
		return nil
	}
	return cloneEdAreaItem(p.areaByID[id])
}

func (p *localEducationProvider) GetAreaItemLevels(areaItemID int) []*AreaItemLevel {
	if areaItemID <= 0 {
		return nil
	}
	if err := p.ensureAreaItems(); err != nil {
		return nil
	}
	return cloneEdAreaItemLevels(p.areaLevelsByItem[areaItemID])
}

func (p *localEducationProvider) GetAreaItemLevel(areaItemID, level int) *AreaItemLevel {
	if areaItemID <= 0 || level <= 0 {
		return nil
	}
	if err := p.ensureAreaItems(); err != nil {
		return nil
	}
	if levels, ok := p.areaLevelByItem[areaItemID]; ok {
		return cloneEdAreaItemLevel(levels[level])
	}
	return nil
}

func (p *localEducationProvider) GetCharacterRank(characterID, rank int) *CharacterRank {
	if characterID <= 0 || rank <= 0 {
		return nil
	}
	if err := p.ensureCharacterRanks(); err != nil {
		return nil
	}
	if ranks, ok := p.rankByChar[characterID]; ok {
		return cloneEdCharacterRank(ranks[rank])
	}
	return nil
}

func (p *localEducationProvider) GetMysekaiGateLevel(gateID, level int) *MysekaiGateLevel {
	if gateID <= 0 || level <= 0 {
		return nil
	}
	if err := p.ensureGateLevels(); err != nil {
		return nil
	}
	if levels, ok := p.gateByID[gateID]; ok {
		return cloneEdMysekaiGateLevel(levels[level])
	}
	return nil
}

func (p *localEducationProvider) GetShopItemByResourceBoxID(resourceBoxID int) *ShopItem {
	if resourceBoxID <= 0 {
		return nil
	}
	if err := p.ensureShopItems(); err != nil {
		return nil
	}
	return cloneEdShopItem(p.shopByBoxID[resourceBoxID])
}

// ===========================================================================
// localPlayerFrameProvider
// ===========================================================================

type localPlayerFrameProvider struct {
	store *localStore

	frameOnce sync.Once
	frameByID map[int]*masterdata.PlayerFrame
	frameErr  error

	groupOnce sync.Once
	groupByID map[int]*masterdata.PlayerFrameGroup
	groupErr  error
}

func (p *localPlayerFrameProvider) ensureFrames() error {
	p.frameOnce.Do(func() {
		items, err := loadJSON[masterdata.PlayerFrame](p.store, "playerFrames.json")
		if err != nil {
			p.frameErr = err
			return
		}
		p.frameByID = make(map[int]*masterdata.PlayerFrame, len(items))
		for i := range items {
			p.frameByID[items[i].ID] = &items[i]
		}
	})
	return p.frameErr
}

func (p *localPlayerFrameProvider) ensureGroups() error {
	p.groupOnce.Do(func() {
		items, err := loadJSON[masterdata.PlayerFrameGroup](p.store, "playerFrameGroups.json")
		if err != nil {
			p.groupErr = err
			return
		}
		p.groupByID = make(map[int]*masterdata.PlayerFrameGroup, len(items))
		for i := range items {
			p.groupByID[items[i].ID] = &items[i]
		}
	})
	return p.groupErr
}

func (p *localPlayerFrameProvider) GetByID(id int) (*masterdata.PlayerFrame, error) {
	if id == 0 {
		return nil, fmt.Errorf("invalid player frame id")
	}
	if err := p.ensureFrames(); err != nil {
		return nil, err
	}
	f, ok := p.frameByID[id]
	if !ok {
		return nil, fmt.Errorf("player frame %d not found", id)
	}
	c := *f
	return &c, nil
}

func (p *localPlayerFrameProvider) GetGroupByID(id int) (*masterdata.PlayerFrameGroup, error) {
	if id == 0 {
		return nil, fmt.Errorf("invalid player frame group id")
	}
	if err := p.ensureGroups(); err != nil {
		return nil, err
	}
	g, ok := p.groupByID[id]
	if !ok {
		return nil, fmt.Errorf("player frame group %d not found", id)
	}
	c := *g
	return &c, nil
}

// ===========================================================================
// localMySekaiProvider
// ===========================================================================

type localMySekaiProvider struct {
	store *localStore
}

func (p *localMySekaiProvider) Configured() bool {
	return true
}

func (p *localMySekaiProvider) LoadList(filename string) []map[string]interface{} {
	data, err := p.store.readFile(filename)
	if err != nil {
		return nil
	}
	var items []map[string]interface{}
	if err := json.Unmarshal(data, &items); err != nil {
		return nil
	}
	return items
}

func (p *localMySekaiProvider) LoadMapByID(filename string) map[int]map[string]interface{} {
	items := p.LoadList(filename)
	if items == nil {
		return nil
	}
	result := make(map[int]map[string]interface{}, len(items))
	for _, item := range items {
		if id, ok := interfaceToInt(item["id"]); ok {
			result[id] = item
		}
	}
	return result
}

func (p *localMySekaiProvider) LoadObject(filename string, target interface{}) bool {
	data, err := p.store.readFile(filename)
	if err != nil {
		return false
	}
	return json.Unmarshal(data, target) == nil
}
