package deck

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/common"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/internal/pjsk/render/userdata"
	"haruki-cloud/utils/drawing"
)

type CardSource interface {
	DefaultRegion() renderregion.Value
	GetCardByID(id int) (*masterdata.Card, error)
}

type EventSource interface {
	GetEventByID(id int) (*masterdata.Event, error)
	GetEvents() []*masterdata.Event
}

type Controller struct {
	cards         CardSource
	events        EventSource
	drawing       *drawing.HarukiDrawingClient
	assets        *assets.AssetHelper
	snapshot      *userdata.Service
	defaultRegion renderregion.Value
	recommendCfg  RecommendConfig
	metaLoader    MusicMetaSource
	localEngine   localEngineProvider
}

func NewController(cards CardSource, events EventSource, drawingClient *drawing.HarukiDrawingClient, assetHelper *assets.AssetHelper, snapshot *userdata.Service, defaultRegion renderregion.Value) *Controller {
	return NewControllerWithConfig(cards, events, drawingClient, assetHelper, snapshot, defaultRegion, RecommendConfig{}, nil)
}

func NewControllerWithConfig(cards CardSource, events EventSource, drawingClient *drawing.HarukiDrawingClient, assetHelper *assets.AssetHelper, snapshot *userdata.Service, defaultRegion renderregion.Value, cfg RecommendConfig, metaLoader MusicMetaSource) *Controller {
	if assetHelper == nil {
		assetHelper = assets.NewAssetHelper("", nil)
	}
	controller := &Controller{
		cards:         cards,
		events:        events,
		drawing:       drawingClient,
		assets:        assetHelper,
		snapshot:      snapshot,
		defaultRegion: renderregion.WithDefault(defaultRegion),
		recommendCfg: RecommendConfig{
			Enabled:          cfg.Enabled,
			UseLocalEngine:   cfg.UseLocalEngine,
			LocalPoolSize:    cfg.LocalPoolSize,
			LocalLibraryDirs: append([]string(nil), cfg.LocalLibraryDirs...),
			StaticDataDir:    cfg.StaticDataDir,
			MasterdataDir:    cfg.MasterdataDir,
			Timeout:          cfg.Timeout,
			DefaultAlgs:      append([]string(nil), cfg.DefaultAlgs...),
		},
		metaLoader: metaLoader,
	}
	if cfg.Enabled && cfg.UseLocalEngine {
		controller.localEngine = newLocalEngineProvider(controller.recommendCfg)
	}
	return controller
}

// WithSnapshot returns a shallow copy of this Controller that uses the given
// snapshot instead of the one configured at construction time. This is used by
// the bridge layer to inject a live Toolbox snapshot on a per-request basis.
func (c *Controller) WithSnapshot(s *userdata.Service) *Controller {
	if c == nil {
		return nil
	}
	clone := *c
	clone.snapshot = s
	return &clone
}

func (c *Controller) BuildRecommendRequest(req drawing.DeckRequest) (*drawing.DeckRequest, error) {
	if strings.TrimSpace(req.Region) == "" {
		return nil, fmt.Errorf("deck request missing region")
	}
	if len(req.DeckData) == 0 {
		return nil, fmt.Errorf("deck request deck_data is empty")
	}
	return &req, nil
}

func (c *Controller) RenderRecommend(req drawing.DeckRequest) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildRecommendRequest(req)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateDeckRecommendation(payload)
}

func (c *Controller) BuildAutoRecommendRequest(query AutoQuery) (*drawing.DeckRequest, error) {
	if c == nil {
		return nil, fmt.Errorf("deck controller is not initialized")
	}
	if c.cards == nil {
		return nil, fmt.Errorf("deck card source is not configured")
	}
	if c.snapshot == nil {
		return nil, fmt.Errorf("user data is required for deck auto recommend")
	}
	if err := c.snapshot.Require(); err != nil {
		return nil, err
	}

	if c.recommendCfg.Enabled && c.recommendCfg.UseLocalEngine {
		return c.buildAutoRecommendWithEngine(query)
	}
	return c.buildAutoRecommendLocal(query)
}

