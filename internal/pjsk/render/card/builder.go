package card

import (
	"fmt"
	"path/filepath"
	"strings"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/common"
	"haruki-cloud/internal/pjsk/render/event"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

func NewBuilder(source DataSource, translation DataSource, eventSource event.DataSource, assetHelper *assets.AssetHelper) *Builder {
	if assetHelper == nil {
		assetHelper = assets.NewAssetHelper("", nil)
	}
	return &Builder{
		source:      source,
		translation: translation,
		events:      eventSource,
		assets:      assetHelper,
	}
}

func (b *Builder) BuildCardDetailRequest(card *masterdata.Card, region renderregion.Value) (*drawing.CardDetailRequest, error) {
	if card == nil {
		return nil, fmt.Errorf("card is required")
	}

	cardInfo := b.BuildCardBasic(card, region)
	supplyType := formatSupplyTypeForDetail(b.source.GetCardSupplyType(card))
	if strings.TrimSpace(supplyType) != "" {
		cardInfo.SupplyType = &supplyType
	}

	eventDetails := b.buildCardEventDetails(card.ID, region)

	var gachaInfo *drawing.CardGachaInfo
	if gachaInfoModel, err := b.source.GetGachaByCardID(card.ID); err == nil && gachaInfoModel != nil {
		gachaInfo = &drawing.CardGachaInfo{
			GachaID:         gachaInfoModel.ID,
			GachaName:       gachaInfoModel.Name,
			StartAt:         gachaInfoModel.StartAt,
			EndAt:           (gachaInfoModel.EndAt/1000 + 1) * 1000,
			GachaBannerPath: b.buildGachaBannerPath(gachaInfoModel.ID, region),
		}
	}

	return &drawing.CardDetailRequest{
		CardInfo:           cardInfo,
		Region:             region.String(),
		EventInfo:          eventDetails.info,
		GachaInfo:          gachaInfo,
		CardImagesPath:     b.buildCardImagePaths(card, region),
		CostumeImagesPath:  b.buildCostumeImagePaths(card, region),
		CharacterIconPath:  b.BuildCharacterIconPath(card.CharacterID, stringValue(cardInfo.Unit), region),
		UnitLogoPath:       b.buildUnitLogoPath(stringValue(cardInfo.Unit), region),
		EventAttrIconPath:  eventDetails.attrIcon,
		EventUnitIconPath:  eventDetails.unitIcon,
		EventCharaIconPath: eventDetails.characterIcon,
	}, nil
}

type cardEventDetails struct {
	info          *drawing.CardEventInfo
	attrIcon      *string
	unitIcon      *string
	characterIcon *string
}

func (b *Builder) buildCardEventDetails(cardID int, region renderregion.Value) cardEventDetails {
	if b.events == nil {
		return cardEventDetails{}
	}
	model, err := b.events.GetEventByCardID(cardID)
	if err != nil || model == nil {
		return cardEventDetails{}
	}
	details := cardEventDetails{info: &drawing.CardEventInfo{
		EventID:         model.ID,
		EventName:       model.Name,
		StartAt:         model.StartAt,
		EndAt:           model.AggregateAt + 1000,
		EventBannerPath: b.buildEventBannerPath(model.AssetBundleName, region),
	}}
	bonuses, err := b.events.GetEventDeckBonuses(model.ID)
	if err != nil {
		return details
	}
	b.applyCardEventBonuses(&details, model.ID, bonuses, region)
	return details
}

func (b *Builder) applyCardEventBonuses(details *cardEventDetails, eventID int, bonuses []*masterdata.EventDeckBonus, region renderregion.Value) {
	if attr, ok := eventBonusAttribute(bonuses); ok {
		details.info.BonusAttr = &attr
		details.attrIcon = new(assets.ResolveAssetPath(b.assets, assets.StaticImagesDir, filepath.Join("card", fmt.Sprintf("attr_icon_%s.png", attr))))
	}
	unit, ok := b.singleEventBonusUnit(bonuses)
	if !ok {
		return
	}
	details.info.Unit = &unit
	if iconName := assets.UnitIconFilename(unit); iconName != "" {
		details.unitIcon = new(assets.ResolveAssetPath(b.assets, assets.StaticImagesDir, iconName+".png"))
	}
	b.applyEventBannerCharacter(details, eventID, unit, region)
}

func eventBonusAttribute(bonuses []*masterdata.EventDeckBonus) (string, bool) {
	var attr string
	for _, bonus := range bonuses {
		if bonus != nil && bonus.CardAttr != "" {
			attr = bonus.CardAttr
		}
	}
	return attr, attr != ""
}

func (b *Builder) singleEventBonusUnit(bonuses []*masterdata.EventDeckBonus) (string, bool) {
	units := make(map[string]struct{})
	for _, bonus := range bonuses {
		if bonus == nil || bonus.GameCharacterUnitID <= 0 {
			continue
		}
		unit, err := b.events.GetGameCharacterUnit(bonus.GameCharacterUnitID)
		if err == nil && unit != nil {
			units[unit.Unit] = struct{}{}
		}
	}
	if len(units) != 1 {
		return "", false
	}
	for unit := range units {
		return unit, true
	}
	return "", false
}

func (b *Builder) applyEventBannerCharacter(details *cardEventDetails, eventID int, unit string, region renderregion.Value) {
	characterID, err := b.events.GetEventBannerCharacterID(eventID)
	if err != nil {
		return
	}
	details.info.BannerCid = &characterID
	path := b.BuildCharacterIconPath(characterID, unit, region)
	if path != "" {
		details.characterIcon = &path
	}
}

func (b *Builder) BuildCardListRequest(cardIDs []int, region renderregion.Value) (*drawing.CardListRequest, error) {
	if len(cardIDs) == 0 {
		return nil, fmt.Errorf("card ids are required")
	}

	cards := make([]drawing.CardBasic, 0, len(cardIDs))
	for _, cardID := range cardIDs {
		card, err := b.source.GetCardByID(cardID)
		if err != nil || card == nil {
			continue
		}
		cardInfo := b.BuildCardBasic(card, region)
		if cardInfo.SupplyType != nil && *cardInfo.SupplyType == "甯搁┗" {
			cardInfo.SupplyType = new("normal")
		}
		cards = append(cards, cardInfo)
	}
	if len(cards) == 0 {
		return nil, fmt.Errorf("no valid cards found from provided ids")
	}

	return &drawing.CardListRequest{
		Cards:               cards,
		Region:              region.String(),
		TermLimitedIconPath: new(assets.ResolveAssetPath(b.assets, assets.StaticImagesDir, filepath.Join("card", "term_limited.png"))),
		FesLimitedIconPath:  new(assets.ResolveAssetPath(b.assets, assets.StaticImagesDir, filepath.Join("card", "fes_limited.png"))),
	}, nil
}

func (b *Builder) BuildCardBoxRequest(cards []*masterdata.Card, region renderregion.Value, detailedProfile *drawing.DetailedProfileCardRequest, showID, showBox, unownedOnly, useAfterTraining bool, groupBy string) (*drawing.CardBoxRequest, error) {
	if len(cards) == 0 {
		return nil, fmt.Errorf("cards are required")
	}

	ownedCards := extractOwnedCards(detailedProfile)
	items := make([]drawing.UserCard, 0, len(cards))
	characterIconPaths := make(map[int]string)
	characterColorCodes := make(map[int]string)
	for _, card := range cards {
		if card == nil {
			continue
		}
		cardInfo := b.buildCardBasic(card, region, cardBasicBuildOptions{})
		userCard, owned := ownedCards[card.ID]
		if owned {
			cardInfo.ThumbnailInfo = b.buildBoxThumbnailInfo(card, region, &userCard, useAfterTraining)
		} else {
			cardInfo.ThumbnailInfo = b.buildBoxThumbnailInfo(card, region, nil, useAfterTraining)
		}
		cardInfo.IsAfterTraining = common.BoolPtr(resolveCardBoxAfterTraining(cardInfo, userCard, useAfterTraining, owned))
		items = append(items, drawing.UserCard{
			Card:    cardInfo,
			HasCard: owned,
		})
		if _, exists := characterIconPaths[card.CharacterID]; !exists {
			characterIconPaths[card.CharacterID] = b.BuildCharacterIconPath(card.CharacterID, stringValue(cardInfo.Unit), region)
			if colorCode, ok := b.source.GetCharacterColorCode(card.CharacterID); ok {
				characterColorCodes[card.CharacterID] = colorCode
			}
		}
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("cards are required")
	}
	return &drawing.CardBoxRequest{
		Cards:               items,
		Region:              region.String(),
		ShowID:              showID,
		ShowBox:             showBox,
		UnownedOnly:         unownedOnly,
		GroupBy:             normalizeCardBoxGroupBy(groupBy),
		Distribution:        b.buildCardBoxDistribution(items, characterIconPaths, characterColorCodes, len(ownedCards) > 0, region),
		CharacterIconPaths:  characterIconPaths,
		CharacterColorCodes: characterColorCodes,
		TermLimitedIconPath: new(assets.ResolveAssetPath(b.assets, assets.StaticImagesDir, filepath.Join("card", "term_limited.png"))),
		FesLimitedIconPath:  new(assets.ResolveAssetPath(b.assets, assets.StaticImagesDir, filepath.Join("card", "fes_limited.png"))),
	}, nil
}

func hasOwnedCardData(detailedProfile *drawing.DetailedProfileCardRequest) bool {
	return len(extractOwnedCards(detailedProfile)) > 0
}

type ownedCardState struct {
	CardID                int
	Level                 int
	MasterRank            int
	SpecialTrainingStatus string
	DefaultImage          string
}

func extractOwnedCards(detailedProfile *drawing.DetailedProfileCardRequest) map[int]ownedCardState {
	if detailedProfile == nil || len(detailedProfile.UserCards) == 0 {
		return nil
	}
	owned := make(map[int]ownedCardState, len(detailedProfile.UserCards))
	for _, raw := range detailedProfile.UserCards {
		switch item := raw.(type) {
		case map[string]any:
			state := ownedCardState{
				CardID:                intValue(item["cardId"], item["card_id"]),
				Level:                 intValue(item["level"]),
				MasterRank:            intValue(item["masterRank"], item["master_rank"]),
				SpecialTrainingStatus: stringValueAny(item["specialTrainingStatus"], item["special_training_status"]),
				DefaultImage:          stringValueAny(item["defaultImage"], item["default_image"]),
			}
			if state.CardID > 0 {
				owned[state.CardID] = state
			}
		}
	}
	return owned
}

func resolveCardBoxAfterTraining(card drawing.CardBasic, state ownedCardState, useAfterTraining bool, hasOwnedState bool) bool {
	// Cards without after-training art (rarity_1, rarity_2) are always "before".
	rare := ""
	if card.Rare != nil {
		rare = *card.Rare
	}
	if rare != "rarity_3" && rare != "rarity_4" && rare != "rarity_birthday" {
		return false
	}
	if !hasOwnedState {
		return useAfterTraining
	}
	if strings.TrimSpace(state.DefaultImage) == "" && strings.TrimSpace(state.SpecialTrainingStatus) == "" {
		return useAfterTraining
	}
	if !strings.EqualFold(strings.TrimSpace(state.SpecialTrainingStatus), "done") {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(state.DefaultImage), "special_training")
}
func (b *Builder) buildBoxThumbnailInfo(card *masterdata.Card, region renderregion.Value, userCard *ownedCardState, useAfterTraining bool) []drawing.CardFullThumbnailRequest {
	if card == nil {
		return nil
	}

	if userCard != nil {
		afterTraining := strings.EqualFold(userCard.DefaultImage, "special_training") && hasAfterTrainingCard(card)
		rareImageType := "normal"
		if strings.EqualFold(userCard.SpecialTrainingStatus, "done") {
			rareImageType = "after_training"
		}
		thumbnailPath := common.ResolveCardThumbnailPath(b.assets, region, card.AssetBundleName, afterTraining)
		rareImgPath := assets.ResolveAssetPath(b.assets, assets.StaticImagesDir, filepath.Join("card", "rare_star_"+rareImageType+".png"))
		if card.CardRarityType == "rarity_birthday" {
			rareImgPath = assets.ResolveAssetPath(b.assets, assets.StaticImagesDir, filepath.Join("card", "rare_birthday.png"))
		}
		return []drawing.CardFullThumbnailRequest{
			common.BuildCardThumbnail(b.assets, card, region, common.ThumbnailOptions{
				AfterTraining: afterTraining,
				ThumbnailPath: thumbnailPath,
				RareImgPath:   rareImgPath,
				TrainRank:     drawing.IntPtr(userCard.MasterRank),
				Level:         drawing.IntPtr(userCard.Level),
				IsPcard:       true,
			}),
		}
	}

	afterTraining := useAfterTraining && hasAfterTrainingCard(card)
	if onlyHasAfterTrainingCard(card) {
		afterTraining = true
	}
	return []drawing.CardFullThumbnailRequest{
		common.BuildCardThumbnail(b.assets, card, region, common.ThumbnailOptions{
			AfterTraining: afterTraining,
			TrainedArt:    afterTraining,
		}),
	}
}

func hasAfterTrainingCard(card *masterdata.Card) bool {
	if card == nil {
		return false
	}
	return card.CardRarityType == "rarity_3" || card.CardRarityType == "rarity_4"
}

func onlyHasAfterTrainingCard(card *masterdata.Card) bool {
	return card != nil && strings.EqualFold(card.InitialSpecialTrainingStatus, "done")
}

func intValue(values ...any) int {
	for _, raw := range values {
		switch value := raw.(type) {
		case int:
			return value
		case int64:
			return int(value)
		case float64:
			return int(value)
		}
	}
	return 0
}

func stringValueAny(values ...any) string {
	for _, raw := range values {
		if value, ok := raw.(string); ok {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
