package deck

import (
	"fmt"
	"path/filepath"
	"sort"
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
}

func NewController(cards CardSource, events EventSource, drawingClient *drawing.HarukiDrawingClient, assetHelper *assets.AssetHelper, snapshot *userdata.Service, defaultRegion renderregion.Value) *Controller {
	if assetHelper == nil {
		assetHelper = assets.NewAssetHelper("", nil)
	}
	return &Controller{
		cards:         cards,
		events:        events,
		drawing:       drawingClient,
		assets:        assetHelper,
		snapshot:      snapshot,
		defaultRegion: renderregion.WithDefault(defaultRegion),
	}
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
	raw := c.snapshot.RawData()
	if raw == nil {
		return nil, fmt.Errorf("user data is required for deck auto recommend")
	}

	region, recType, err := c.normalizeAutoQuery(query)
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
			CardThumbnail: common.BuildCardThumbnail(c.assets, pick.card, common.ThumbnailOptions{
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

	profile := c.snapshot.DetailedProfile(region)
	if profile == nil {
		profile = &drawing.DetailedProfileCardRequest{
			ID:              "1",
			Region:          strings.ToUpper(region.String()),
			Nickname:        "Unknown",
			Source:          "local_fallback",
			UpdateTime:      time.Now().UnixMilli(),
			IsHideUID:       true,
			LeaderImagePath: "",
			HasFrame:        false,
		}
	}

	score := totalPower * 3
	eventBonus := defaultEventBonus(recType)
	supportDeckBonusRate := 0.0
	multiLiveScoreUp := 20.0
	deckData := []drawing.DeckData{
		{
			CardData:             cardData,
			Score:                drawing.IntPtr(score),
			LiveScore:            drawing.IntPtr(score),
			MySekaiEventPoint:    drawing.IntPtr(score),
			EventBonusRate:       &eventBonus,
			SupportDeckBonusRate: &supportDeckBonusRate,
			MultiLiveScoreUp:     &multiLiveScoreUp,
			TotalPower:           drawing.IntPtr(totalPower),
		},
	}

	target := "score"
	canvasPath := "mysekai/icon/category_icon/icon_canvas.png"
	request := &drawing.DeckRequest{
		Region:              region.String(),
		Profile:             *profile,
		DeckData:            deckData,
		RecommendType:       recType,
		Target:              &target,
		ModelName:           []interface{}{"dfs"},
		CostTimes:           map[string]interface{}{"dfs": 0.01},
		WaitTimes:           map[string]interface{}{"dfs": 0.0},
		CanvasThumbnailPath: &canvasPath,
	}

	switch recType {
	case "challenge":
		liveType := "single"
		liveName := "单人"
		request.LiveType = &liveType
		request.LiveName = &liveName
	case "mysekai":
		liveType := "mysekai"
		liveName := "烤森"
		request.LiveType = &liveType
		request.LiveName = &liveName
	default:
		liveType := "multi"
		liveName := "协力"
		request.LiveType = &liveType
		request.LiveName = &liveName
	}

	var finalEventID int
	if query.EventID != nil && *query.EventID > 0 {
		finalEventID = *query.EventID
	} else if recType != "no_event" && recType != "challenge" {
		finalEventID = c.pickCurrentOrNextEventID()
	}
	if finalEventID > 0 {
		request.EventID = drawing.IntPtr(finalEventID)
		eventName := fmt.Sprintf("Event #%d", finalEventID)
		request.EventName = &eventName
		if c.events != nil {
			if eventInfo, err := c.events.GetEventByID(finalEventID); err == nil && eventInfo != nil {
				eventName = eventInfo.Name
				request.EventName = &eventName
				if bannerPath := c.resolveEventBannerPath(eventInfo.AssetBundleName); bannerPath != "" {
					request.EventBannerPath = &bannerPath
				}
			}
		}
	}

	return request, nil
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

func (c *Controller) resolveEventBannerPath(assetBundleName string) string {
	if c == nil || c.assets == nil || strings.TrimSpace(assetBundleName) == "" {
		return ""
	}
	return assets.ResolveAssetPath(
		c.assets,
		"",
		filepath.Join("home", "banner", assetBundleName, assetBundleName+".png"),
		filepath.Join("event", assetBundleName, "banner.png"),
	)
}

func defaultEventBonus(recommendType string) float64 {
	if recommendType == "event" || recommendType == "bonus" {
		return 20.0
	}
	return 0.0
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