func (c *Controller) buildAutoRecommendWithEngine(query AutoQuery) (*drawing.DeckRequest, error) {
	if c.localEngine == nil {
		return nil, fmt.Errorf("deck local engine is not configured")
	}

	region, recType, err := c.normalizeAutoQuery(query)
	if err != nil {
		return nil, err
	}

	userBytes, err := c.snapshot.RawBytes()
	if err != nil {
		return nil, err
	}

	recommender, err := c.localEngine.Get(region.String())
	if err != nil {
		return nil, err
	}

	option, err := c.buildRecommendOption(region, recType, query)
	if err != nil {
		return nil, err
	}

	result, err := recommender.Recommend(RecommendRequest{
		Region:      region.String(),
		UserData:    userBytes,
		MusicMeta:   c.resolveMusicMeta(region),
		BatchOption: recommender.ExpandAlgorithms(option),
	})
	if err != nil {
		return nil, err
	}

	return c.buildDrawingRequestFromRecommendResult(region, recType, query, option, result)
}

func (c *Controller) buildAutoRecommendLocal(query AutoQuery) (*drawing.DeckRequest, error) {
	raw := c.snapshot.RawData()
	if raw == nil {
		return nil, fmt.Errorf("user data is required for deck auto recommend")
	}

	region, recType, err := c.normalizeAutoQuery(query)
	if err != nil {
		return nil, err
	}
	option, err := c.buildRecommendOption(region, recType, query)
	if err != nil {
		return nil, err
	}

	type deckCandidate struct {
		card     *masterdata.Card
		userCard userdata.RawUserCard
		power    int
	}
	candidates := make([]deckCandidate, 0, len(raw.UserCards))
	for _, userCard := range raw.UserCards {
		card, err := c.cards.GetCardByID(userCard.CardID)
		if err != nil || card == nil {
			continue
		}
		candidates = append(candidates, deckCandidate{
			card:     card,
			userCard: userCard,
			power:    calculateDeckCardPower(card),
		})
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no available user cards for deck recommend")
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].power == candidates[j].power {
			return candidates[i].card.ID < candidates[j].card.ID
		}
		return candidates[i].power > candidates[j].power
	})

	limit := query.Limit
	if limit <= 0 || limit > 5 {
		limit = 5
	}
	if len(candidates) < limit {
		limit = len(candidates)
	}

	cardData := make([]drawing.DeckCardData, 0, limit)
	totalPower := 0
	for _, pick := range candidates[:limit] {
		totalPower += pick.power
		afterTraining := isAfterTraining(pick.userCard)
		cardData = append(cardData, drawing.DeckCardData{
			CardThumbnail: common.BuildCardThumbnail(c.assets, pick.card, region, common.ThumbnailOptions{
				AfterTraining: afterTraining,
				TrainedArt:    afterTraining,
				TrainRank:     drawing.IntPtr(pick.userCard.MasterRank),
				Level:         drawing.IntPtr(pick.userCard.Level),
				IsPcard:       false,
			}),
			CharaID:         pick.card.CharacterID,
			SkillLevel:      "4",
			IsAfterTraining: afterTraining,
			SkillRate:       120.0,
			EventBonusRate:  defaultEventBonus(recType),
			IsBeforeStory:   true,
			IsAfterStory:    true,
			HasCanvasBonus:  false,
		})
	}

	profile := c.resolveProfile(region, query.Profile, "local_fallback")
	score := totalPower * 3
	eventBonus := defaultEventBonus(recType)
	supportDeckBonusRate := 0.0
	multiLiveScoreUp := 20.0
	request := &drawing.DeckRequest{
		Region:              region.String(),
		Profile:             *profile,
		DeckData:            []drawing.DeckData{{CardData: cardData, Score: drawing.IntPtr(score), LiveScore: drawing.IntPtr(score), MySekaiEventPoint: drawing.IntPtr(score), EventBonusRate: &eventBonus, SupportDeckBonusRate: &supportDeckBonusRate, MultiLiveScoreUp: &multiLiveScoreUp, TotalPower: drawing.IntPtr(totalPower)}},
		RecommendType:       recType,
		Target:              drawing.StringPtr("score"),
		ModelName:           []interface{}{"dfs"},
		CostTimes:           map[string]interface{}{"dfs": 0.01},
		WaitTimes:           map[string]interface{}{"dfs": 0.0},
		CanvasThumbnailPath: drawing.StringPtr(assets.ResolveRegionAssetPath(c.assets, region.String(), "mysekai/icon/category_icon/icon_canvas.png")),
	}

	c.applyOptionRequestFields(request, option)
	c.applyCommonRecommendMetadata(request, region, recType, option, query)
	return request, nil
}

