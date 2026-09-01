package profile

import (
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/common"
	renderhonor "haruki-cloud/internal/pjsk/render/honor"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/snapshot"
	"haruki-cloud/internal/pjsk/sekai"
)

func resolveProfileBGSettings(settings *drawing.ProfileBgSettings) *drawing.ProfileBgSettings {
	if settings == nil {
		return &drawing.ProfileBgSettings{Alpha: 100, Blur: 4, Vertical: false}
	}
	cloned := *settings
	if settings.ImgPath != nil {
		cloned.ImgPath = new(filepath.ToSlash(strings.TrimSpace(*settings.ImgPath)))
	}
	return &cloned
}

func applyProfileBGVerticalOverride(settings *drawing.ProfileBgSettings, override *bool) *drawing.ProfileBgSettings {
	resolved := resolveProfileBGSettings(settings)
	if override != nil {
		resolved.Vertical = *override
	}
	return resolved
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
// master-data lookup, mirroring the logic in snapshot.resolveLeaderImagePath but without
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
	if path := common.ResolveCardThumbnailPath(helper, region, card.AssetBundleName, afterTraining); strings.TrimSpace(path) != "" {
		return path
	}
	return fallback
}

func profileUnknownImagePath(helper *assets.AssetHelper) string {
	return assets.ResolveProfilePlaceholderPath(helper)
}

func (c *Controller) buildFramePaths(source DataSource, userFrames []snapshot.RawUserFrame) (*drawing.PlayerFramePaths, bool) {
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

func (c *Controller) buildPCards(source DataSource, userCards []snapshot.RawUserCard, decks []snapshot.RawUserDeck, activeDeckID int, region renderregion.Value) []drawing.CardFullThumbnailRequest {
	activeDeck := snapshot.FindActiveDeck(decks, activeDeckID)
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
		userCard := snapshot.FindUserCard(userCards, cardID)
		var level *int
		var trainRank *int
		if userCard != nil {
			level = new(userCard.Level)
			trainRank = new(userCard.MasterRank)
		}
		displayAfterTraining := isSnapshotCardTrainedArt(userCard)
		rareImgPath := ""
		if userCard != nil && strings.EqualFold(userCard.SpecialTrainingStatus, "done") {
			rareImgPath = common.ResolveCardRareImagePath(c.assets, cardInfo, true)
		}
		result = append(result, common.BuildCardThumbnail(c.assets, cardInfo, region, common.ThumbnailOptions{
			AfterTraining: displayAfterTraining,
			TrainedArt:    displayAfterTraining,
			RareImgPath:   rareImgPath,
			TrainRank:     trainRank,
			Level:         level,
			IsPcard:       true,
		}))
	}
	return result
}

func (c *Controller) buildHonors(source DataSource, region renderregion.Value, profileHonors []snapshot.RawUserProfileHonor, userHonors []snapshot.RawUserHonor, musicCounts []drawing.MusicClearCount) []drawing.HonorRequest {
	builder := renderhonor.NewBuilder(source, c.assets)
	fcApLevels := buildHonorFcApLevels(musicCounts)
	requests := buildSelectedProfileHonors(builder, region, profileHonors, fcApLevels)
	if len(requests) > 0 {
		return requests
	}
	return buildFallbackUserHonors(builder, region, userHonors, fcApLevels)
}

func buildSelectedProfileHonors(
	builder *renderhonor.Builder,
	region renderregion.Value,
	profileHonors []snapshot.RawUserProfileHonor,
	fcApLevels map[int]*int,
) []drawing.HonorRequest {
	selected := make([]snapshot.RawUserProfileHonor, 0, len(profileHonors))
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
			Region:              region,
			HonorID:             honorID,
			HonorLevel:          item.HonorLevel,
			IsMain:              item.Seq == 1,
			BondsHonorViewType:  item.BondsHonorViewType,
			BondsHonorWordID:    item.BondsHonorWordId,
			FcOrApLevelOverride: fcApLevels[honorID],
		})
		if err == nil && req != nil {
			requests = append(requests, *req)
		}
	}
	return requests
}

