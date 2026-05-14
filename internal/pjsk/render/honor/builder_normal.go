package honor

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

func (b *Builder) buildNormalHonorRequest(req *drawing.HonorRequest, honorID, honorLevel int, fcOrApLevelOverride *int, region renderregion.Value) error {
	honorInfo, _ := b.source.GetHonorByID(honorID)
	group, err := b.source.GetHonorGroupByID(honorInfo.GroupID)
	if err != nil {
		return fmt.Errorf("honor group %d not found for honor %d: %w", honorInfo.GroupID, honorID, err)
	}

	assetName := honorInfo.AssetBundleName
	rarity := honorInfo.HonorRarity
	if resolvedLevel, ok := resolveHonorLevelVisual(honorInfo.Levels, honorLevel); ok {
		if assetName == "" && resolvedLevel.AssetBundleName != "" {
			assetName = resolvedLevel.AssetBundleName
		}
		if rarity == "" && resolvedLevel.HonorRarity != "" {
			rarity = resolvedLevel.HonorRarity
		}
		if honorLevel <= 0 {
			honorLevel = resolvedLevel.Level
		}
	}

	req.HonorLevel = &honorLevel

	bgAssetName := assetName
	if group.BackgroundAssetBundleName != nil && *group.BackgroundAssetBundleName != "" {
		bgAssetName = *group.BackgroundAssetBundleName
	}

	groupType := group.HonorType
	if isWorldLinkHonorGroup(groupType, bgAssetName, assetName) {
		groupType = "wl_event"
	}
	req.GroupType = &groupType
	req.HonorRarity = &rarity

	mode := "sub"
	if req.IsMainHonor {
		mode = "main"
	}

	resolveGameAsset := func(relPaths ...string) string {
		return assets.ResolveRegionAssetPath(b.assets, region.String(), relPaths...)
	}

	var honorImgPath string
	switch {
	case groupType == "rank_match":
		honorImgPath = resolveGameAsset(fmt.Sprintf("rank_live/honor/%s/degree_%s.png", bgAssetName, mode))
	case group.BackgroundAssetBundleName != nil && *group.BackgroundAssetBundleName != "":
		honorImgPath = resolveGameAsset(fmt.Sprintf("honor/%s/degree_%s.png", *group.BackgroundAssetBundleName, mode))
	default:
		honorImgPath = resolveGameAsset(fmt.Sprintf("honor/%s/degree_%s.png", assetName, mode))
	}
	if (groupType == "event" || groupType == "wl_event") && !b.assetExists(honorImgPath) {
		if derived := deriveHonorBackgroundAssetName(assetName); derived != "" {
			candidate := resolveGameAsset(fmt.Sprintf("honor/%s/degree_%s.png", derived, mode))
			if b.assetExists(candidate) {
				honorImgPath = candidate
			}
		}
	}
	if (groupType == "event" || groupType == "wl_event") && !b.assetExists(honorImgPath) {
		var fallback string
		if group.BackgroundAssetBundleName != nil && *group.BackgroundAssetBundleName != "" {
			fallback = resolveGameAsset(fmt.Sprintf("honor/%s/rank_%s.png", *group.BackgroundAssetBundleName, mode))
		} else {
			fallback = resolveGameAsset(fmt.Sprintf("honor/%s/rank_%s.png", assetName, mode))
		}
		if b.assetExists(fallback) {
			honorImgPath = fallback
		}
	}
	req.HonorImgPath = &honorImgPath

	if assetName != "" && (groupType == "event" || groupType == "wl_event" || groupType == "rank_match") {
		switch groupType {
		case "rank_match":
			req.RankImgPath = new(resolveGameAsset(fmt.Sprintf("rank_live/honor/%s/%s.png", assetName, mode)))
		case "event":
			rankCandidate := resolveGameAsset(fmt.Sprintf("honor/%s/rank_%s.png", assetName, mode))
			if rankCandidate != honorImgPath {
				req.RankImgPath = &rankCandidate
			}
		case "wl_event":
			rankCandidate := resolveGameAsset(fmt.Sprintf("honor/%s/rank_%s.png", assetName, mode))
			if rankCandidate != honorImgPath {
				req.RankImgPath = &rankCandidate
			}
		}
	}

	frameName := ""
	if group.FrameName != nil {
		frameName = *group.FrameName
	}
	honorType := "normal"
	if groupType == "birthday" || strings.HasPrefix(frameName, "honor_frame_birthday") || strings.HasPrefix(bgAssetName, "honor_bg_birthday") || strings.HasPrefix(assetName, "honor_bg_birthday") {
		honorType = "birthday"
	}
	req.HonorType = &honorType
	rarityRank := mapHonorRarity(rarity)

	// Level-1 birthday honors render as the plain background without any frame overlay.
	// Some groups still expose birthday frame bundle names, but the actual assets are
	// incomplete or intentionally absent for rarity rank 1.
	if honorType == "birthday" && rarityRank <= 1 {
		req.FrameImgPath = nil
		req.FrameDegreeLevelImgPath = nil
	} else {
		staticFramePath := fmt.Sprintf("%s/honor/frame_degree_%s_%d.png", assets.StaticImagesDir, string(mode[0]), rarityRank)

		if honorType == "birthday" && frameName == "" {
			switch {
			case strings.HasPrefix(bgAssetName, "honor_bg_birthday_"):
				frameName = "honor_frame_birthday_" + strings.TrimPrefix(bgAssetName, "honor_bg_birthday_")
			case strings.HasPrefix(assetName, "honor_bg_birthday_"):
				frameName = "honor_frame_birthday_" + strings.TrimPrefix(assetName, "honor_bg_birthday_")
			}
		}
		if frameName != "" {
			isBirthdayFrame := strings.HasPrefix(frameName, "honor_frame_birthday")
			startRare := 2
			if strings.HasPrefix(frameName, "event") {
				startRare = 3
			}
			framePath := resolveGameAsset(fmt.Sprintf("honor_frame/%s/frame_degree_%s_%d.png", frameName, string(mode[0]), rarityRank))
			if isBirthdayFrame && b.assetExists(framePath) {
				req.FrameImgPath = &framePath
			} else if rarityRank >= startRare && b.assetExists(framePath) {
				req.FrameImgPath = &framePath
			} else {
				req.FrameImgPath = &staticFramePath
			}
			if isBirthdayFrame && req.FrameImgPath != nil && *req.FrameImgPath == framePath {
				levelPath := resolveGameAsset(fmt.Sprintf("honor_frame/%s/frame_degree_level_%d.png", frameName, rarityRank))
				if b.assetExists(levelPath) {
					req.FrameDegreeLevelImgPath = new(levelPath)
				}
			}
		} else {
			req.FrameImgPath = &staticFramePath
		}
	}

	_, hasScore := diffScoreMap[honorID]
	if hasScore || groupType == "event" || groupType == "wl_event" {
		if hasScore {
			req.GroupType = new("fc_ap")
		}
		scrollPath := resolveGameAsset(fmt.Sprintf("honor/%s/scroll.png", assetName))
		if b.assetExists(scrollPath) {
			req.ScrollImgPath = &scrollPath
		}
		fcApLevelValue := honorLevel
		if fcOrApLevelOverride != nil {
			fcApLevelValue = *fcOrApLevelOverride
		}
		req.FcOrApLevel = new(strconv.Itoa(fcApLevelValue))
	}

	if groupType == "character" || groupType == "achievement" || strings.HasPrefix(*req.GroupType, "fc_ap") {
		req.LvImgPath = new(filepath.ToSlash(filepath.Join(assets.StaticImagesDir, "honor", "icon_degreeLv.png")))
		req.Lv6ImgPath = new(filepath.ToSlash(filepath.Join(assets.StaticImagesDir, "honor", "icon_degreeLv6.png")))
	}
	return nil
}