func (c *Controller) buildRecommendOption(region renderregion.Value, recType string, query AutoQuery) (map[string]interface{}, error) {
	eventID := 0
	if query.EventID != nil && *query.EventID > 0 {
		eventID = *query.EventID
	}
	if eventID == 0 && recType != "no_event" && recType != "challenge" {
		if id := c.pickCurrentOrNextEventID(); id > 0 {
			eventID = id
		}
	}

	limit := query.Limit
	if limit <= 0 {
		limit = 6
	}

	option := map[string]interface{}{
		"region":                       region.String(),
		"algorithm":                    "all",
		"timeout_ms":                   60000,
		"limit":                        limit,
		"target":                       "score",
		"live_type":                    "multi",
		"music_id":                     10000,
		"music_diff":                   "master",
		"member":                       5,
		"multi_live_teammate_power":    250000,
		"multi_live_teammate_score_up": 200,
		"rarity_1_config":              defaultDeckConfig12(),
		"rarity_2_config":              defaultDeckConfig12(),
		"rarity_3_config":              defaultDeckConfig34bd(),
		"rarity_4_config":              defaultDeckConfig34bd(),
		"rarity_birthday_config":       defaultDeckConfig34bd(),
		"single_card_configs":          []interface{}{},
		"fixed_cards":                  []int{},
		"fixed_characters":             []int{},
		"best_skill_as_leader":         true,
		"keep_after_training_state":    false,
	}

	switch recType {
	case "challenge":
		option["live_type"] = "challenge"
		option["event_id"] = nil
	case "no_event":
		option["live_type"] = "multi"
		option["event_id"] = nil
	case "bonus":
		option["algorithm"] = "dfs"
		option["live_type"] = "solo"
		option["target"] = "bonus"
		option["target_bonus_list"] = pickBonusTargets(query.TargetBonuses, query.Args)
		option["rarity_1_config"] = noChangeDeckConfig()
		option["rarity_2_config"] = noChangeDeckConfig()
		option["rarity_3_config"] = noChangeDeckConfig()
		option["rarity_4_config"] = noChangeDeckConfig()
		option["rarity_birthday_config"] = noChangeDeckConfig()
		if eventID > 0 {
			option["event_id"] = eventID
		}
	case "mysekai":
		option["algorithm"] = "ga"
		option["live_type"] = "mysekai"
		option["event_id"] = nil
		option["rarity_1_config"] = noChangeDeckConfig()
		option["rarity_2_config"] = noChangeDeckConfig()
		option["rarity_3_config"] = noChangeDeckConfig()
		option["rarity_4_config"] = noChangeDeckConfig()
		option["rarity_birthday_config"] = noChangeDeckConfig()
	default:
		if eventID > 0 {
			option["event_id"] = eventID
		}
	}

	applyRecommendOptionOverrides(option, recType, query)
	return option, nil
}

