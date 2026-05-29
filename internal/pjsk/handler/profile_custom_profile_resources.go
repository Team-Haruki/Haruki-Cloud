package handler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	json "github.com/bytedance/sonic"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/common"
	renderhonor "haruki-cloud/internal/pjsk/render/honor"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/provider"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
)

func buildCustomProfileResources(ctx context.Context, app *renderapp.App, region string, card sekaiapi.UserCustomProfileCard, resp *sekaiapi.GetAnotherProfileResponse) (drawing.CustomProfileResources, error) {
	resources := drawing.CustomProfileResources{}
	regionValue := renderregion.WithDefault(renderregion.Normalize(region))
	resources["charaRankIconPathMap"] = customProfileCharaRankIconPathMap(app)
	collector := newCustomProfileResourceCollector(card, resp)

	if err := collectCustomProfileMasterResources(app, regionValue, collector, resources); err != nil {
		return nil, err
	}
	if err := collectCustomProfileStampResources(ctx, app, regionValue, collector, resources); err != nil {
		return nil, err
	}
	if err := collectCustomProfileCardResources(ctx, app, regionValue, collector, resources); err != nil {
		return nil, err
	}
	if err := collectCustomProfileStoryFavoriteResources(app, regionValue, collector, resources); err != nil {
		return nil, err
	}
	if err := collectCustomProfileHonorResources(ctx, app, regionValue, collector, resources); err != nil {
		return nil, err
	}

	return resources, nil
}

func customProfileCharaRankIconPathMap(app *renderapp.App) map[string]string {
	helper := (*assets.AssetHelper)(nil)
	if app != nil {
		helper = app.Assets
	}
	result := make(map[string]string, len(assets.CharacterIDToNickname))
	for id, nickname := range assets.CharacterIDToNickname {
		result[strconv.Itoa(id)] = assets.ResolveAssetPath(helper, assets.StaticImagesDir, filepath.Join("chara_icon", nickname+".png"))
	}
	return result
}

type customProfileResourceCollector struct {
	textColorIDs      map[int]struct{}
	textFontIDs       map[int]struct{}
	shapeIDs          map[int]struct{}
	playerInfoIDs     map[int]struct{}
	generalBgIDs      map[int]struct{}
	storyBgIDs        map[int]struct{}
	standMemberIDs    map[int]struct{}
	collectionIDs     map[int]struct{}
	collectionTargets map[int]map[int]struct{}
	otherIDs          map[int]struct{}
	stampIDs          map[int]struct{}
	cardIDs           map[int]struct{}
	honorQueries      map[string]renderhonor.Query
	profileHonors     map[string]renderhonor.Query
	bondsHonorQueries map[string]renderhonor.Query
	storyFavorites    []sekaiapi.UserStoryFavorite
}

