package profile

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/common"
	renderhonor "haruki-cloud/internal/pjsk/render/honor"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	regionsource "haruki-cloud/internal/pjsk/render/source"
	"haruki-cloud/internal/pjsk/render/userdata"
	"haruki-cloud/utils/censor"
	"haruki-cloud/utils/drawing"
	sekai "haruki-cloud/utils/sekai"
)

var wordTagPattern = regexp.MustCompile(`<#.*?>`)

type Controller struct {
	sources  *regionsource.Registry[Source]
	drawing  *drawing.HarukiDrawingClient
	assets   *assets.AssetHelper
	snapshot *userdata.Service
	censor   *censor.Service
}

func NewController(defaultSource Source, drawingClient *drawing.HarukiDrawingClient, assetHelper *assets.AssetHelper, snapshot *userdata.Service) *Controller {
	if assetHelper == nil {
		assetHelper = assets.NewAssetHelper("", nil)
	}
	ctrl := &Controller{
		sources:  regionsource.NewRegistry[Source](renderregion.JP),
		drawing:  drawingClient,
		assets:   assetHelper,
		snapshot: snapshot,
	}
	ctrl.RegisterSource(defaultSource)
	return ctrl
}

func (c *Controller) RegisterSource(source Source) {
	if c == nil || c.sources == nil {
		return
	}
	c.sources.RegisterSource(source)
}

func (c *Controller) SetCensor(svc *censor.Service) {
	if c == nil {
		return
	}
	c.censor = svc
}

func (c *Controller) BuildProfileRequest(query Query) (*drawing.ProfileRequest, error) {
	if c == nil || c.sources == nil {
		return nil, fmt.Errorf("profile controller is not initialized")
	}
	if c.snapshot == nil {
		return nil, fmt.Errorf("local user snapshot is not configured")
	}
	if err := c.snapshot.Require(); err != nil {
		return nil, err
	}

	region := c.sources.ResolveRegion(renderregion.Normalize(query.Region))
	source, ok := c.sources.SourceForRegion(region)
	if !ok {
		return nil, fmt.Errorf("profile data source is not configured")
	}

	raw := c.snapshot.RawData()
	if raw == nil {
		return nil, fmt.Errorf("user snapshot is missing raw profile data")
	}
	detail := c.snapshot.DetailedProfile(region)
	if detail == nil {
		return nil, fmt.Errorf("user snapshot is missing profile data")
	}

	framePaths, hasFrame := c.buildFramePaths(source, raw.UserFrames)
	var framePath *string
	if framePaths != nil {
		path := framePaths.Base
		framePath = &path
	}
	updateTime := detail.UpdateTime

	nickname := detail.Nickname
	if c.censor != nil && nickname != "" {
		if !c.censor.CensorName(context.Background(), 0, detail.ID, nickname, query.Region) {
			nickname = ""
		}
	}
	word := cleanWord(raw.UserProfile.Word)
	if c.censor != nil && word != "" {
		if !c.censor.CensorShortBio(context.Background(), 0, strconv.FormatInt(raw.UserGamedata.UserID, 10), word, query.Region) {
			word = ""
		}
	}

	return &drawing.ProfileRequest{
		Profile: drawing.BasicProfile{
			ID:              detail.ID,
			Region:          detail.Region,
			Nickname:        nickname,
			IsHideUID:       detail.IsHideUID,
			LeaderImagePath: detail.LeaderImagePath,
			HasFrame:        hasFrame,
			FramePath:       framePath,
		},
		Rank:                 raw.UserGamedata.Rank,
		TwitterID:            raw.UserProfile.TwitterID,
		Word:                 word,
		Pcards:               c.buildPCards(source, raw.UserCards, raw.UserDecks, raw.UserGamedata.Deck, region),
		BgSettings:           resolveProfileBGSettings(query.BgSettings),
		Honors:               c.buildHonors(source, raw.UserProfileHonors, raw.UserHonors),
		MusicDifficultyCount: buildMusicCounts(raw.UserMusicClear, raw.UserMusicStats),
		CharacterRank:        buildCharacterRanks(raw.UserCharacters),
		SoloLive:             buildSoloLive(raw.UserChallengeLiveSoloResults, raw.UserChallengeLiveSoloStages),
		UpdateTime:           &updateTime,
		LvRankBgPath:         "user/lv_rank_bg.png",
		XIconPath:            "user/icon_twitter.png",
		IconClearPath:        "icon_clear.png",
		IconFcPath:           "icon_fc.png",
		IconApPath:           "icon_ap.png",
		CharaRankIconPathMap: buildCharaIconMap(),
		FramePaths:           framePaths,
	}, nil
}

func (c *Controller) RenderProfile(query Query) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildProfileRequest(query)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateProfile(payload)
}

