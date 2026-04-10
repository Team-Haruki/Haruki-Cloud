package deck

import (
	"fmt"
	"sort"
	"strings"

	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/common"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/internal/pjsk/render/userdata"
	"haruki-cloud/utils/drawing"
)

func (c *Controller) buildDrawingRequestFromRecommendResult(region renderregion.Value, recType string, query AutoQuery, option map[string]any, result *RecommendResult) (*drawing.DeckRequest, error) {
	if result == nil || len(result.Decks) == 0 {
		return nil, fmt.Errorf("deck recommend service returned no deck results")
	}
	region, cardSource, err := c.resolveCardSource(region)
	if err != nil {
		return nil, err
	}

	profile := c.resolveProfile(region, query.Profile, "deck_remote_service")
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
			card, err := cardSource.GetCardByID(deckCard.CardID)
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

	c.applyOptionRequestFields(request, option, query)
	c.applyCommonRecommendMetadata(request, region, recType, option, query)
	return request, nil
}

func (c *Controller) applyOptionRequestFields(request *drawing.DeckRequest, option map[string]any, query AutoQuery) {
	if request == nil {
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
	if strings.TrimSpace(query.MusicTitle) != "" && request.MusicTitle == nil {
		request.MusicTitle = drawing.StringPtr(query.MusicTitle)
	}
	if strings.TrimSpace(query.MusicCoverPath) != "" && request.MusicCoverPath == nil {
		request.MusicCoverPath = drawing.StringPtr(query.MusicCoverPath)
	}
	if diff := optionString(option, "music_diff"); diff != "" {
		request.MusicDiff = drawing.StringPtr(diff)
	}
	if option == nil {
		return
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
