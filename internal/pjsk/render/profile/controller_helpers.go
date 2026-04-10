package profile

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/common"
	renderhonor "haruki-cloud/internal/pjsk/render/honor"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/internal/pjsk/render/userdata"
	"haruki-cloud/utils/drawing"
	sekai "haruki-cloud/utils/sekai"
)

func logProfilePayloadDebug(source string, payload *drawing.ProfileRequest) {
	if payload == nil {
		slog.Info("profile payload debug", "source", source, "payload_nil", true)
		return
	}

	honors := make([]any, 0, len(payload.Honors))
	for i, honor := range payload.Honors {
		honors = append(honors, map[string]any{
			"index":                   i,
			"honor_type":              stringPtrLogValue(honor.HonorType),
			"group_type":              stringPtrLogValue(honor.GroupType),
			"honor_rarity":            stringPtrLogValue(honor.HonorRarity),
			"honor_level":             honor.HonorLevel,
			"is_main_honor":           honor.IsMainHonor,
			"honor_img_path":          stringPtrLogValue(honor.HonorImgPath),
			"frame_img_path":          stringPtrLogValue(honor.FrameImgPath),
			"frame_degree_level_path": stringPtrLogValue(honor.FrameDegreeLevelImgPath),
			"rank_img_path":           stringPtrLogValue(honor.RankImgPath),
		})
	}

	slog.Info(
		"profile payload debug",
		"source", source,
		"profile_id", payload.Profile.ID,
		"region", payload.Profile.Region,
		"nickname", payload.Profile.Nickname,
		"leader_image_path", payload.Profile.LeaderImagePath,
		"lv_rank_bg_path", payload.LvRankBgPath,
		"x_icon_path", payload.XIconPath,
		"icon_clear_path", payload.IconClearPath,
		"icon_fc_path", payload.IconFcPath,
		"icon_ap_path", payload.IconApPath,
		"honors", honors,
	)
}

func stringPtrLogValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func resolveProfileBGSettings(settings *drawing.ProfileBgSettings) *drawing.ProfileBgSettings {
	if settings == nil {
		return &drawing.ProfileBgSettings{Alpha: 100, Blur: 4, Vertical: false}
	}
	cloned := *settings
	if settings.ImgPath != nil {
		path := filepath.ToSlash(strings.TrimSpace(*settings.ImgPath))
		cloned.ImgPath = &path
	}
	return &cloned
}

func buildAPIUserCardEntries(cards []sekai.AnotherUserCard, deck sekai.UserDeck) []any {
	entries := make([]any, 0, len(cards))
	seen := make(map[int]struct{}, len(cards))
	for _, card := range cards {
		if card.CardID == 0 {
			continue
		}
		if _, ok := seen[card.CardID]; ok {
			continue
		}
		seen[card.CardID] = struct{}{}
		entries = append(entries, map[string]any{
			"cardId":                card.CardID,
			"level":                 card.Level,
			"masterRank":            card.MasterRank,
			"defaultImage":          card.DefaultImage,
			"specialTrainingStatus": card.SpecialTrainingStatus,
		})
	}
	if len(entries) > 0 {
		return entries
	}

	ids := []int{deck.Leader, deck.SubLeader, deck.Member1, deck.Member2, deck.Member3, deck.Member4, deck.Member5}
	entries = make([]any, 0, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		entries = append(entries, map[string]any{
			"cardId": id,
		})
	}
	return entries
}

// buildLeaderImagePathFromSource resolves the leader card thumbnail path using the DataSource's
// master-data lookup, mirroring the logic in userdata.resolveLeaderImagePath but without
// requiring a direct ent client reference.
func (c *Controller) cardByIDWithFallback(source DataSource, region renderregion.Value, cardID int) (*masterdata.Card, error) {
	if cardID == 0 || source == nil {
		return nil, fmt.Errorf("card not found: %d", cardID)
	}
	if card, err := source.GetCardByID(cardID); err == nil && card != nil && strings.TrimSpace(card.AssetBundleName) != "" {
		return card, nil
	}
	if c == nil || c.sources == nil || renderregion.WithDefault(region) == renderregion.JP {
		return nil, fmt.Errorf("card not found: %d", cardID)
	}
	jpSource, ok := c.sources.SourceForRegion(renderregion.JP)
	if !ok || jpSource == nil || jpSource == source {
		return nil, fmt.Errorf("card not found: %d", cardID)
	}
	card, err := jpSource.GetCardByID(cardID)
	if err != nil || card == nil || strings.TrimSpace(card.AssetBundleName) == "" {
		return nil, fmt.Errorf("card not found: %d", cardID)
	}
	return card, nil
}