func buildFallbackUserHonors(
	builder *renderhonor.Builder,
	region renderregion.Value,
	userHonors []snapshot.RawUserHonor,
	fcApLevels map[int]*int,
) []drawing.HonorRequest {
	requests := make([]drawing.HonorRequest, 0, 3)
	for _, item := range userHonors {
		if len(requests) >= 3 {
			break
		}
		req, err := builder.BuildHonorRequest(renderhonor.Query{
			Region:              region,
			HonorID:             item.HonorID,
			HonorLevel:          item.HonorLevel,
			IsMain:              len(requests) == 0,
			FcOrApLevelOverride: fcApLevels[item.HonorID],
		})
		if err == nil && req != nil {
			requests = append(requests, *req)
		}
	}
	return requests
}

func buildHonorFcApLevels(musicCounts []drawing.MusicClearCount) map[int]*int {
	if len(musicCounts) == 0 {
		return nil
	}

	countsByDifficulty := make(map[string]drawing.MusicClearCount, len(musicCounts))
	for _, count := range musicCounts {
		difficulty := strings.ToLower(strings.TrimSpace(count.Difficulty))
		if difficulty == "" {
			continue
		}
		countsByDifficulty[difficulty] = count
	}
	if len(countsByDifficulty) == 0 {
		return nil
	}

	result := make(map[int]*int)
	for _, honorID := range []int{3009, 3010, 3011, 3012, 3013, 3014, 4700, 4701} {
		difficulty, score, ok := renderhonor.LookupFcApCounter(honorID)
		if !ok {
			continue
		}
		count, ok := countsByDifficulty[strings.ToLower(strings.TrimSpace(difficulty))]
		if !ok {
			continue
		}

		value := 0
		switch score {
		case "fullCombo":
			value = count.Fc
		case "allPerfect":
			value = count.Ap
		default:
			continue
		}

		result[honorID] = new(value)
	}
	return result
}

func buildMusicCounts(clears []snapshot.RawMusicClear, stats []snapshot.RawMusicResult) []drawing.MusicClearCount {
	difficulties := []string{"easy", "normal", "hard", "expert", "master", "append"}
	if len(clears) > 0 {
		return buildMusicCountsFromClears(difficulties, clears)
	}
	return buildMusicCountsFromResults(difficulties, stats)
}

func buildMusicCountsFromClears(difficulties []string, clears []snapshot.RawMusicClear) []drawing.MusicClearCount {
	result := make([]drawing.MusicClearCount, 0, len(difficulties))
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

func buildMusicCountsFromResults(difficulties []string, stats []snapshot.RawMusicResult) []drawing.MusicClearCount {
	result := make([]drawing.MusicClearCount, 0, len(difficulties))
	for _, difficulty := range difficulties {
		result = append(result, buildMusicResultCount(difficulty, stats))
	}
	return result
}

func buildMusicResultCount(difficulty string, stats []snapshot.RawMusicResult) drawing.MusicClearCount {
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
	return count
}

func buildCharacterRanks(ranks []snapshot.RawUserCharacter) []drawing.CharacterRank {
	result := make([]drawing.CharacterRank, 0, len(ranks))
	for _, item := range ranks {
		result = append(result, drawing.CharacterRank{
			CharacterID: item.CharacterID,
			Rank:        item.CharacterRank,
		})
	}
	return result
}

func buildSoloLive(results []snapshot.RawChallengeLiveResult, stages []snapshot.RawChallengeLiveStage) *drawing.SoloLiveRank {
	if len(results) == 0 {
		return nil
	}
	items := slices.Clone(results)
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

func buildMultiLive(count snapshot.RawUserMultiLiveTopScoreCount) *drawing.MultiLiveTopScoreCount {
	if count.MVP <= 0 && count.SuperStar <= 0 {
		return nil
	}
	return &drawing.MultiLiveTopScoreCount{
		MVP:       count.MVP,
		SuperStar: count.SuperStar,
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
