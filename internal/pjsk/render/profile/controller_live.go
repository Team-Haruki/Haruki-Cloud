package profile

import (
	"fmt"
	"strconv"
	"strings"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/common"
	"haruki-cloud/internal/pjsk/render/snapshot"
	"haruki-cloud/internal/pjsk/sekai"
)

// BuildProfileRequestFromAPI builds a ProfileRequest from a live GetUserProfile API response.
// framesJSON is the optional raw bytes from a userPlayerFrames snapshot payload; pass nil to
// render without a player frame.
// query.Visible maps directly to !IsHideUID (false = hide UID, true = show UID).
// UpdateTime is always nil so that the image cache system produces a stable cache key for
// identical renders.
// UserEventResults are intentionally ignored here; profile honors are rendered from
// the public profile payload plus aggregate clear-count data.
func (c *Controller) BuildProfileRequestFromAPI(query Query, resp *sekai.GetAnotherProfileResponse, framesJSON []byte) (*drawing.ProfileRequest, error) {
	return c.buildProfileRequestFromAPIState(query, resp, parseFramesJSON(framesJSON), nil)
}

func (c *Controller) BuildProfileRequestFromAPIWithSnapshot(query Query, resp *sekai.GetAnotherProfileResponse, snapshot snapshot.Snapshot) (*drawing.ProfileRequest, error) {
	return c.buildProfileRequestFromAPIState(query, resp, snapshotFrames(snapshot), snapshot)
}

func (c *Controller) buildProfileRequestFromAPIState(query Query, resp *sekai.GetAnotherProfileResponse, frames []snapshot.RawUserFrame, snapshot snapshot.Snapshot) (*drawing.ProfileRequest, error) {
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

	state := resolveProfileRenderState(resp, snapshot)
	leaderImagePath := c.buildLeaderImagePathFromSource(source, state.leaderCardID, state.leaderTrainedArt, region)

	framePaths, hasFrame := c.buildFramePaths(source, frames)
	var framePath *string
	if framePaths != nil {
		framePath = new(framePaths.Base)
	}

	musicCounts := buildMusicCounts(adaptAPIMusicClearCount(resp.UserMusicDifficultyClearCount), nil)

	// 拼接drawing的背景路径
	if query.BgSettings != nil && query.BgSettings.ImgPath != nil && *query.BgSettings.ImgPath != "" {
		*query.BgSettings.ImgPath = fmt.Sprintf("asset/%s", *query.BgSettings.ImgPath)
	}
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
		Pcards:               c.buildPCards(source, state.userCards, state.decks, state.activeDeckID, region),
		BgSettings:           applyProfileBGVerticalOverride(query.BgSettings, query.VerticalOverride),
		Honors:               c.buildHonors(source, region, adaptAPIProfileHonors(resp.UserProfileHonors), adaptAPIUserHonors(resp.UserHonors), musicCounts),
		MusicDifficultyCount: musicCounts,
		CharacterRank:        buildCharacterRanks(adaptAPICharacters(resp.UserCharacters)),
		SoloLive:             buildSoloLive(adaptAPIChallengeLiveResult(resp.UserChallengeLiveSoloResult), adaptAPIChallengeLiveStages(resp.UserChallengeLiveSoloStages)),
		UpdateTime:           nil,
		LvRankBgPath:         assets.ResolveAssetPath(c.assets, assets.StaticImagesDir, "lv_rank_bg.png"),
		XIconPath:            assets.ResolveAssetPath(c.assets, assets.StaticImagesDir, "x_icon.png"),
		IconClearPath:        assets.ResolveAssetPath(c.assets, assets.StaticImagesDir, "icon_clear.png"),
		IconFcPath:           assets.ResolveAssetPath(c.assets, assets.StaticImagesDir, "icon_fc.png"),
		IconApPath:           assets.ResolveAssetPath(c.assets, assets.StaticImagesDir, "icon_ap.png"),
		CharaRankIconPathMap: buildCharaIconMap(c.assets),
		FramePaths:           framePaths,
	}, nil
}