func (c *Controller) buildLeaderImagePathFromSource(source DataSource, cardID int, afterTraining bool, region renderregion.Value) string {
	helper := c.assets
	fallback := profileUnknownImagePath(helper)
	if cardID == 0 {
		return fallback
	}
	card, err := c.cardByIDWithFallback(source, region, cardID)
	if err != nil || card == nil || strings.TrimSpace(card.AssetBundleName) == "" {
		return fallback
	}
	imageType := "normal"
	if afterTraining {
		imageType = "after_training"
	}
	return assets.ResolveRegionAssetPath(helper, region.String(),
		filepath.Join("thumbnail", "chara", fmt.Sprintf("%s_%s.png", card.AssetBundleName, imageType)),
		filepath.Join("character", "member", card.AssetBundleName, "card_normal.png"),
	)
}

func (c *Controller) buildProfileImagePathFromSource(
	source DataSource,
	profileCardID int,
	profileAfterTraining bool,
	leaderCardID int,
	leaderAfterTraining bool,
	region renderregion.Value,
) string {
	helper := c.assets
	fallback := profileUnknownImagePath(helper)

	if path := c.buildLeaderImagePathFromSource(source, profileCardID, profileAfterTraining, region); path != fallback {
		return path
	}
	if profileCardID != leaderCardID {
		if path := c.buildLeaderImagePathFromSource(source, leaderCardID, leaderAfterTraining, region); path != fallback {
			return path
		}
	}
	return fallback
}

func profileUnknownImagePath(helper *assets.AssetHelper) string {
	return assets.ResolveAssetPath(helper, assets.StaticImagesDir, "unknown.jpg")
}

func (c *Controller) buildFramePaths(source DataSource, userFrames []userdata.RawUserFrame) (*drawing.PlayerFramePaths, bool) {
	equippedID := 0
	for _, item := range userFrames {
		if strings.EqualFold(item.PlayerFrameAttachStatus, "equipped") {
			equippedID = item.PlayerFrameID
			break
		}
	}
	if equippedID == 0 {
		return nil, false
	}

	frame, err := source.GetPlayerFrameByID(equippedID)
	if err != nil {
		return nil, false
	}
	group, err := source.GetPlayerFrameGroupByID(frame.PlayerFrameGroupID)
	if err != nil || strings.TrimSpace(group.AssetBundleName) == "" {
		return nil, false
	}

	base := filepath.ToSlash(filepath.Join("player_frame", group.AssetBundleName, strconv.Itoa(equippedID)))
	return &drawing.PlayerFramePaths{
		Base:        filepath.ToSlash(filepath.Join(base, "horizontal", "frame_base.png")),
		CenterTop:   filepath.ToSlash(filepath.Join(base, "vertical", "frame_centertop.png")),
		LeftBottom:  filepath.ToSlash(filepath.Join(base, "vertical", "frame_leftbottom.png")),
		LeftTop:     filepath.ToSlash(filepath.Join(base, "horizontal", "frame_lefttop.png")),
		RightBottom: filepath.ToSlash(filepath.Join(base, "horizontal", "frame_rightbottom.png")),
		RightTop:    filepath.ToSlash(filepath.Join(base, "horizontal", "frame_righttop.png")),
	}, true
}

func (c *Controller) buildPCards(source DataSource, userCards []userdata.RawUserCard, decks []userdata.RawUserDeck, activeDeckID int, region renderregion.Value) []drawing.CardFullThumbnailRequest {
	activeDeck := userdata.FindActiveDeck(decks, activeDeckID)
	memberIDs := []int{activeDeck.Member1, activeDeck.Member2, activeDeck.Member3, activeDeck.Member4, activeDeck.Member5}
	result := make([]drawing.CardFullThumbnailRequest, 0, len(memberIDs))
	for _, cardID := range memberIDs {
		if cardID == 0 {
			continue
		}
		cardInfo, err := c.cardByIDWithFallback(source, region, cardID)
		if err != nil || cardInfo == nil {
			continue
		}
		userCard := userdata.FindUserCard(userCards, cardID)
		var level *int
		if userCard != nil {
			value := userCard.Level
			level = &value
		}
		result = append(result, common.BuildCardThumbnail(c.assets, cardInfo, region, common.ThumbnailOptions{
			AfterTraining: userCard != nil && strings.EqualFold(userCard.SpecialTrainingStatus, "done"),
			TrainedArt:    userCard != nil && strings.EqualFold(userCard.DefaultImage, "special_training"),
			Level:         level,
			IsPcard:       true,
		}))
	}
	return result
}