func (c *Controller) buildDrawingRequestFromRecommendResult(region renderregion.Value, recType string, query AutoQuery, option map[string]interface{}, result *RecommendResult) (*drawing.DeckRequest, error) {
	if result == nil || len(result.Decks) == 0 {
		return nil, fmt.Errorf("deck local engine returned no deck results")
	}

	profile := c.resolveProfile(region, query.Profile, "deck_local_engine")
	userCardMap := make(map[int]userdata.RawUserCard)
	if raw := c.snapshot.RawData(); raw != nil {
		for _, userCard := range raw.UserCards {
			userCardMap[userCard.CardID] = userCard
		}
	}

	deckData := make([]drawing.DeckData, 0, len(result.Decks))
	for _, deckInfo := range result.Decks {
		cardData := make([]drawing.DeckCardData, 0, len(deckInfo.Cards))
		for _, deckCard := range deckInfo.Cards {
			card, err := c.cards.GetCardByID(deckCard.CardID)
			if err != nil || card == nil {
				continue
			}

			userCard, hasUserCard := userCardMap[deckCard.CardID]
			trainedArt := strings.EqualFold(deckCard.DefaultImage, "special_training")
			originalTrained := deckCard.IsAfterTraining
			if hasUserCard {
				originalTrained = isAfterTraining(userCard)
			}

			level := deckCard.Level
			if level <= 0 {
				level = 60
			}
			masterRank := deckCard.MasterRank

			cardData = append(cardData, drawing.DeckCardData{
				CardThumbnail: common.BuildCardThumbnail(c.assets, card, region, common.ThumbnailOptions{
					AfterTraining: originalTrained,
					TrainedArt:    trainedArt,
					TrainRank:     drawing.IntPtr(masterRank),
					Level:         drawing.IntPtr(level),
					IsPcard:       true,
				}),
				CharaID:         card.CharacterID,
				SkillLevel:      fmt.Sprintf("%d", deckCard.SkillLevel),
				IsAfterTraining: originalTrained,
				SkillRate:       deckCard.SkillRate,
				EventBonusRate:  deckCard.EventBonusRate,
				IsBeforeStory:   deckCard.IsBeforeStory,
				IsAfterStory:    deckCard.IsAfterStory,
				HasCanvasBonus:  deckCard.HasCanvasBonus,
			})
		}

		if len(cardData) > 1 {
			teammates := cardData[1:]
			sort.SliceStable(teammates, func(i, j int) bool {
				left := deckInfo.Cards[i+1]
				right := deckInfo.Cards[j+1]
				if left.EventBonusRate != right.EventBonusRate {
					return left.EventBonusRate > right.EventBonusRate
				}
				if left.MasterRank != right.MasterRank {
					return left.MasterRank > right.MasterRank
				}
				if left.Level != right.Level {
					return left.Level > right.Level
				}
				return left.CardID > right.CardID
			})
		}

		deckData = append(deckData, drawing.DeckData{
			CardData:             cardData,
			Score:                drawing.IntPtr(deckInfo.Score),
			LiveScore:            drawing.IntPtr(deckInfo.LiveScore),
			MySekaiEventPoint:    drawing.IntPtr(deckInfo.MysekaiEventPoint),
			EventBonusRate:       float64Ptr(deckInfo.EventBonusRate),
			SupportDeckBonusRate: float64Ptr(deckInfo.SupportDeckBonusRate),
			MultiLiveScoreUp:     float64Ptr(deckInfo.MultiLiveScoreUp),
			TotalPower:           drawing.IntPtr(deckInfo.TotalPower),
			ChallengeScoreDelta:  drawing.IntPtr(deckInfo.ChallengeScoreDelta),
		})
	}

	target := "score"
	if value, ok := option["target"].(string); ok && strings.TrimSpace(value) != "" {
		target = value
	}
	request := &drawing.DeckRequest{
		Region:              region.String(),
		Profile:             *profile,
		DeckData:            deckData,
		RecommendType:       recType,
		Target:              drawing.StringPtr(target),
		ModelName:           toInterfaceSlice(result.DeckAlgs),
		CostTimes:           toInterfaceMap(result.CostTimes),
		WaitTimes:           toInterfaceMap(result.WaitTimes),
		CanvasThumbnailPath: drawing.StringPtr(assets.ResolveRegionAssetPath(c.assets, region.String(), "mysekai/icon/category_icon/icon_canvas.png")),
	}

	c.applyOptionRequestFields(request, option)
	c.applyCommonRecommendMetadata(request, region, recType, option, query)
	return request, nil
}

func (c *Controller) applyCommonRecommendMetadata(request *drawing.DeckRequest, region renderregion.Value, recType string, option map[string]interface{}, query AutoQuery) {
	if request == nil {
		return
	}

	liveType := optionString(option, "live_type")
	if liveType == "" {
		switch recType {
		case "challenge":
			liveType = "challenge"
		case "mysekai":
			liveType = "mysekai"
		default:
			liveType = "multi"
		}
	}
	switch liveType {
	case "solo", "challenge":
		request.LiveType = drawing.StringPtr("solo")
		request.LiveName = drawing.StringPtr("单人")
	case "auto", "challenge_auto":
		request.LiveType = drawing.StringPtr("auto")
		request.LiveName = drawing.StringPtr("自动")
	case "mysekai":
		request.LiveType = drawing.StringPtr("mysekai")
		request.LiveName = drawing.StringPtr("烤森")
	default:
		request.LiveType = drawing.StringPtr("multi")
		request.LiveName = drawing.StringPtr("协力")
	}

	finalEventID := 0
	if option != nil {
		finalEventID = optionInt(option, "event_id")
	}
	if finalEventID <= 0 && query.EventID != nil && *query.EventID > 0 {
		finalEventID = *query.EventID
	}
	if finalEventID <= 0 && shouldUseFallbackEventMetadata(recType, option) {
		finalEventID = c.pickCurrentOrNextEventID()
	}

	if optionHasUnitAttrEvent(option) && recType == "event" {
		request.RecommendType = "unit_attr"
	}
	if optionHasWorldBloom(option) {
		request.IsWl = true
		switch recType {
		case "event":
			request.RecommendType = "wl"
		case "bonus":
			request.RecommendType = "wl_bonus"
		}
		if cid := optionInt(option, "world_bloom_character_id"); cid > 0 {
			if icon := c.resolveCharacterIconPath(cid); icon != "" {
				request.WlCharaIconPath = &icon
				if request.CharaIconPath == nil {
					request.CharaIconPath = &icon
				}
			}
		}
	}
	if recType == "challenge" {
		if cid := optionInt(option, "challenge_live_character_id"); cid > 0 {
			if icon := c.resolveCharacterIconPath(cid); icon != "" {
				request.CharaIconPath = &icon
			}
		}
	}
	if unit := optionString(option, "event_unit"); unit != "" {
		if path := c.resolveUnitIconPath(unit); path != "" {
			request.UnitLogoPath = &path
		}
	}
	if attr := optionString(option, "event_attr"); attr != "" {
		if path := c.resolveAttrIconPath(attr); path != "" {
			request.AttrIconPath = &path
		}
	}
	if finalEventID > 0 {
		request.EventID = drawing.IntPtr(finalEventID)
		eventName := fmt.Sprintf("Event #%d", finalEventID)
		request.EventName = &eventName
		if c.events != nil {
			if eventInfo, err := c.events.GetEventByID(finalEventID); err == nil && eventInfo != nil {
				eventName = eventInfo.Name
				request.EventName = &eventName
				if bannerPath := c.resolveEventBannerPath(eventInfo.AssetBundleName, region); bannerPath != "" {
					request.EventBannerPath = &bannerPath
				}
			}
		}
	}
}