// BuildProfileRequestFromAPI builds a ProfileRequest from a live GetUserProfile API response.
// framesJSON is the optional raw bytes from a ?key=userPlayerFrames toolbox key-query; pass nil
// to render without a player frame.
// query.Visible maps directly to !IsHideUID (false = hide UID, true = show UID).
// UpdateTime is always nil so that the image cache system produces a stable cache key for
// identical renders.
// UserEventResults are intentionally ignored — honor badges show the honor level,
// and FcOrApLevel is not an event-rank rendering field.
func (c *Controller) BuildProfileRequestFromAPI(query Query, resp *sekai.GetAnotherProfileResponse, framesJSON []byte) (*drawing.ProfileRequest, error) {
	if c == nil || c.sources == nil {
		return nil, fmt.Errorf("profile controller is not initialized")
	}
	if resp == nil {
		return nil, fmt.Errorf("nil API response")
	}

	region := c.sources.ResolveRegion(renderregion.Normalize(query.Region))
	source, ok := c.sources.SourceForRegion(region)
	if !ok {
		return nil, fmt.Errorf("profile data source is not configured")
	}

	leaderCard := findAPIUserCard(resp.UserCards, resp.UserDeck.Leader)
	leaderImagePath := buildLeaderImagePathFromSource(source, c.assets, resp.UserDeck.Leader, isAPICardAfterTraining(leaderCard), region)

	frames := parseFramesJSON(framesJSON)
	framePaths, hasFrame := c.buildFramePaths(source, frames)
	var framePath *string
	if framePaths != nil {
		path := framePaths.Base
		framePath = &path
	}

	adaptedCards := adaptAPICards(resp.UserCards)
	adaptedDecks := adaptAPIDeckAsList(resp.UserDeck)

	return &drawing.ProfileRequest{
		Profile: drawing.BasicProfile{
			ID:              strconv.FormatInt(resp.User.UserID, 10),
			Region:          strings.ToUpper(region.String()),
			Nickname:        resp.User.Name,
			IsHideUID:       !query.Visible,
			LeaderImagePath: leaderImagePath,
			HasFrame:        hasFrame,
			FramePath:       framePath,
		},
		Rank:                 resp.User.Rank,
		TwitterID:            resp.UserProfile.TwitterID,
		Word:                 cleanWord(resp.UserProfile.Word),
		Pcards:               c.buildPCards(source, adaptedCards, adaptedDecks, resp.UserDeck.DeckID, region),
		BgSettings:           resolveProfileBGSettings(query.BgSettings),
		Honors:               c.buildHonors(source, adaptAPIProfileHonors(resp.UserProfileHonors), adaptAPIUserHonors(resp.UserHonors)),
		MusicDifficultyCount: buildMusicCounts(adaptAPIMusicClearCount(resp.UserMusicDifficultyClearCount), nil),
		CharacterRank:        buildCharacterRanks(adaptAPICharacters(resp.UserCharacters)),
		SoloLive:             buildSoloLive(adaptAPIChallengeLiveResult(resp.UserChallengeLiveSoloResult), adaptAPIChallengeLiveStages(resp.UserChallengeLiveSoloStages)),
		UpdateTime:           nil,
		LvRankBgPath:         "user/lv_rank_bg.png",
		XIconPath:            "user/icon_twitter.png",
		IconClearPath:        "icon_clear.png",
		IconFcPath:           "icon_fc.png",
		IconApPath:           "icon_ap.png",
		CharaRankIconPathMap: buildCharaIconMap(),
		FramePaths:           framePaths,
	}, nil
}

func (c *Controller) BuildDetailedProfileCardFromAPI(query Query, resp *sekai.GetAnotherProfileResponse, framesJSON []byte) (*drawing.DetailedProfileCardRequest, error) {
	if c == nil || c.sources == nil {
		return nil, fmt.Errorf("profile controller is not initialized")
	}
	if resp == nil {
		return nil, fmt.Errorf("nil API response")
	}

	region := c.sources.ResolveRegion(renderregion.Normalize(query.Region))
	source, ok := c.sources.SourceForRegion(region)
	if !ok {
		return nil, fmt.Errorf("profile data source is not configured")
	}

	leaderCard := findAPIUserCard(resp.UserCards, resp.UserDeck.Leader)
	leaderImagePath := buildLeaderImagePathFromSource(source, c.assets, resp.UserDeck.Leader, isAPICardAfterTraining(leaderCard), region)

	frames := parseFramesJSON(framesJSON)
	framePaths, hasFrame := c.buildFramePaths(source, frames)
	var framePath *string
	if framePaths != nil {
		path := framePaths.Base
		framePath = &path
	}

	return &drawing.DetailedProfileCardRequest{
		ID:              strconv.FormatInt(resp.User.UserID, 10),
		Region:          strings.ToUpper(region.String()),
		Nickname:        resp.User.Name,
		Source:          "sekai_api_public",
		UpdateTime:      0,
		IsHideUID:       !query.Visible,
		LeaderImagePath: leaderImagePath,
		HasFrame:        hasFrame,
		FramePath:       framePath,
		UserCards:       buildAPIUserCardEntries(resp.UserDeck),
	}, nil
}

