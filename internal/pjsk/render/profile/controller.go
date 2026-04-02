package profile

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/common"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	regionsource "haruki-cloud/internal/pjsk/render/source"
	"haruki-cloud/internal/pjsk/render/userdata"
	"haruki-cloud/utils/censor"
	"haruki-cloud/utils/drawing"
	sekai "haruki-cloud/utils/sekai"
)

var wordTagPattern = regexp.MustCompile(`<#.*?>`)

type Controller struct {
	sources  *regionsource.Registry[DataSource]
	drawing  *drawing.HarukiDrawingClient
	assets   *assets.AssetHelper
	snapshot *userdata.Service
	censor   *censor.Service
}

func NewController(defaultSource DataSource, drawingClient *drawing.HarukiDrawingClient, assetHelper *assets.AssetHelper, snapshot *userdata.Service) *Controller {
	if assetHelper == nil {
		assetHelper = assets.NewAssetHelper("", nil)
	}
	ctrl := &Controller{
		sources:  regionsource.NewRegistry[DataSource](renderregion.JP),
		drawing:  drawingClient,
		assets:   assetHelper,
		snapshot: snapshot,
	}
	ctrl.RegisterSource(defaultSource)
	return ctrl
}

func (c *Controller) RegisterSource(source DataSource) {
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
		Honors:               c.buildHonors(source, region, raw.UserProfileHonors, raw.UserHonors),
		MusicDifficultyCount: buildMusicCounts(raw.UserMusicClear, raw.UserMusicStats),
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

	profileCardID := userdata.SelectProfileImageCardID(resp.UserProfile.ProfileImageType, resp.UserProfile.ProfileImageID, resp.UserDeck.Leader)
	profileCard := findAPIUserCard(resp.UserCards, profileCardID)
	leaderCard := findAPIUserCard(resp.UserCards, resp.UserDeck.Leader)
	leaderImagePath := buildProfileImagePathFromSource(
		source,
		c.assets,
		profileCardID,
		isAPICardAfterTraining(profileCard),
		resp.UserDeck.Leader,
		isAPICardAfterTraining(leaderCard),
		region,
	)

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
		Honors:               c.buildHonors(source, region, adaptAPIProfileHonors(resp.UserProfileHonors), adaptAPIUserHonors(resp.UserHonors)),
		MusicDifficultyCount: buildMusicCounts(adaptAPIMusicClearCount(resp.UserMusicDifficultyClearCount), nil),
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

	profileCardID := userdata.SelectProfileImageCardID(resp.UserProfile.ProfileImageType, resp.UserProfile.ProfileImageID, resp.UserDeck.Leader)
	profileCard := findAPIUserCard(resp.UserCards, profileCardID)
	leaderCard := findAPIUserCard(resp.UserCards, resp.UserDeck.Leader)
	leaderImagePath := buildProfileImagePathFromSource(
		source,
		c.assets,
		profileCardID,
		isAPICardAfterTraining(profileCard),
		resp.UserDeck.Leader,
		isAPICardAfterTraining(leaderCard),
		region,
	)

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
		UserCards:       buildAPIUserCardEntries(resp.UserCards, resp.UserDeck),
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
			FramePath:       common.CloneStringPtr(detail.FramePath),
		},
		DataSources: []drawing.ProfileDataSource{
			{
				Name:   "Sekai API",
				Source: &source,
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