func applyRecommendOptionOverrides(option map[string]interface{}, recType string, query AutoQuery) {
	if option == nil {
		return
	}
	if algorithm := normalizeRecommendAlgorithm(query.Algorithm); algorithm != "" {
		option["algorithm"] = algorithm
	}
	if liveType := normalizeRecommendLiveType(recType, query.LiveType); liveType != "" {
		option["live_type"] = liveType
	}
	if target := normalizeRecommendTarget(query.Target); target != "" {
		option["target"] = target
	}
	if query.MusicID != nil && *query.MusicID > 0 {
		option["music_id"] = *query.MusicID
	}
	if diff := normalizeRecommendDifficulty(query.MusicDiff); diff != "" {
		option["music_diff"] = diff
	}
	if len(query.TargetBonuses) > 0 {
		option["target_bonus_list"] = append([]int(nil), query.TargetBonuses...)
	}

	explicitEventID := query.EventID != nil && *query.EventID > 0
	if explicitEventID {
		option["event_id"] = *query.EventID
	}

	fakeEvent := false
	if attr := normalizeRecommendAttr(query.EventAttr); attr != "" {
		option["event_attr"] = attr
		fakeEvent = true
	}
	if unit := normalizeRecommendUnit(query.EventUnit); unit != "" {
		option["event_unit"] = unit
		fakeEvent = true
	}
	if query.WorldBloomEventTurn != nil && *query.WorldBloomEventTurn > 0 {
		option["world_bloom_event_turn"] = *query.WorldBloomEventTurn
		option["event_type"] = "world_bloom"
		fakeEvent = true
	}
	if query.WorldBloomCharacterID != nil && *query.WorldBloomCharacterID > 0 {
		option["world_bloom_character_id"] = *query.WorldBloomCharacterID
	}
	if fakeEvent {
		option["event_id"] = nil
	}

	if query.ChallengeLiveCharacterID != nil && *query.ChallengeLiveCharacterID > 0 {
		option["challenge_live_character_id"] = *query.ChallengeLiveCharacterID
	}
	if len(query.FixedCards) > 0 {
		option["fixed_cards"] = append([]int(nil), query.FixedCards...)
	}
	if len(query.FixedCharacters) > 0 {
		option["fixed_characters"] = append([]int(nil), query.FixedCharacters...)
	}

	applyDeckConfigPatch(option, "rarity_1_config", query.Rarity1Config)
	applyDeckConfigPatch(option, "rarity_2_config", query.Rarity2Config)
	applyDeckConfigPatch(option, "rarity_3_config", query.Rarity3Config)
	applyDeckConfigPatch(option, "rarity_4_config", query.Rarity4Config)
	applyDeckConfigPatch(option, "rarity_birthday_config", query.RarityBirthdayConfig)
	if len(query.SingleCardConfigs) > 0 {
		option["single_card_configs"] = toSingleCardConfigInterfaces(query.SingleCardConfigs)
	}

	if query.MultiLiveTeammatePower != nil && *query.MultiLiveTeammatePower > 0 {
		option["multi_live_teammate_power"] = *query.MultiLiveTeammatePower
	}
	if query.MultiLiveTeammateScoreUp != nil && *query.MultiLiveTeammateScoreUp >= 0 {
		option["multi_live_teammate_score_up"] = *query.MultiLiveTeammateScoreUp
	}
	if query.MultiLiveScoreUpLowerBound != nil && *query.MultiLiveScoreUpLowerBound >= 0 {
		option["multi_live_score_up_lower_bound"] = *query.MultiLiveScoreUpLowerBound
	}
	if strategy := normalizeRecommendStrategy(query.SkillOrderChooseStrategy); strategy != "" {
		option["skill_order_choose_strategy"] = strategy
	}
	if strategy := normalizeRecommendStrategy(query.SkillReferenceChooseStrategy); strategy != "" {
		option["skill_reference_choose_strategy"] = strategy
	}
	if query.KeepAfterTrainingState {
		option["keep_after_training_state"] = true
	}
}

