package profile

import (
	"fmt"
	"strconv"

	"haruki-cloud/internal/pjsk/render/assets"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/drawing"
)

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
	musicCounts := buildMusicCounts(raw.UserMusicClear, raw.UserMusicStats)

	nickname := detail.Nickname
	if c.censor != nil && nickname != "" {
		if !c.censor.CensorName(c.contextOrBackground(), 0, detail.ID, nickname, query.Region) {
			nickname = ""
		}
	}
	word := cleanWord(raw.UserProfile.Word)
	if c.censor != nil && word != "" {
		if !c.censor.CensorShortBio(c.contextOrBackground(), 0, strconv.FormatInt(raw.UserGamedata.UserID, 10), word, query.Region) {
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
		BgSettings:           applyProfileBGVerticalOverride(query.BgSettings, query.VerticalOverride),
		Honors:               c.buildHonors(source, region, raw.UserProfileHonors, raw.UserHonors, musicCounts),
		MusicDifficultyCount: musicCounts,
		CharacterRank:        buildCharacterRanks(raw.UserCharacters),
		SoloLive:             buildSoloLive(raw.UserChallengeLiveSoloResults, raw.UserChallengeLiveSoloStages),
		UpdateTime:           &updateTime,
		LvRankBgPath:         assets.ResolveAssetPath(c.assets, assets.StaticImagesDir, "lv_rank_bg.png"),
		XIconPath:            assets.ResolveAssetPath(c.assets, assets.StaticImagesDir, "x_icon.png"),
		IconClearPath:        assets.ResolveAssetPath(c.assets, assets.StaticImagesDir, "icon_clear.png"),
		IconFcPath:           assets.ResolveAssetPath(c.assets, assets.StaticImagesDir, "icon_fc.png"),
		IconApPath:           assets.ResolveAssetPath(c.assets, assets.StaticImagesDir, "icon_ap.png"),
		CharaRankIconPathMap: buildCharaIconMap(c.assets),
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
	logProfilePayloadDebug("snapshot", payload)
	return c.drawing.GenerateProfile(payload)
}

func (c *Controller) SnapshotDetailedProfile(region renderregion.Value) *drawing.DetailedProfileCardRequest {
	if c == nil || c.snapshot == nil {
		return nil
	}
	return c.snapshot.DetailedProfile(region)
}