func newCustomProfileResourceCollector(card sekaiapi.UserCustomProfileCard, resp *sekaiapi.GetAnotherProfileResponse) customProfileResourceCollector {
	c := customProfileResourceCollector{
		textColorIDs:      make(map[int]struct{}),
		textFontIDs:       make(map[int]struct{}),
		shapeIDs:          make(map[int]struct{}),
		playerInfoIDs:     make(map[int]struct{}),
		generalBgIDs:      make(map[int]struct{}),
		storyBgIDs:        make(map[int]struct{}),
		standMemberIDs:    make(map[int]struct{}),
		collectionIDs:     make(map[int]struct{}),
		collectionTargets: make(map[int]map[int]struct{}),
		otherIDs:          make(map[int]struct{}),
		stampIDs:          make(map[int]struct{}),
		cardIDs:           make(map[int]struct{}),
		honorQueries:      make(map[string]renderhonor.Query),
		profileHonors:     make(map[string]renderhonor.Query),
		bondsHonorQueries: make(map[string]renderhonor.Query),
	}

	data := card.CustomProfileCard
	fcApLevels := customProfileHonorFcApLevels(resp)
	for _, item := range data.Generals {
		addID(c.playerInfoIDs, item.PlayerInfoResourceID)
	}
	for _, item := range data.GeneralBackgrounds {
		addID(c.generalBgIDs, item.ID)
	}
	for _, item := range data.StoryBackgrounds {
		addID(c.storyBgIDs, item.ID)
	}
	for _, item := range data.StandMembers {
		addID(c.standMemberIDs, item.ID)
	}
	for _, item := range data.Collections {
		addID(c.collectionIDs, item.ID)
		addNestedID(c.collectionTargets, item.ID, item.TargetID)
	}
	for _, item := range data.Others {
		addID(c.otherIDs, item.ID)
	}
	for _, item := range data.Shapes {
		addID(c.shapeIDs, item.ID)
		addID(c.textColorIDs, item.ColorID)
		addID(c.textColorIDs, item.OutlineColorID)
	}
	for _, item := range data.Texts {
		addID(c.textColorIDs, item.ColorID)
		addID(c.textColorIDs, item.OutlineColorID)
		addID(c.textFontIDs, item.FontID)
	}
	for _, item := range data.Stamps {
		addID(c.stampIDs, item.ID)
	}
	for _, item := range data.CardMembers {
		addID(c.cardIDs, item.ID)
	}

	if resp != nil {
		deck := resp.UserDeck
		for _, cardID := range []int{deck.Leader, deck.Member1, deck.Member2, deck.Member3, deck.Member4, deck.Member5} {
			addID(c.cardIDs, cardID)
		}
		for _, row := range resp.UserProfileHonors {
			query := renderhonor.Query{
				HonorID:             row.HonorID,
				HonorLevel:          row.HonorLevel,
				IsMain:              row.Seq == 1,
				BondsHonorViewType:  row.BondsHonorViewType,
				BondsHonorWordID:    row.BondsHonorWordID,
				FcOrApLevelOverride: fcApLevels[row.HonorID],
			}
			if query.HonorID <= 0 {
				continue
			}
			c.profileHonors[fmt.Sprintf("profile:%d", row.Seq)] = query
			c.profileHonors[fmt.Sprintf("profile:%d:%d", row.HonorID, row.Seq)] = query
			c.profileHonors[customProfileHonorRequestKey(row.HonorID, row.HonorLevel, query.IsMain)] = query
		}
	}

	for _, item := range data.Honors {
		level := customProfileUserHonorLevel(resp, item.ID)
		query := renderhonor.Query{
			HonorID:             item.ID,
			HonorLevel:          level,
			IsMain:              item.FullSize,
			FcOrApLevelOverride: fcApLevels[item.ID],
		}
		if query.HonorID > 0 {
			c.honorQueries[customProfileHonorRequestKey(item.ID, level, item.FullSize)] = query
		}
	}
	for _, item := range data.BondsHonors {
		level := customProfileUserBondsHonorLevel(resp, item.ID)
		query := renderhonor.Query{
			HonorID:              item.ID,
			HonorLevel:           level,
			IsMain:               item.FullSize,
			BondsHonorViewType:   customProfileBondsViewType(item.Inverse),
			BondsHonorWordID:     item.WordID,
			UseUnitVirtualSinger: item.UseUnitVirtualSinger,
		}
		if query.HonorID > 0 {
			c.bondsHonorQueries[customProfileBondsHonorRequestKey(item.ID, level, item.FullSize, item.WordID, item.Inverse, item.UseUnitVirtualSinger)] = query
		}
	}
	if _, ok := c.playerInfoIDs[14]; ok && resp != nil {
		c.storyFavorites = append(c.storyFavorites, resp.UserStoryFavorites...)
	}

	return c
}