func applyDeckConfigPatch(option map[string]interface{}, key string, patch *CardConfigPatch) {
	if option == nil || patch == nil {
		return
	}
	cfg, _ := option[key].(map[string]interface{})
	if cfg == nil {
		cfg = noChangeDeckConfig()
	}
	if patch.Disable {
		cfg["disable"] = true
	}
	if patch.LevelMax {
		cfg["level_max"] = true
	}
	if patch.EpisodeRead {
		cfg["episode_read"] = true
	}
	if patch.MasterMax {
		cfg["master_max"] = true
	}
	if patch.SkillMax {
		cfg["skill_max"] = true
	}
	if patch.Canvas {
		cfg["canvas"] = true
	}
	option[key] = cfg
}

func toSingleCardConfigInterfaces(cfgs []SingleCardConfigPatch) []interface{} {
	if len(cfgs) == 0 {
		return nil
	}
	result := make([]interface{}, 0, len(cfgs))
	for _, cfg := range cfgs {
		if cfg.CardID <= 0 {
			continue
		}
		result = append(result, map[string]interface{}{
			"card_id":      cfg.CardID,
			"disable":      cfg.Disable,
			"level_max":    cfg.LevelMax,
			"episode_read": cfg.EpisodeRead,
			"master_max":   cfg.MasterMax,
			"skill_max":    cfg.SkillMax,
			"canvas":       cfg.Canvas,
		})
	}
	return result
}

func (c *Controller) applyOptionRequestFields(request *drawing.DeckRequest, option map[string]interface{}) {
	if request == nil || option == nil {
		return
	}
	if target := optionString(option, "target"); target != "" {
		request.Target = drawing.StringPtr(target)
	}
	if musicID := optionInt(option, "music_id"); musicID > 0 {
		request.MusicID = drawing.IntPtr(musicID)
		if musicID == 10000 {
			request.MusicTitle = drawing.StringPtr("おまかせ (所有歌曲平均) | 技能顺序: 平均情况 | BloomFes花前吸取: 平均值")
			request.MusicCoverPath = drawing.StringPtr("static_images/omakase.png")
		}
	}
	if diff := optionString(option, "music_diff"); diff != "" {
		request.MusicDiff = drawing.StringPtr(diff)
	}
	if teammatePower := optionInt(option, "multi_live_teammate_power"); teammatePower > 0 {
		request.MultiLiveTeammatePower = drawing.IntPtr(teammatePower)
	}
	if teammateScoreUp, ok := optionFloat(option, "multi_live_teammate_score_up"); ok {
		request.MultiLiveTeammateScoreUp = float64Ptr(teammateScoreUp)
	}
	if lowerBound, ok := optionFloat(option, "multi_live_score_up_lower_bound"); ok {
		request.MultiLiveScoreUpLowerBound = float64Ptr(lowerBound)
	}
	if fixedCards, ok := option["fixed_cards"].([]int); ok {
		request.FixedCardsID = append([]int(nil), fixedCards...)
	}
	if fixedCharacters, ok := option["fixed_characters"].([]int); ok {
		request.FixedCharactersID = append([]int(nil), fixedCharacters...)
	}
	if keepAfterTrainingState, ok := option["keep_after_training_state"].(bool); ok {
		request.KeepAfterTrainingState = keepAfterTrainingState
	}
}

func shouldUseFallbackEventMetadata(recType string, option map[string]interface{}) bool {
	switch recType {
	case "event", "bonus":
		return !optionHasSimulatedEvent(option)
	default:
		return false
	}
}

func optionHasSimulatedEvent(option map[string]interface{}) bool {
	if option == nil {
		return false
	}
	return optionHasUnitAttrEvent(option) || optionInt(option, "world_bloom_event_turn") > 0
}

func optionHasUnitAttrEvent(option map[string]interface{}) bool {
	if option == nil {
		return false
	}
	return optionString(option, "event_unit") != "" && optionString(option, "event_attr") != ""
}

