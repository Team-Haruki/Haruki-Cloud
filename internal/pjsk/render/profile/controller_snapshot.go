package profile

import (
	"fmt"
	"strconv"
	"sync"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
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
		framePath = new(framePaths.Base)
	}
	musicCounts := buildMusicCounts(raw.UserMusicClear, raw.UserMusicStats)

	nickname, word := c.moderateProfileText(
		query.Region, detail.ID, raw.UserGamedata.UserID, detail.Nickname, cleanWord(raw.UserProfile.Word),
	)

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
		MultiLive:            buildMultiLive(raw.UserMultiLiveTopScoreCount),
		UpdateTime:           new(detail.UpdateTime),
		LvRankBgPath:         assets.ResolveAssetPath(c.assets, assets.StaticImagesDir, "lv_rank_bg.png"),
		XIconPath:            assets.ResolveAssetPath(c.assets, assets.StaticImagesDir, "x_icon.png"),
		IconClearPath:        assets.ResolveAssetPath(c.assets, assets.StaticImagesDir, "icon_clear.png"),
		IconFcPath:           assets.ResolveAssetPath(c.assets, assets.StaticImagesDir, "icon_fc.png"),
		IconApPath:           assets.ResolveAssetPath(c.assets, assets.StaticImagesDir, "icon_ap.png"),
		CharaRankIconPathMap: buildCharaIconMap(c.assets),
		FramePaths:           framePaths,
	}, nil
}

func (c *Controller) moderateProfileText(region, profileID string, userID int64, nickname, word string) (string, string) {
	if c.censor == nil || (nickname == "" && word == "") {
		return nickname, word
	}
	// Name and bio moderation are independent (distinct verdict caches,
	// separate upstream calls on miss), so overlap the two cache misses.
	censorCtx := c.contextOrBackground()
	var wg sync.WaitGroup
	if nickname != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if !c.censor.CensorName(censorCtx, 0, profileID, nickname, region) {
				nickname = ""
			}
		}()
	}
	if word != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if !c.censor.CensorShortBio(censorCtx, 0, strconv.FormatInt(userID, 10), word, region) {
				word = ""
			}
		}()
	}
	wg.Wait()
	return nickname, word
}

func (c *Controller) RenderProfile(query Query) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	finishBuild := commandtrace.MeasureOperation(c.contextOrBackground(), "payload.build")
	payload, err := c.BuildProfileRequest(query)
	finishBuild()
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateProfile(payload)
}

func (c *Controller) SnapshotDetailedProfile(region renderregion.Value) *drawing.DetailedProfileCardRequest {
	if c == nil || c.snapshot == nil {
		return nil
	}
	return c.snapshot.DetailedProfile(region)
}