func collectCustomProfileMasterResources(app *renderapp.App, region renderregion.Value, c customProfileResourceCollector, resources drawing.CustomProfileResources) error {
	tables := []struct {
		key         string
		filename    string
		ids         map[int]struct{}
		fallbackDir string
		withPath    bool
	}{
		{"customProfileTextColors", "customProfileTextColors.json", c.textColorIDs, "", false},
		{"customProfileTextFonts", "customProfileTextFonts.json", c.textFontIDs, "", false},
		{"customProfileShapeResources", "customProfileShapeResources.json", c.shapeIDs, "shape", true},
		{"customProfilePlayerInfoResources", "customProfilePlayerInfoResources.json", c.playerInfoIDs, "", true},
		{"customProfileGeneralBackgroundResources", "customProfileGeneralBackgroundResources.json", c.generalBgIDs, "", true},
		{"customProfileStoryBackgroundResources", "customProfileStoryBackgroundResources.json", c.storyBgIDs, "", true},
		{"customProfileMemberStandingPictureResources", "customProfileMemberStandingPictureResources.json", c.standMemberIDs, "", true},
		{"customProfileCollectionResources", "customProfileCollectionResources.json", c.collectionIDs, "", true},
		{"customProfileEtcResources", "customProfileEtcResources.json", c.otherIDs, "", true},
	}

	for _, table := range tables {
		if len(table.ids) == 0 {
			continue
		}
		items, err := loadCustomProfileMasterTable(app, region, table.filename, table.ids)
		if err != nil {
			return err
		}
		if table.withPath {
			for _, item := range items {
				if imagePath := resolveCustomProfileResourceImagePath(app, region, item, table.fallbackDir); imagePath != "" {
					item["imagePath"] = imagePath
				}
			}
		}
		resources[table.key] = items
		if table.key == "customProfileCollectionResources" {
			if err := collectCustomProfileOmikujiResources(app, region, c, items, resources); err != nil {
				return err
			}
		}
	}
	return nil
}