func optionHasWorldBloom(option map[string]interface{}) bool {
	if option == nil {
		return false
	}
	return optionInt(option, "world_bloom_character_id") > 0 || optionInt(option, "world_bloom_event_turn") > 0
}

func optionString(option map[string]interface{}, key string) string {
	if option == nil {
		return ""
	}
	value, _ := option[key].(string)
	return strings.TrimSpace(value)
}

func optionInt(option map[string]interface{}, key string) int {
	if option == nil {
		return 0
	}
	switch value := option[key].(type) {
	case int:
		return value
	case float64:
		return int(value)
	case float32:
		return int(value)
	default:
		return 0
	}
}

func optionFloat(option map[string]interface{}, key string) (float64, bool) {
	if option == nil {
		return 0, false
	}
	switch value := option[key].(type) {
	case float64:
		return value, true
	case float32:
		return float64(value), true
	case int:
		return float64(value), true
	default:
		return 0, false
	}
}

func normalizeRecommendAlgorithm(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "dfs", "sa", "ga", "all":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return ""
	}
}

func normalizeRecommendLiveType(recType string, raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		return ""
	case "multi", "solo", "auto":
		switch recType {
		case "challenge":
			if strings.EqualFold(strings.TrimSpace(raw), "auto") {
				return "challenge_auto"
			}
			return "challenge"
		case "mysekai":
			return "mysekai"
		default:
			return strings.ToLower(strings.TrimSpace(raw))
		}
	case "challenge", "challenge_auto", "mysekai":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return ""
	}
}

func normalizeRecommendTarget(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "score", "power", "skill", "bonus":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return ""
	}
}

func normalizeRecommendDifficulty(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "easy", "normal", "hard", "expert", "master", "append":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return ""
	}
}

func normalizeRecommendAttr(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "cute", "cool", "pure", "happy", "mysterious":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return ""
	}
}

func normalizeRecommendUnit(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "light_sound", "idol", "street", "theme_park", "school_refusal", "piapro":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return ""
	}
}

func normalizeRecommendStrategy(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "max", "min", "average":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return ""
	}
}

func (c *Controller) resolveCharacterIconPath(characterID int) string {
	if c == nil || c.assets == nil || characterID <= 0 {
		return ""
	}
	if nickname, ok := assets.CharacterIDToNickname[characterID]; ok {
		return assets.ResolveAssetPath(
			c.assets,
			assets.StaticImagesDir,
			filepath.Join("chara_icon", nickname+".png"),
			filepath.Join("chara_icon", fmt.Sprintf("chr_icon_%d.png", characterID)),
		)
	}
	return assets.ResolveAssetPath(
		c.assets,
		assets.StaticImagesDir,
		filepath.Join("chara_icon", fmt.Sprintf("chr_icon_%d.png", characterID)),
	)
}

func (c *Controller) resolveUnitIconPath(unit string) string {
	icon := assets.UnitIconFilename(unit)
	if icon == "" {
		return ""
	}
	return assets.ResolveAssetPath(c.assets, assets.StaticImagesDir, icon+".png")
}

func (c *Controller) resolveAttrIconPath(attr string) string {
	attr = normalizeRecommendAttr(attr)
	if attr == "" {
		return ""
	}
	return assets.ResolveAssetPath(c.assets, assets.StaticImagesDir, filepath.Join("card", fmt.Sprintf("attr_icon_%s.png", attr)))
}

func (c *Controller) resolveProfile(region renderregion.Value, override *drawing.DetailedProfileCardRequest, source string) *drawing.DetailedProfileCardRequest {
	if override != nil {
		profile := *override
		return &profile
	}
	if c.snapshot != nil {
		if profile := c.snapshot.DetailedProfile(region); profile != nil {
			return profile
		}
	}
	return &drawing.DetailedProfileCardRequest{
		ID:              "1",
		Region:          strings.ToUpper(region.String()),
		Nickname:        "Unknown",
		Source:          source,
		UpdateTime:      time.Now().UnixMilli(),
		IsHideUID:       true,
		LeaderImagePath: "",
		HasFrame:        false,
	}
}

func (c *Controller) resolveMusicMeta(region renderregion.Value) []byte {
	if c != nil && c.metaLoader != nil {
		if payload := c.metaLoader.Get(region.String()); len(payload) > 0 {
			return payload
		}
	}
	if c != nil && c.snapshot != nil {
		return c.snapshot.MusicMetaBytes()
	}
	return nil
}

func (c *Controller) RenderAutoRecommend(query AutoQuery) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildAutoRecommendRequest(query)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateDeckRecommendation(payload)
}

