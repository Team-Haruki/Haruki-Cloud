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
	visual, honorLevel, err := b.resolveNormalHonorVisual(req, honorID, honorLevel)
	if err != nil {
		return err
	}
	resolveGameAsset := func(relPaths ...string) string {
		return assets.ResolveRegionAssetPath(b.assets, region.String(), relPaths...)
	}

	honorImgPath := b.normalHonorImagePath(visual, resolveGameAsset)
	req.HonorImgPath = &honorImgPath
	b.setNormalHonorRankImage(req, visual, honorImgPath, resolveGameAsset)
	b.setNormalHonorFrame(req, visual, resolveGameAsset)
	b.setNormalHonorProgress(req, visual, honorID, honorLevel, fcOrApLevelOverride, resolveGameAsset)
	setNormalHonorLevelIcons(req, visual.groupType)
	return nil
}

type normalHonorVisual struct {
	group       *masterdata.HonorGroup
	assetName   string
	rarity      string
	bgAssetName string
	groupType   string
	mode        string
	frameName   string
	honorType   string
	rarityRank  int
}

func (b *Builder) resolveNormalHonorVisual(req *drawing.HonorRequest, honorID, honorLevel int) (normalHonorVisual, int, error) {
	honorInfo, _ := b.source.GetHonorByID(honorID)
	group, err := b.source.GetHonorGroupByID(honorInfo.GroupID)
	if err != nil {
		return normalHonorVisual{}, honorLevel, fmt.Errorf("honor group %d not found for honor %d: %w", honorInfo.GroupID, honorID, err)
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
	frameName := ""
	if group.FrameName != nil {
		frameName = *group.FrameName
	}
	honorType := normalHonorType(groupType, frameName, bgAssetName, assetName)
	req.HonorType = &honorType
	return normalHonorVisual{
		group:       group,
		assetName:   assetName,
		rarity:      rarity,
		bgAssetName: bgAssetName,
		groupType:   groupType,
		mode:        mode,
		frameName:   frameName,
		honorType:   honorType,
		rarityRank:  mapHonorRarity(rarity),
	}, honorLevel, nil
}

func (b *Builder) normalHonorImagePath(visual normalHonorVisual, resolveGameAsset func(...string) string) string {
	var honorImgPath string
	switch {
	case visual.groupType == "rank_match":
		honorImgPath = resolveGameAsset(fmt.Sprintf("rank_live/honor/%s/degree_%s.png", visual.bgAssetName, visual.mode))
	case visual.group.BackgroundAssetBundleName != nil && *visual.group.BackgroundAssetBundleName != "":
		honorImgPath = resolveGameAsset(fmt.Sprintf("honor/%s/degree_%s.png", *visual.group.BackgroundAssetBundleName, visual.mode))
	default:
		honorImgPath = resolveGameAsset(fmt.Sprintf("honor/%s/degree_%s.png", visual.assetName, visual.mode))
	}
	if visual.eventType() && !b.assetExists(honorImgPath) {
		if derived := deriveHonorBackgroundAssetName(visual.assetName); derived != "" {
			candidate := resolveGameAsset(fmt.Sprintf("honor/%s/degree_%s.png", derived, visual.mode))
			if b.assetExists(candidate) {
				honorImgPath = candidate
			}
		}
	}
	if visual.eventType() && !b.assetExists(honorImgPath) {
		fallback := normalHonorFallbackPath(visual, resolveGameAsset)
		if b.assetExists(fallback) {
			honorImgPath = fallback
		}
	}
	return honorImgPath
}

func (v normalHonorVisual) eventType() bool {
	return v.groupType == "event" || v.groupType == "wl_event"
}

func normalHonorFallbackPath(visual normalHonorVisual, resolveGameAsset func(...string) string) string {
	if visual.group.BackgroundAssetBundleName != nil && *visual.group.BackgroundAssetBundleName != "" {
		return resolveGameAsset(fmt.Sprintf("honor/%s/rank_%s.png", *visual.group.BackgroundAssetBundleName, visual.mode))
	}
	return resolveGameAsset(fmt.Sprintf("honor/%s/rank_%s.png", visual.assetName, visual.mode))
}

func (b *Builder) setNormalHonorRankImage(req *drawing.HonorRequest, visual normalHonorVisual, honorImgPath string, resolveGameAsset func(...string) string) {
	if visual.assetName == "" {
		return
	}
	switch visual.groupType {
	case "rank_match":
		req.RankImgPath = new(resolveGameAsset(fmt.Sprintf("rank_live/honor/%s/%s.png", visual.assetName, visual.mode)))
	case "sekai_echo":
		rankCandidate := resolveGameAsset(fmt.Sprintf("honor/%s/rank_%s.png", visual.assetName, visual.mode))
		if b.assetExists(rankCandidate) {
			req.RankImgPath = &rankCandidate
		}
	case "event", "wl_event":
		rankCandidate := resolveGameAsset(fmt.Sprintf("honor/%s/rank_%s.png", visual.assetName, visual.mode))
		if rankCandidate != honorImgPath {
			req.RankImgPath = &rankCandidate
		}
	}
}

func normalHonorType(groupType, frameName, bgAssetName, assetName string) string {
	if groupType == "birthday" || strings.HasPrefix(frameName, "honor_frame_birthday") || strings.HasPrefix(bgAssetName, "honor_bg_birthday") || strings.HasPrefix(assetName, "honor_bg_birthday") {
		return "birthday"
	}
	return "normal"
}

func (b *Builder) setNormalHonorFrame(req *drawing.HonorRequest, visual normalHonorVisual, resolveGameAsset func(...string) string) {
	// Level-1 birthday honors render as the plain background without any frame overlay.
	// Some groups still expose birthday frame bundle names, but the actual assets are
	// incomplete or intentionally absent for rarity rank 1.
	if visual.honorType == "birthday" && visual.rarityRank <= 1 {
		req.FrameImgPath = nil
		req.FrameDegreeLevelImgPath = nil
		return
	}
	staticFramePath := fmt.Sprintf("%s/honor/frame_degree_%s_%d.png", assets.StaticImagesDir, string(visual.mode[0]), visual.rarityRank)
	frameName := resolvedNormalHonorFrameName(visual)
	if frameName == "" {
		req.FrameImgPath = &staticFramePath
		return
	}
	b.setNamedNormalHonorFrame(req, visual, frameName, staticFramePath, resolveGameAsset)
}

func resolvedNormalHonorFrameName(visual normalHonorVisual) string {
	if visual.frameName != "" || visual.honorType != "birthday" {
		return visual.frameName
	}
	if strings.HasPrefix(visual.bgAssetName, "honor_bg_birthday_") {
		return "honor_frame_birthday_" + strings.TrimPrefix(visual.bgAssetName, "honor_bg_birthday_")
	}
	if strings.HasPrefix(visual.assetName, "honor_bg_birthday_") {
		return "honor_frame_birthday_" + strings.TrimPrefix(visual.assetName, "honor_bg_birthday_")
	}
	return ""
}

func (b *Builder) setNamedNormalHonorFrame(req *drawing.HonorRequest, visual normalHonorVisual, frameName, staticFramePath string, resolveGameAsset func(...string) string) {
	isBirthdayFrame := strings.HasPrefix(frameName, "honor_frame_birthday")
	startRare := 2
	if strings.HasPrefix(frameName, "event") {
		startRare = 3
	}
	framePath := resolveGameAsset(fmt.Sprintf("honor_frame/%s/frame_degree_%s_%d.png", frameName, string(visual.mode[0]), visual.rarityRank))
	if b.assetExists(framePath) && (isBirthdayFrame || visual.rarityRank >= startRare) {
		req.FrameImgPath = &framePath
	} else {
		req.FrameImgPath = &staticFramePath
	}
	if !isBirthdayFrame || *req.FrameImgPath != framePath {
		return
	}
	levelPath := resolveGameAsset(fmt.Sprintf("honor_frame/%s/frame_degree_level_%d.png", frameName, visual.rarityRank))
	if b.assetExists(levelPath) {
		req.FrameDegreeLevelImgPath = new(levelPath)
	}
}

func (b *Builder) setNormalHonorProgress(req *drawing.HonorRequest, visual normalHonorVisual, honorID, honorLevel int, fcOrApLevelOverride *int, resolveGameAsset func(...string) string) {
	_, hasScore := diffScoreMap[honorID]
	if !hasScore && !visual.eventType() {
		return
	}
	if hasScore {
		req.GroupType = new("fc_ap")
	}
	scrollPath := resolveGameAsset(fmt.Sprintf("honor/%s/scroll.png", visual.assetName))
	if b.assetExists(scrollPath) {
		req.ScrollImgPath = &scrollPath
	}
	if fcOrApLevelOverride != nil {
		honorLevel = *fcOrApLevelOverride
	}
	req.FcOrApLevel = new(strconv.Itoa(honorLevel))
}

func setNormalHonorLevelIcons(req *drawing.HonorRequest, groupType string) {
	if groupType == "character" || groupType == "achievement" || strings.HasPrefix(*req.GroupType, "fc_ap") {
		req.LvImgPath = new(filepath.ToSlash(filepath.Join(assets.StaticImagesDir, "honor", "icon_degreeLv.png")))
		req.Lv6ImgPath = new(filepath.ToSlash(filepath.Join(assets.StaticImagesDir, "honor", "icon_degreeLv6.png")))
	}
}

func resolveHonorLevelVisual(levels []masterdata.HonorLevel, requestedLevel int) (*masterdata.HonorLevel, bool) {
	var bestAtOrBelow *masterdata.HonorLevel
	var firstUsable *masterdata.HonorLevel
	for i := range levels {
		level := &levels[i]
		if !usableHonorLevelVisual(level) {
			continue
		}
		if firstUsable == nil {
			firstUsable = level
		}
		if level.Level == requestedLevel {
			return level, true
		}
		if betterHonorLevelVisual(level, bestAtOrBelow, requestedLevel) {
			bestAtOrBelow = level
		}
	}
	selected := bestAtOrBelow
	if selected == nil {
		selected = firstUsable
	}
	return selected, selected != nil
}

func usableHonorLevelVisual(level *masterdata.HonorLevel) bool {
	return level != nil && (level.AssetBundleName != "" || level.HonorRarity != "")
}

func betterHonorLevelVisual(candidate, current *masterdata.HonorLevel, requestedLevel int) bool {
	return requestedLevel > 0 && candidate.Level <= requestedLevel && (current == nil || candidate.Level > current.Level)
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