func collectCustomProfileOmikujiResources(app *renderapp.App, region renderregion.Value, c customProfileResourceCollector, collectionResources map[int]map[string]any, resources drawing.CustomProfileResources) error {
	ids := make(map[int]struct{})
	for resourceID, resource := range collectionResources {
		if !strings.EqualFold(strings.TrimSpace(mapString(resource, "customProfileResourceCollectionType")), "omikuji") {
			continue
		}
		for targetID := range c.collectionTargets[resourceID] {
			addID(ids, targetID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	omikujis, err := loadCustomProfileMasterTable(app, region, "omikujis.json", ids)
	if err != nil {
		return err
	}
	resources["omikujis"] = omikujis
	return nil
}

func collectCustomProfileStampResources(ctx context.Context, app *renderapp.App, region renderregion.Value, c customProfileResourceCollector, resources drawing.CustomProfileResources) error {
	if len(c.stampIDs) == 0 {
		return nil
	}
	src := customProfileProviderForRegion(app, region)
	if src == nil || src.Stamps() == nil {
		return fmt.Errorf("stamp masterdata provider is not configured")
	}
	stamps, err := src.Stamps().GetAll(ctx)
	if err != nil {
		return err
	}
	masters := make(map[int]map[string]any, len(c.stampIDs))
	assetsByID := make(map[int]map[string]any, len(c.stampIDs))
	for _, stamp := range stamps {
		if _, ok := c.stampIDs[stamp.ID]; !ok {
			continue
		}
		masters[stamp.ID] = map[string]any{
			"id":              stamp.ID,
			"assetbundleName": stamp.AssetBundleName,
			"characterId":     stamp.CharacterID,
		}
		assetsByID[stamp.ID] = map[string]any{
			"id":              stamp.ID,
			"assetbundleName": stamp.AssetBundleName,
			"imagePath":       resolveCustomProfileStampImagePath(app, region, stamp),
		}
	}
	resources["stamps"] = masters
	resources["stampAssets"] = assetsByID
	return nil
}

func collectCustomProfileCardResources(ctx context.Context, app *renderapp.App, region renderregion.Value, c customProfileResourceCollector, resources drawing.CustomProfileResources) error {
	if len(c.cardIDs) == 0 {
		return nil
	}
	src := customProfileProviderForRegion(app, region)
	if src == nil || src.Cards() == nil {
		return fmt.Errorf("card masterdata provider is not configured")
	}
	cards := make(map[int]map[string]any, len(c.cardIDs))
	cardAssets := make(map[int]map[string]any, len(c.cardIDs))
	for cardID := range c.cardIDs {
		cardInfo, err := src.Cards().GetByID(ctx, cardID)
		if err != nil || cardInfo == nil {
			continue
		}
		cards[cardInfo.ID] = customProfileCardMasterMap(cardInfo)
		cardAssets[cardInfo.ID] = customProfileCardAssetMap(app, region, cardInfo)
	}
	resources["cards"] = cards
	resources["cardAssets"] = cardAssets
	return nil
}

func collectCustomProfileStoryFavoriteResources(app *renderapp.App, region renderregion.Value, c customProfileResourceCollector, resources drawing.CustomProfileResources) error {
	if len(c.storyFavorites) == 0 {
		return nil
	}
	eventStoryIDs := make(map[int]struct{})
	for _, story := range c.storyFavorites {
		if story.StoryID > 0 && strings.EqualFold(strings.TrimSpace(story.StoryType), "event_story") {
			eventStoryIDs[story.StoryID] = struct{}{}
		}
	}
	if len(eventStoryIDs) == 0 {
		return nil
	}
	eventStories, err := loadCustomProfileMasterTable(app, region, "eventStories.json", eventStoryIDs)
	if err != nil {
		return err
	}
	eventIDs := make(map[int]struct{})
	for _, row := range eventStories {
		if eventID, ok := mapInt(row, "eventId"); ok {
			eventIDs[eventID] = struct{}{}
		}
	}
	events := map[int]map[string]any{}
	if len(eventIDs) > 0 {
		if loaded, err := loadCustomProfileMasterTable(app, region, "events.json", eventIDs); err == nil {
			events = loaded
		}
	}

	items := make(map[string]any, len(eventStories))
	for _, story := range c.storyFavorites {
		key := customProfileStoryFavoriteKey(story.StoryType, story.StoryID)
		item := map[string]any{
			"storyType": story.StoryType,
			"storyId":   story.StoryID,
		}
		row := eventStories[story.StoryID]
		if row != nil {
			assetBundleName := strings.TrimSpace(mapString(row, "assetbundleName"))
			item["assetbundleName"] = assetBundleName
			if imagePath := resolveCustomProfileEventStoryBannerPath(app, region, assetBundleName); imagePath != "" {
				item["imagePath"] = imagePath
			}
			if eventID, ok := mapInt(row, "eventId"); ok {
				item["eventId"] = eventID
				if eventRow := events[eventID]; eventRow != nil {
					if title := strings.TrimSpace(mapString(eventRow, "name")); title != "" {
						item["title"] = title
					}
				}
			}
		}
		items[key] = item
	}
	resources["storyFavoriteResources"] = items
	return nil
}

func collectCustomProfileHonorResources(ctx context.Context, app *renderapp.App, region renderregion.Value, c customProfileResourceCollector, resources drawing.CustomProfileResources) error {
	if app == nil || app.Honors == nil {
		if len(c.honorQueries) > 0 || len(c.profileHonors) > 0 || len(c.bondsHonorQueries) > 0 {
			return fmt.Errorf("honor controller is not configured")
		}
		return nil
	}
	ctrl := app.Honors.WithContext(ctx)
	build := func(queries map[string]renderhonor.Query) (map[string]any, error) {
		out := make(map[string]any, len(queries))
		for key, query := range queries {
			query.Region = region
			req, err := ctrl.BuildHonorRequest(query)
			if err != nil {
				return nil, err
			}
			out[key] = req
		}
		return out, nil
	}
	if len(c.honorQueries) > 0 {
		items, err := build(c.honorQueries)
		if err != nil {
			return err
		}
		resources["honorRequests"] = items
	}
	if len(c.profileHonors) > 0 {
		items, err := build(c.profileHonors)
		if err != nil {
			return err
		}
		resources["profileHonorRequests"] = items
	}
	if len(c.bondsHonorQueries) > 0 {
		items, err := build(c.bondsHonorQueries)
		if err != nil {
			return err
		}
		resources["bondsHonorRequests"] = items
	}
	return nil
}

func loadCustomProfileMasterTable(app *renderapp.App, region renderregion.Value, filename string, ids map[int]struct{}) (map[int]map[string]any, error) {
	if len(ids) == 0 {
		return map[int]map[string]any{}, nil
	}
	var data []byte
	var readErr error
	for _, dir := range customProfileMasterdataDirs(app, region) {
		var err error
		data, err = os.ReadFile(filepath.Join(dir, filename))
		if err == nil {
			break
		}
		if readErr == nil {
			readErr = err
		}
	}
	if len(data) == 0 {
		if readErr == nil {
			readErr = os.ErrNotExist
		}
		return nil, fmt.Errorf("read custom profile masterdata %s: %w", filename, readErr)
	}
	var rows []map[string]any
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, fmt.Errorf("unmarshal custom profile masterdata %s: %w", filename, err)
	}
	result := make(map[int]map[string]any, len(ids))
	for _, row := range rows {
		id, ok := mapInt(row, "id")
		if !ok {
			continue
		}
		if _, wanted := ids[id]; wanted {
			result[id] = row
		}
	}
	return result, nil
}

func customProfileMasterdataDirs(app *renderapp.App, region renderregion.Value) []string {
	if app == nil {
		return nil
	}
	roots := []string{
		app.Config.LocalMasterdata.Dir,
		app.Config.DeckRecommend.MasterdataDir,
	}
	result := make([]string, 0, 12)
	seen := make(map[string]struct{}, 12)
	add := func(dir string) {
		dir = strings.TrimSpace(dir)
		if dir == "" || dir == "." || dir == "/" {
			return
		}
		dir = filepath.Clean(dir)
		if _, ok := seen[dir]; ok {
			return
		}
		seen[dir] = struct{}{}
		result = append(result, dir)
	}
	repoDir := customProfileMasterdataRepoDir(region)
	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		add(filepath.Join(root, region.String()))
		if repoDir != "" {
			add(filepath.Join(root, repoDir, "master"))
		}
		add(root)
		current := filepath.Clean(root)
		for depth := 0; depth < 4; depth++ {
			parent := filepath.Dir(current)
			if parent == "" || parent == "." || parent == "/" || parent == current {
				break
			}
			current = parent
			add(filepath.Join(current, region.String()))
			if repoDir != "" {
				add(filepath.Join(current, repoDir, "master"))
			}
		}
	}
	return result
}

func customProfileMasterdataRepoDir(region renderregion.Value) string {
	switch renderregion.WithDefault(region) {
	case renderregion.JP:
		return "haruki-sekai-master"
	case renderregion.CN:
		return "haruki-sekai-sc-master"
	case renderregion.TW:
		return "haruki-sekai-tc-master"
	case renderregion.KR:
		return "haruki-sekai-kr-master"
	case renderregion.EN:
		return "haruki-sekai-en-master"
	default:
		return ""
	}
}

func customProfileProviderForRegion(app *renderapp.App, region renderregion.Value) provider.MasterDataProvider {
	if app == nil {
		return nil
	}
	if app.Providers != nil {
		if src := app.Providers[renderregion.WithDefault(region)]; src != nil {
			return src
		}
	}
	return app.Provider
}

func customProfileCardMasterMap(card *masterdata.Card) map[string]any {
	return map[string]any{
		"id":                           card.ID,
		"characterId":                  card.CharacterID,
		"cardRarityType":               card.CardRarityType,
		"attr":                         card.Attr,
		"prefix":                       card.Prefix,
		"assetbundleName":              card.AssetBundleName,
		"releaseAt":                    card.ReleaseAt,
		"initialSpecialTrainingStatus": card.InitialSpecialTrainingStatus,
	}
}

func customProfileCardAssetMap(app *renderapp.App, region renderregion.Value, card *masterdata.Card) map[string]any {
	item := customProfileCardMasterMap(card)
	helper := (*assets.AssetHelper)(nil)
	if app != nil {
		helper = app.Assets
	}
	item["normalPath"] = common.ResolveCardMemberImagePath(helper, region, card.AssetBundleName, "card_normal.png")
	item["afterTrainingPath"] = common.ResolveCardMemberImagePath(helper, region, card.AssetBundleName, "card_after_training.png")
	item["deckNormalPath"] = resolveCustomProfileDeckCardImagePath(app, region, card.AssetBundleName, false)
	item["deckAfterTrainingPath"] = resolveCustomProfileDeckCardImagePath(app, region, card.AssetBundleName, true)
	item["clipNormalPath"] = resolveCustomProfileClipCardImagePath(app, region, card.AssetBundleName, false)
	item["clipAfterTrainingPath"] = resolveCustomProfileClipCardImagePath(app, region, card.AssetBundleName, true)
	item["smallNormalPath"] = resolveCustomProfileSmallCardImagePath(app, region, card.AssetBundleName, false)
	item["smallAfterTrainingPath"] = resolveCustomProfileSmallCardImagePath(app, region, card.AssetBundleName, true)
	return item
}

func resolveCustomProfileSmallCardImagePath(app *renderapp.App, region renderregion.Value, bundle string, afterTraining bool) string {
	fileName := "card_normal.png"
	if afterTraining {
		fileName = "card_after_training.png"
	}
	helper := (*assets.AssetHelper)(nil)
	if app != nil {
		helper = app.Assets
	}
	return assets.ResolveRegionAssetPath(helper, region.String(),
		filepath.Join("character", "member_small", bundle, fileName),
		filepath.Join("character", "member_small", bundle+"_rip", fileName),
	)
}

func resolveCustomProfileDeckCardImagePath(app *renderapp.App, region renderregion.Value, bundle string, afterTraining bool) string {
	cutoutFile := "normal.png"
	cutoutTrimFile := "card_normal_trim.png"
	if afterTraining {
		cutoutFile = "after_training.png"
		cutoutTrimFile = "card_after_training_trim.png"
	}
	helper := (*assets.AssetHelper)(nil)
	if app != nil {
		helper = app.Assets
	}
	return assets.ResolveRegionAssetPath(helper, region.String(),
		filepath.Join("character", "member_cutout", bundle, cutoutFile),
		filepath.Join("character", "member_cutout", bundle, cutoutTrimFile),
		filepath.Join("character", "member_cutout", bundle+"_rip", cutoutFile),
		filepath.Join("character", "member_cutout", bundle+"_rip", cutoutTrimFile),
		filepath.Join("character", "member_cutout", bundle, "deck.png"),
		filepath.Join("character", "member_cutout", bundle+"_rip", "deck.png"),
	)
}

func resolveCustomProfileClipCardImagePath(app *renderapp.App, region renderregion.Value, bundle string, afterTraining bool) string {
	cutoutFile := "normal.png"
	cutoutTrimFile := "card_normal_trim.png"
	if afterTraining {
		cutoutFile = "after_training.png"
		cutoutTrimFile = "card_after_training_trim.png"
	}
	helper := (*assets.AssetHelper)(nil)
	if app != nil {
		helper = app.Assets
	}
	return assets.ResolveRegionAssetPath(helper, region.String(),
		filepath.Join("character", "member_cutout_trm", bundle, cutoutFile),
		filepath.Join("character", "member_cutout_trm", bundle, cutoutTrimFile),
		filepath.Join("character", "member_cutout_trm", bundle+"_rip", cutoutFile),
		filepath.Join("character", "member_cutout_trm", bundle+"_rip", cutoutTrimFile),
	)
}

func resolveCustomProfileStampImagePath(app *renderapp.App, region renderregion.Value, stamp masterdata.Stamp) string {
	helper := (*assets.AssetHelper)(nil)
	if app != nil {
		helper = app.Assets
	}
	return assets.ResolveRegionAssetPath(helper, region.String(), filepath.Join("stamp", stamp.AssetBundleName, stamp.AssetBundleName+".png"))
}

func resolveCustomProfileEventStoryBannerPath(app *renderapp.App, region renderregion.Value, assetBundleName string) string {
	assetBundleName = strings.TrimSpace(assetBundleName)
	if assetBundleName == "" {
		return ""
	}
	helper := (*assets.AssetHelper)(nil)
	if app != nil {
		helper = app.Assets
	}
	return assets.ResolveRegionAssetPath(helper, region.String(),
		filepath.Join("event_story", assetBundleName, "screen_image", "banner_event_story.png"),
	)
}

func resolveCustomProfileResourceImagePath(app *renderapp.App, region renderregion.Value, resource map[string]any, fallbackDir string) string {
	fileName := strings.Trim(mapString(resource, "fileName"), "/")
	if fileName == "" {
		return ""
	}
	if !strings.HasSuffix(strings.ToLower(fileName), ".png") {
		fileName += ".png"
	}
	loadVal := strings.Trim(mapString(resource, "resourceLoadVal"), "/")
	rels := make([]string, 0, 3)
	switch {
	case strings.HasPrefix(loadVal, "custom_profile/"):
		rels = append(rels, filepath.Join(loadVal, fileName))
	case loadVal == "custom_profile":
		rels = append(rels, filepath.Join("custom_profile", fileName))
	case loadVal != "":
		rels = append(rels, filepath.Join("custom_profile", loadVal, fileName), filepath.Join(loadVal, fileName))
	case fallbackDir != "":
		rels = append(rels, filepath.Join("custom_profile", fallbackDir, fileName))
	default:
		rels = append(rels, filepath.Join("custom_profile", fileName))
	}
	helper := (*assets.AssetHelper)(nil)
	if app != nil {
		helper = app.Assets
	}
	return assets.ResolveRegionAssetPath(helper, region.String(), rels...)
}

func customProfileUserHonorLevel(resp *sekaiapi.GetAnotherProfileResponse, honorID int) int {
	if resp == nil {
		return 0
	}
	for _, row := range resp.UserHonors {
		if row.HonorID == honorID {
			return row.Level
		}
	}
	for _, row := range resp.UserProfileHonors {
		if row.HonorID == honorID {
			return row.HonorLevel
		}
	}
	return 0
}

func customProfileHonorFcApLevels(resp *sekaiapi.GetAnotherProfileResponse) map[int]*int {
	if resp == nil || len(resp.UserMusicDifficultyClearCount) == 0 {
		return nil
	}
	counts := make(map[string]sekaiapi.AnotherUserMusicDifficultyClearCount, len(resp.UserMusicDifficultyClearCount))
	for _, count := range resp.UserMusicDifficultyClearCount {
		difficulty := strings.ToLower(strings.TrimSpace(string(count.MusicDifficultyType)))
		if difficulty != "" {
			counts[difficulty] = count
		}
	}
	result := make(map[int]*int)
	for _, honorID := range []int{3009, 3010, 3011, 3012, 3013, 3014, 4700, 4701} {
		difficulty, score, ok := renderhonor.LookupFcApCounter(honorID)
		if !ok {
			continue
		}
		count, ok := counts[strings.ToLower(strings.TrimSpace(difficulty))]
		if !ok {
			continue
		}
		value := 0
		switch score {
		case "fullCombo":
			value = count.FullCombo
		case "allPerfect":
			value = count.AllPerfect
		default:
			continue
		}
		result[honorID] = new(value)
	}
	return result
}

func customProfileUserBondsHonorLevel(resp *sekaiapi.GetAnotherProfileResponse, bondsHonorID int) int {
	if resp == nil {
		return 0
	}
	for _, row := range resp.UserBondsHonors {
		if row.BondsHonorID == bondsHonorID {
			return row.Level
		}
	}
	return 0
}

func customProfileHonorRequestKey(honorID, level int, fullSize bool) string {
	mode := "sub"
	if fullSize {
		mode = "main"
	}
	return fmt.Sprintf("%d:%d:%s", honorID, level, mode)
}

func customProfileBondsHonorRequestKey(honorID, level int, fullSize bool, wordID int, inverse bool, useUnitVirtualSinger bool) string {
	view := "normal"
	if inverse {
		view = "reverse"
	}
	mode := "sub"
	if fullSize {
		mode = "main"
	}
	suffix := ""
	if useUnitVirtualSinger {
		suffix = ":unit_vs"
	}
	return fmt.Sprintf("%d:%d:%s:%d:%s%s", honorID, level, mode, wordID, view, suffix)
}

func customProfileStoryFavoriteKey(storyType string, storyID int) string {
	return fmt.Sprintf("%s:%d", strings.TrimSpace(storyType), storyID)
}

func customProfileBondsViewType(inverse bool) string {
	if inverse {
		return "reverse"
	}
	return ""
}

func addID(set map[int]struct{}, id int) {
	if id > 0 {
		set[id] = struct{}{}
	}
}

func addNestedID(set map[int]map[int]struct{}, id int, nestedID int) {
	if id <= 0 || nestedID <= 0 {
		return
	}
	if set[id] == nil {
		set[id] = make(map[int]struct{})
	}
	set[id][nestedID] = struct{}{}
}

func mapInt(row map[string]any, key string) (int, bool) {
	value, ok := row[key]
	if !ok {
		return 0, false
	}
	switch v := value.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(v))
		return i, err == nil
	default:
		return 0, false
	}
}

func mapString(row map[string]any, key string) string {
	value, ok := row[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}