func (c *Controller) BuildProfileCardFromAPI(query Query, resp *sekai.GetAnotherProfileResponse, framesJSON []byte) (*drawing.ProfileCardRequest, error) {
	detail, err := c.BuildDetailedProfileCardFromAPI(query, resp, framesJSON)
	if err != nil {
		return nil, err
	}
	source := detail.Source
	return &drawing.ProfileCardRequest{
		Profile: &drawing.BasicProfile{
			ID:              detail.ID,
			Region:          detail.Region,
			Nickname:        detail.Nickname,
			IsHideUID:       detail.IsHideUID,
			LeaderImagePath: detail.LeaderImagePath,
			HasFrame:        detail.HasFrame,
			FramePath:       cloneStringPtr(detail.FramePath),
		},
		DataSources: []drawing.ProfileDataSource{
			{
				Name:   "Sekai API",
				Source: &source,
				Mode:   cloneStringPtr(detail.Mode),
			},
		},
	}, nil
}

// RenderProfileFromAPI is a convenience wrapper that calls BuildProfileRequestFromAPI and
// then sends the result to the drawing service.
func (c *Controller) RenderProfileFromAPI(query Query, resp *sekai.GetAnotherProfileResponse, framesJSON []byte) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildProfileRequestFromAPI(query, resp, framesJSON)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateProfile(payload)
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

func buildAPIUserCardEntries(deck sekai.UserDeck) []interface{} {
	ids := []int{deck.Leader, deck.SubLeader, deck.Member1, deck.Member2, deck.Member3, deck.Member4, deck.Member5}
	entries := make([]interface{}, 0, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		entries = append(entries, map[string]interface{}{
			"card_id": id,
		})
	}
	return entries
}

// buildLeaderImagePathFromSource resolves the leader card thumbnail path using the Source's
// master-data lookup, mirroring the logic in userdata.resolveLeaderImagePath but without
// requiring a direct ent client reference.
func buildLeaderImagePathFromSource(source Source, helper *assets.AssetHelper, cardID int, afterTraining bool, region renderregion.Value) string {
	const fallback = "user/leader.png"
	if cardID == 0 || source == nil {
		return fallback
	}
	card, err := source.GetCardByID(cardID)
	if err != nil || card == nil || strings.TrimSpace(card.AssetBundleName) == "" {
		return fallback
	}
	imageType := "normal"
	if afterTraining {
		imageType = "after_training"
	}
	return assets.ResolveAssetPath(helper, assets.RegionAssetDir(region.String()),
		filepath.Join("thumbnail", "chara", fmt.Sprintf("%s_%s.png", card.AssetBundleName, imageType)),
		filepath.Join("character", "member", card.AssetBundleName, "card_normal.png"),
	)
}

func (c *Controller) buildFramePaths(source Source, userFrames []userdata.RawUserFrame) (*drawing.PlayerFramePaths, bool) {
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

func (c *Controller) buildPCards(source Source, userCards []userdata.RawUserCard, decks []userdata.RawUserDeck, activeDeckID int, region renderregion.Value) []drawing.CardFullThumbnailRequest {
	activeDeck := findActiveDeck(decks, activeDeckID)
	memberIDs := []int{activeDeck.Member1, activeDeck.Member2, activeDeck.Member3, activeDeck.Member4, activeDeck.Member5}
	result := make([]drawing.CardFullThumbnailRequest, 0, len(memberIDs))
	for _, cardID := range memberIDs {
		if cardID == 0 {
			continue
		}
		cardInfo, err := source.GetCardByID(cardID)
		if err != nil || cardInfo == nil {
			continue
		}
		userCard := findUserCard(userCards, cardID)
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

func (c *Controller) buildHonors(source Source, profileHonors []userdata.RawUserProfileHonor, userHonors []userdata.RawUserHonor) []drawing.HonorRequest {
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
			Region:           source.DefaultRegion(),
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
			Region:     source.DefaultRegion(),
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

func (c *Controller) findEventRank(results []userdata.RawUserEventResult, eventID int) int {
	if eventID == 0 {
		return 0
	}
	for _, item := range results {
		if item.EventID == eventID {
			return item.Rank
		}
	}
	return 0
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

func buildCharaIconMap() map[string]string {
	result := make(map[string]string, len(assets.CharacterIDToNickname))
	for id, nickname := range assets.CharacterIDToNickname {
		result[strconv.Itoa(id)] = filepath.ToSlash(filepath.Join("chara_rank_icon", nickname+".png"))
	}
	return result
}

func cleanWord(word string) string {
	return wordTagPattern.ReplaceAllString(word, "")
}

func findActiveDeck(decks []userdata.RawUserDeck, activeID int) userdata.RawUserDeck {
	for _, deck := range decks {
		if deck.DeckID == activeID {
			return deck
		}
	}
	if len(decks) > 0 {
		return decks[0]
	}
	return userdata.RawUserDeck{}
}

func findUserCard(cards []userdata.RawUserCard, cardID int) *userdata.RawUserCard {
	for i := range cards {
		if cards[i].CardID == cardID {
			return &cards[i]
		}
	}
	return nil
}

func cloneStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