func (c *Controller) normalizeAutoQuery(query AutoQuery) (renderregion.Value, string, error) {
	region := renderregion.Normalize(query.Region)
	if region.IsZero() && c.cards != nil {
		region = c.cards.DefaultRegion()
	}
	if region.IsZero() {
		region = c.defaultRegion
	}

	recType := strings.ToLower(strings.TrimSpace(query.RecommendType))
	if recType == "" {
		recType = "event"
	}
	switch recType {
	case "event", "challenge", "no_event", "bonus", "mysekai":
		return renderregion.WithDefault(region), recType, nil
	default:
		return renderregion.Unknown, "", fmt.Errorf("unsupported recommend_type: %s", recType)
	}
}

func (c *Controller) pickCurrentOrNextEventID() int {
	if c == nil || c.events == nil {
		return 0
	}
	now := time.Now().UnixMilli()
	var current *masterdata.Event
	var next *masterdata.Event
	var latest *masterdata.Event
	for _, eventInfo := range c.events.GetEvents() {
		if eventInfo == nil {
			continue
		}
		if latest == nil || eventInfo.StartAt > latest.StartAt {
			latest = eventInfo
		}
		if eventInfo.StartAt <= now && now <= eventInfo.AggregateAt {
			if current == nil || eventInfo.StartAt > current.StartAt {
				current = eventInfo
			}
			continue
		}
		if eventInfo.StartAt > now {
			if next == nil || eventInfo.StartAt < next.StartAt {
				next = eventInfo
			}
		}
	}
	if current != nil {
		return current.ID
	}
	if next != nil {
		return next.ID
	}
	if latest != nil {
		return latest.ID
	}
	return 0
}

func (c *Controller) resolveEventBannerPath(assetBundleName string, region renderregion.Value) string {
	if c == nil || c.assets == nil || strings.TrimSpace(assetBundleName) == "" {
		return ""
	}
	return assets.ResolveRegionAssetPath(
		c.assets, region.String(),
		filepath.Join("home", "banner", assetBundleName, assetBundleName+".png"),
		filepath.Join("event", assetBundleName, "banner.png"),
	)
}

func defaultDeckConfig12() map[string]interface{} {
	return map[string]interface{}{
		"disable":      false,
		"level_max":    true,
		"episode_read": true,
		"master_max":   true,
		"skill_max":    true,
		"canvas":       false,
	}
}

func defaultDeckConfig34bd() map[string]interface{} {
	return map[string]interface{}{
		"disable":      false,
		"level_max":    true,
		"episode_read": false,
		"master_max":   false,
		"skill_max":    false,
		"canvas":       false,
	}
}

func noChangeDeckConfig() map[string]interface{} {
	return map[string]interface{}{
		"disable":      false,
		"level_max":    false,
		"episode_read": false,
		"master_max":   false,
		"skill_max":    false,
		"canvas":       false,
	}
}

func defaultEventBonus(recommendType string) float64 {
	if recommendType == "event" || recommendType == "bonus" {
		return 20.0
	}
	return 0.0
}

func pickBonusTargets(list []int, args string) []int {
	if len(list) > 0 {
		return list
	}
	parts := strings.Fields(strings.TrimSpace(args))
	var values []int
	for _, part := range parts {
		value, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || value <= 0 {
			continue
		}
		values = append(values, value)
	}
	if len(values) == 0 {
		return []int{120}
	}
	return values
}

func toInterfaceSlice(values []string) []interface{} {
	if len(values) == 0 {
		return nil
	}
	result := make([]interface{}, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	return result
}

func toInterfaceMap(values map[string]float64) map[string]interface{} {
	if len(values) == 0 {
		return nil
	}
	result := make(map[string]interface{}, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func float64Ptr(value float64) *float64 {
	return &value
}

func isAfterTraining(userCard userdata.RawUserCard) bool {
	return strings.EqualFold(userCard.SpecialTrainingStatus, "done")
}

func calculateDeckCardPower(card *masterdata.Card) int {
	if card == nil {
		return 0
	}
	var p1, p2, p3 int
	for _, param := range card.CardParameters {
		switch param.CardParameterType {
		case "param1":
			if param.Power > p1 {
				p1 = param.Power
			}
		case "param2":
			if param.Power > p2 {
				p2 = param.Power
			}
		case "param3":
			if param.Power > p3 {
				p3 = param.Power
			}
		}
	}
	return p1 + p2 + p3 + card.SpecialTrainingPower1BonusFixed + card.SpecialTrainingPower2BonusFixed + card.SpecialTrainingPower3BonusFixed
}