func (c *Controller) buildHonors(source DataSource, region renderregion.Value, profileHonors []userdata.RawUserProfileHonor, userHonors []userdata.RawUserHonor) []drawing.HonorRequest {
	builder := renderhonor.NewBuilder(source, c.assets)
	selected := make([]userdata.RawUserProfileHonor, 0, len(profileHonors))
	for _, item := range profileHonors {
		if item.HonorID > 0 || item.HonorId2 > 0 {
			selected = append(selected, item)
		}
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].Seq < selected[j].Seq })

	requests := make([]drawing.HonorRequest, 0, 3)
	for _, item := range selected {
		honorID := item.HonorID
		if honorID == 0 {
			honorID = item.HonorId2
		}
		req, err := builder.BuildHonorRequest(renderhonor.Query{
			Region:           region,
			HonorID:          honorID,
			HonorLevel:       item.HonorLevel,
			IsMain:           item.Seq == 1,
			BondsHonorWordID: item.BondsHonorWordId,
		})
		if err == nil && req != nil {
			requests = append(requests, *req)
		}
	}
	if len(requests) > 0 {
		return requests
	}

	for _, item := range userHonors {
		if len(requests) >= 3 {
			break
		}
		req, err := builder.BuildHonorRequest(renderhonor.Query{
			Region:     region,
			HonorID:    item.HonorID,
			HonorLevel: item.HonorLevel,
			IsMain:     len(requests) == 0,
		})
		if err == nil && req != nil {
			requests = append(requests, *req)
		}
	}
	return requests
}

func buildMusicCounts(clears []userdata.RawMusicClear, stats []userdata.RawMusicResult) []drawing.MusicClearCount {
	difficulties := []string{"easy", "normal", "hard", "expert", "master", "append"}
	result := make([]drawing.MusicClearCount, 0, len(difficulties))

	if len(clears) > 0 {
		for _, difficulty := range difficulties {
			count := drawing.MusicClearCount{Difficulty: difficulty}
			for _, item := range clears {
				if strings.EqualFold(item.MusicDifficultyType, difficulty) {
					count.Clear = item.LiveClear
					count.Fc = item.FullCombo
					count.Ap = item.AllPerfect
					break
				}
			}
			result = append(result, count)
		}
		return result
	}

	for _, difficulty := range difficulties {
		count := drawing.MusicClearCount{Difficulty: difficulty}
		seen := make(map[int]struct{})
		for _, item := range stats {
			if !strings.EqualFold(item.MusicDifficultyType, difficulty) {
				continue
			}
			if _, ok := seen[item.MusicID]; ok {
				continue
			}
			seen[item.MusicID] = struct{}{}
			count.Clear++
			if item.FullComboFlg {
				count.Fc++
			}
			if item.FullPerfectFlg {
				count.Ap++
			}
		}
		result = append(result, count)
	}
	return result
}

func buildCharacterRanks(ranks []userdata.RawUserCharacter) []drawing.CharacterRank {
	result := make([]drawing.CharacterRank, 0, len(ranks))
	for _, item := range ranks {
		result = append(result, drawing.CharacterRank{
			CharacterID: item.CharacterID,
			Rank:        item.CharacterRank,
		})
	}
	return result
}

func buildSoloLive(results []userdata.RawChallengeLiveResult, stages []userdata.RawChallengeLiveStage) *drawing.SoloLiveRank {
	if len(results) == 0 {
		return nil
	}
	items := append([]userdata.RawChallengeLiveResult(nil), results...)
	sort.Slice(items, func(i, j int) bool { return items[i].HighScore > items[j].HighScore })
	top := items[0]
	rank := 1
	for _, item := range stages {
		if item.CharacterID == top.CharacterID && item.Rank > rank {
			rank = item.Rank
		}
	}
	return &drawing.SoloLiveRank{
		CharacterID: top.CharacterID,
		Score:       top.HighScore,
		Rank:        rank,
	}
}

func buildCharaIconMap(helper *assets.AssetHelper) map[string]string {
	result := make(map[string]string, len(assets.CharacterIDToNickname))
	for id, nickname := range assets.CharacterIDToNickname {
		result[strconv.Itoa(id)] = assets.ResolveAssetPath(helper, assets.StaticImagesDir, filepath.Join("chara_rank_icon", nickname+".png"))
	}
	return result
}

func cleanWord(word string) string {
	return wordTagPattern.ReplaceAllString(word, "")
}