func (c *Controller) BuildDetailedProfileCardFromAPI(query Query, resp *sekai.GetAnotherProfileResponse, framesJSON []byte) (*drawing.DetailedProfileCardRequest, error) {
	return c.buildDetailedProfileCardFromAPIState(query, resp, parseFramesJSON(framesJSON), nil)
}

func (c *Controller) BuildDetailedProfileCardFromAPIWithSnapshot(query Query, resp *sekai.GetAnotherProfileResponse, snapshot snapshot.Snapshot) (*drawing.DetailedProfileCardRequest, error) {
	return c.buildDetailedProfileCardFromAPIState(query, resp, snapshotFrames(snapshot), snapshot)
}

func (c *Controller) buildDetailedProfileCardFromAPIState(query Query, resp *sekai.GetAnotherProfileResponse, frames []snapshot.RawUserFrame, snapshot snapshot.Snapshot) (*drawing.DetailedProfileCardRequest, error) {
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

	state := resolveProfileRenderState(resp, snapshot)
	leaderImagePath := c.buildLeaderImagePathFromSource(source, state.leaderCardID, state.leaderTrainedArt, region)

	framePaths, hasFrame := c.buildFramePaths(source, frames)
	var framePath *string
	if framePaths != nil {
		framePath = new(framePaths.Base)
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
		UserCards:       state.detailedUserCards,
	}, nil
}

func (c *Controller) BuildProfileCardFromAPI(query Query, resp *sekai.GetAnotherProfileResponse, framesJSON []byte) (*drawing.ProfileCardRequest, error) {
	detail, err := c.buildDetailedProfileCardFromAPIState(query, resp, parseFramesJSON(framesJSON), nil)
	if err != nil {
		return nil, err
	}
	return &drawing.ProfileCardRequest{
		Profile: &drawing.BasicProfile{
			ID:              detail.ID,
			Region:          detail.Region,
			Nickname:        detail.Nickname,
			IsHideUID:       detail.IsHideUID,
			LeaderImagePath: detail.LeaderImagePath,
			HasFrame:        detail.HasFrame,
			FramePath:       common.CloneStringPtr(detail.FramePath),
		},
		DataSources: []drawing.ProfileDataSource{
			{
				Name:   "Sekai API",
				Source: new(detail.Source),
				Mode:   common.CloneStringPtr(detail.Mode),
			},
		},
	}, nil
}

func (c *Controller) BuildProfileCardFromAPIWithSnapshot(query Query, resp *sekai.GetAnotherProfileResponse, snapshot snapshot.Snapshot) (*drawing.ProfileCardRequest, error) {
	detail, err := c.buildDetailedProfileCardFromAPIState(query, resp, snapshotFrames(snapshot), snapshot)
	if err != nil {
		return nil, err
	}
	return &drawing.ProfileCardRequest{
		Profile: &drawing.BasicProfile{
			ID:              detail.ID,
			Region:          detail.Region,
			Nickname:        detail.Nickname,
			IsHideUID:       detail.IsHideUID,
			LeaderImagePath: detail.LeaderImagePath,
			HasFrame:        detail.HasFrame,
			FramePath:       common.CloneStringPtr(detail.FramePath),
		},
		DataSources: []drawing.ProfileDataSource{
			{
				Name:   "Sekai API",
				Source: new(detail.Source),
				Mode:   common.CloneStringPtr(detail.Mode),
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
	logProfilePayloadDebug("sekai_api_public", payload)
	return c.drawing.GenerateProfile(payload)
}

func (c *Controller) RenderProfileFromAPIWithSnapshot(query Query, resp *sekai.GetAnotherProfileResponse, snapshot snapshot.Snapshot) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildProfileRequestFromAPIWithSnapshot(query, resp, snapshot)
	if err != nil {
		return nil, err
	}
	logProfilePayloadDebug("sekai_api_public", payload)
	return c.drawing.GenerateProfile(payload)
}