func resolveHonorLevelVisual(levels []masterdata.HonorLevel, requestedLevel int) (*masterdata.HonorLevel, bool) {
	var bestAtOrBelow *masterdata.HonorLevel
	var firstUsable *masterdata.HonorLevel

	for i := range levels {
		level := &levels[i]
		if level.AssetBundleName == "" && level.HonorRarity == "" {
			continue
		}
		if firstUsable == nil {
			firstUsable = level
		}
		if level.Level == requestedLevel {
			return level, true
		}
		if requestedLevel > 0 && level.Level <= requestedLevel {
			if bestAtOrBelow == nil || level.Level > bestAtOrBelow.Level {
				bestAtOrBelow = level
			}
		}
	}

	if bestAtOrBelow != nil {
		return bestAtOrBelow, true
	}
	if firstUsable != nil {
		return firstUsable, true
	}
	return nil, false
}

func (b *Builder) assetExists(rel string) bool {
	rel = strings.TrimSpace(rel)
	if rel == "" || b.assets == nil {
		return false
	}
	return b.assets.FirstExisting(filepath.ToSlash(rel)) != ""
}

func mapHonorRarity(rarity string) int {
	switch rarity {
	case "middle":
		return 2
	case "high":
		return 3
	case "highest":
		return 4
	default:
		return 1
	}
}

var diffScoreMap = map[int]struct {
	diff  string
	score string
}{
	3009: {diff: "easy", score: "fullCombo"},
	3010: {diff: "normal", score: "fullCombo"},
	3011: {diff: "hard", score: "fullCombo"},
	3012: {diff: "expert", score: "fullCombo"},
	3013: {diff: "master", score: "fullCombo"},
	3014: {diff: "master", score: "allPerfect"},
	4700: {diff: "append", score: "fullCombo"},
	4701: {diff: "append", score: "allPerfect"},
}

func LookupFcApCounter(honorID int) (difficulty string, score string, ok bool) {
	spec, ok := diffScoreMap[honorID]
	if !ok {
		return "", "", false
	}
	return spec.diff, spec.score, true
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func deriveHonorBackgroundAssetName(assetName string) string {
	assetName = strings.TrimSpace(assetName)
	if !strings.HasPrefix(assetName, "honor_top_") {
		return ""
	}
	parts := strings.SplitN(assetName, "_", 4)
	if len(parts) != 4 {
		return ""
	}
	return "honor_bg_" + parts[3]
}

func isWorldLinkHonorGroup(groupType, bgAssetName, assetName string) bool {
	if groupType == "world_link" {
		return true
	}
	bgAssetName = strings.TrimSpace(bgAssetName)
	assetName = strings.TrimSpace(assetName)
	return strings.Contains(bgAssetName, "event_wl") || strings.Contains(assetName, "event_wl")
}
