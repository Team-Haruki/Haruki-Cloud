package profile

import (
	"fmt"
	"strconv"
	"strings"

	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/common"
	"haruki-cloud/internal/pjsk/render/snapshot"
	"haruki-cloud/internal/pjsk/sekai"
)

func (c *Controller) BuildModularProfileRequestFromAPIWithSnapshot(query Query, resp *sekai.GetAnotherProfileResponse, snap snapshot.Snapshot) (*drawing.ModularProfileRenderRequest, error) {
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

	state := resolveProfileRenderState(resp, snap)
	leaderImagePath := c.buildLeaderImagePathFromSource(source, state.leaderCardID, state.leaderTrainedArt, region)
	framePaths, hasFrame := c.buildFramePaths(source, snapshotFrames(snap))
	var framePath *string
	if framePaths != nil {
		framePath = new(framePaths.Base)
	}

	musicCounts := buildMusicCounts(adaptAPIMusicClearCount(resp.UserMusicDifficultyClearCount), nil)
	profile := drawing.BasicProfile{
		ID:              strconv.FormatInt(resp.User.UserID, 10),
		Region:          strings.ToUpper(region.String()),
		Nickname:        resp.User.Name,
		IsHideUID:       !query.Visible,
		LeaderImagePath: leaderImagePath,
		HasFrame:        hasFrame,
		FramePath:       framePath,
	}
	bgSettings := applyProfileBGVerticalOverride(query.BgSettings, query.VerticalOverride)
	if bgSettings != nil && bgSettings.ImgPath != nil && *bgSettings.ImgPath != "" {
		path := strings.TrimSpace(*bgSettings.ImgPath)
		if !strings.HasPrefix(path, "asset/") {
			path = fmt.Sprintf("asset/%s", path)
		}
		bgSettings.ImgPath = &path
	}

	detail := &drawing.DetailedProfileCardRequest{
		ID:              profile.ID,
		Region:          profile.Region,
		Nickname:        profile.Nickname,
		Source:          "sekai_api_public",
		UpdateTime:      0,
		IsHideUID:       profile.IsHideUID,
		LeaderImagePath: profile.LeaderImagePath,
		HasFrame:        profile.HasFrame,
		FramePath:       common.CloneStringPtr(profile.FramePath),
		UserCards:       state.detailedUserCards,
	}
	inheritSnapshotProfileMetadata(detail, snap, region)

	return &drawing.ModularProfileRenderRequest{
		SchemaVersion:  1,
		RenderVersion:  drawing.ModularProfileRenderVersion,
		Kind:           drawing.ModularProfileRequestKind,
		Region:         region.String(),
		Profile:        profile,
		DataSources:    buildProfileCardDataSources(detail, snap, region),
		BgSettings:     bgSettings,
		Preset:         buildDefaultModularProfilePreset(c, source, region, resp, state, musicCounts),
		ProfileContext: drawing.NewCustomProfileContext(resp),
		Resources: drawing.CustomProfileResources{
			"chara_rank_icon_path_map": buildCharaIconMap(c.assets),
			"lv_rank_bg_path":          assets.ResolveAssetPath(c.assets, assets.StaticImagesDir, "lv_rank_bg.png"),
		},
	}, nil
}

func (c *Controller) RenderModularProfileFromAPIWithSnapshot(query Query, resp *sekai.GetAnotherProfileResponse, snap snapshot.Snapshot) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	finishBuild := commandtrace.MeasureOperation(c.contextOrBackground(), "payload.build")
	payload, err := c.BuildModularProfileRequestFromAPIWithSnapshot(query, resp, snap)
	finishBuild()
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateModularProfile(payload)
}

func buildDefaultModularProfilePreset(
	c *Controller,
	source DataSource,
	region renderregion.Value,
	resp *sekai.GetAnotherProfileResponse,
	state profileRenderState,
	musicCounts []drawing.MusicClearCount,
) drawing.ModularProfilePreset {
	characterRanks := buildCharacterRanks(adaptAPICharacters(resp.UserCharacters))
	deckCards := c.buildPCards(source, state.userCards, state.decks, state.activeDeckID, region)
	focusCharacter := firstCharacterRank(characterRanks)
	focusCard := firstDeckCard(deckCards)

	return drawing.ModularProfilePreset{
		ID:     "default_widgets_v1",
		Name:   "默认模块个人信息",
		Source: "cloud_default",
		Theme: map[string]any{
			"style":            "dark_soft_panel",
			"background_color": "#1b1d2e",
			"panel_color":      "#2b2d42",
			"panel_alpha":      0.92,
			"corner_radius":    28,
			"accent_color":     "#e0175b",
			"text_color":       "#f7f8ff",
			"muted_text_color": "#9296ad",
		},
		Grid: drawing.ModularProfileGrid{
			Columns:   4,
			RowHeight: 156,
			Gutter:    16,
			Padding:   24,
		},
		Widgets: []drawing.ModularProfileWidget{
			modularWidget("profile.summary", "profile_summary", "large", "", 0, 0, 4, 1, nil, map[string]any{
				"rank":        resp.User.Rank,
				"twitter_id":  resp.UserProfile.TwitterID,
				"total_power": resp.TotalPower,
			}),
			modularWidget("deck.current", "deck_cards", "large", "", 0, 1, 4, 1, nil, map[string]any{
				"cards": deckCards,
				"deck":  resp.UserDeck,
			}),
			modularWidget("music.clear", "fc_ap_clear", "large", "", 0, 2, 4, 1, nil, map[string]any{
				"counts": musicCounts,
			}),
			modularWidget("character.single", "single_character_rank", "medium", "", 0, 3, 2, 1, nil, map[string]any{
				"character_rank": focusCharacter,
			}),
			modularWidget("card.single", "single_card", "medium", "", 2, 3, 2, 1, nil, map[string]any{
				"card": focusCard,
			}),
			modularWidget("event.settlement", "event_rank_pt", "large", "", 0, 4, 4, 1, map[string]any{
				"state": "placeholder",
			}, map[string]any{
				"rank": nil,
				"pt":   nil,
			}),
			modularWidget("characters.full_radar", "character_rank_full_radar", "large", "", 0, 5, 4, 4, nil, map[string]any{
				"character_rank": characterRanks,
			}),
		},
	}
}

func modularWidget(id, widgetType, family, title string, x, y, w, h int, options, data map[string]any) drawing.ModularProfileWidget {
	return drawing.ModularProfileWidget{
		ID:     id,
		Type:   widgetType,
		Family: family,
		Title:  common.OptionalString(title),
		Frame: drawing.ModularProfileWidgetFrame{
			X: x,
			Y: y,
			W: w,
			H: h,
		},
		Options: options,
		Data:    data,
	}
}

func firstCharacterRank(ranks []drawing.CharacterRank) *drawing.CharacterRank {
	for i := range ranks {
		if ranks[i].CharacterID > 0 {
			return &ranks[i]
		}
	}
	return nil
}

func firstDeckCard(cards []drawing.CardFullThumbnailRequest) *drawing.CardFullThumbnailRequest {
	for i := range cards {
		if cards[i].CardID > 0 {
			return &cards[i]
		}
	}
	return nil
}
