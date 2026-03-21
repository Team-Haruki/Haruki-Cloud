package honor

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/utils/drawing"
)

type Builder struct {
	source DataSource
	assets *assets.AssetHelper
}

func NewBuilder(source DataSource, assetHelper *assets.AssetHelper) *Builder {
	return &Builder{
		source: source,
		assets: assetHelper,
	}
}

func (b *Builder) BuildHonorRequest(query Query) (*drawing.HonorRequest, error) {
	req := &drawing.HonorRequest{
		IsMainHonor: query.IsMain,
	}

	_, errNormal := b.source.GetHonorByID(query.HonorID)
	bondsHonor, errBonds := b.source.GetBondsHonorByID(query.HonorID)

	isNormal := errNormal == nil
	isBonds := errBonds == nil
	if !isNormal && !isBonds {
		return nil, fmt.Errorf("honor %d not found in any masterdata table", query.HonorID)
	}

	if isNormal {
		if err := b.buildNormalHonorRequest(req, query.HonorID, query.HonorLevel); err != nil {
			return nil, err
		}
	} else if isBonds {
		if err := b.buildBondsHonorRequest(req, bondsHonor, query.HonorLevel, query.BondsHonorWordID); err != nil {
			return nil, err
		}
	}

	if query.Rank > 0 {
		rankText := strconv.Itoa(query.Rank)
		req.FcOrApLevel = &rankText
	}

	return req, nil
}

func (b *Builder) buildNormalHonorRequest(req *drawing.HonorRequest, honorID, honorLevel int) error {
	honorInfo, _ := b.source.GetHonorByID(honorID)
	group, err := b.source.GetHonorGroupByID(honorInfo.GroupID)
	if err != nil {
		return fmt.Errorf("honor group %d not found for honor %d: %w", honorInfo.GroupID, honorID, err)
	}

	assetName := honorInfo.AssetBundleName
	rarity := honorInfo.HonorRarity
	for _, level := range honorInfo.Levels {
		if level.Level != honorLevel {
			continue
		}
		if level.AssetBundleName != "" {
			assetName = level.AssetBundleName
		}
		if level.HonorRarity != "" {
			rarity = level.HonorRarity
		}
		break
	}

	req.HonorLevel = &honorLevel
	honorType := "normal"
	req.HonorType = &honorType

	groupType := group.HonorType
	if groupType == "world_link" {
		groupType = "wl_event"
	}
	req.GroupType = &groupType
	req.HonorRarity = &rarity

	bgAssetName := assetName
	if group.BackgroundAssetBundleName != nil && *group.BackgroundAssetBundleName != "" {
		bgAssetName = *group.BackgroundAssetBundleName
	}

	mode := "sub"
	if req.IsMainHonor {
		mode = "main"
	}

	var honorImgPath string
	switch {
	case group.BackgroundAssetBundleName != nil && *group.BackgroundAssetBundleName != "":
		honorImgPath = fmt.Sprintf("honor/%s/degree_%s.png", *group.BackgroundAssetBundleName, mode)
	case groupType == "rank_match":
		honorImgPath = fmt.Sprintf("rank_live/honor/%s/degree_%s.png", bgAssetName, mode)
	default:
		honorImgPath = fmt.Sprintf("honor/%s/degree_%s.png", assetName, mode)
	}
	if (groupType == "event" || groupType == "wl_event") && !b.assetExists(honorImgPath) {
		if derived := deriveHonorBackgroundAssetName(assetName); derived != "" {
			candidate := fmt.Sprintf("honor/%s/degree_%s.png", derived, mode)
			if b.assetExists(candidate) {
				honorImgPath = candidate
			}
		}
	}
	if (groupType == "event" || groupType == "wl_event") && !b.assetExists(honorImgPath) {
		var fallback string
		if group.BackgroundAssetBundleName != nil && *group.BackgroundAssetBundleName != "" {
			fallback = fmt.Sprintf("honor/%s/rank_%s.png", *group.BackgroundAssetBundleName, mode)
		} else {
			fallback = fmt.Sprintf("honor/%s/rank_%s.png", assetName, mode)
		}
		if b.assetExists(fallback) {
			honorImgPath = fallback
		}
	}
	req.HonorImgPath = &honorImgPath

	if assetName != "" && (groupType == "event" || groupType == "wl_event" || groupType == "rank_match") {
		switch groupType {
		case "rank_match":
			rankImgPath := fmt.Sprintf("rank_live/honor/%s/%s.png", assetName, mode)
			req.RankImgPath = &rankImgPath
		case "event", "wl_event":
			rankCandidate := fmt.Sprintf("honor/%s/rank_%s.png", assetName, mode)
			degreeCandidate := fmt.Sprintf("honor/%s/degree_%s.png", assetName, mode)
			switch {
			case rankCandidate != honorImgPath && b.assetExists(rankCandidate):
				req.RankImgPath = &rankCandidate
			case degreeCandidate != honorImgPath && b.assetExists(degreeCandidate):
				req.RankImgPath = &degreeCandidate
			}
		}
	}

	frameName := ""
	if group.FrameName != nil {
		frameName = *group.FrameName
	}
	rarityRank := mapHonorRarity(rarity)
	if frameName != "" {
		framePath := fmt.Sprintf("honor_frame/%s/frame_degree_%s_%d.png", frameName, string(mode[0]), rarityRank)
		req.FrameImgPath = &framePath
	} else {
		framePath := fmt.Sprintf("honor/frame_degree_%s_%d.png", string(mode[0]), rarityRank)
		req.FrameImgPath = &framePath
	}

	scoreInfo, hasScore := diffScoreMap[honorID]
	if hasScore || groupType == "event" || groupType == "wl_event" {
		if hasScore {
			fcApGroup := "fc_ap"
			req.GroupType = &fcApGroup
			_ = scoreInfo
		}
		scrollPath := fmt.Sprintf("honor/%s/scroll.png", assetName)
		if b.assetExists(scrollPath) {
			req.ScrollImgPath = &scrollPath
		}
		fcApLevel := strconv.Itoa(honorLevel)
		req.FcOrApLevel = &fcApLevel
	}

	if groupType == "character" || groupType == "achievement" || strings.HasPrefix(*req.GroupType, "fc_ap") {
		lvImg := "honor/icon_degreeLv.png"
		lv6Img := "honor/icon_degreeLv6.png"
		req.LvImgPath = &lvImg
		req.Lv6ImgPath = &lv6Img
	}
	return nil
}

func (b *Builder) buildBondsHonorRequest(req *drawing.HonorRequest, honorInfo *masterdata.BondsHonor, honorLevel, bondsHonorWordID int) error {
	honorType := "bonds"
	req.HonorType = &honorType
	req.HonorRarity = &honorInfo.HonorRarity
	req.HonorLevel = &honorLevel

	mode := "sub"
	if req.IsMainHonor {
		mode = "main"
	}

	cuid1 := honorInfo.GameCharacterUnitID1
	cuid2 := honorInfo.GameCharacterUnitID2

	bgSuffix := "_sub"
	if req.IsMainHonor {
		bgSuffix = ""
	}

	var cid1, cid2 int
	if unit1, ok := b.source.GetGameCharacterUnitByID(cuid1); ok {
		cid1 = unit1.GameCharacterID
	}
	if unit2, ok := b.source.GetGameCharacterUnitByID(cuid2); ok {
		cid2 = unit2.GameCharacterID
	}

	bgPath1 := fmt.Sprintf("honor/bonds/%d%s.png", cid1, bgSuffix)
	bgPath2 := fmt.Sprintf("honor/bonds/%d%s.png", cid2, bgSuffix)
	req.BondsBgPath = &bgPath1
	req.BondsBgPath2 = &bgPath2

	charaPath1 := fmt.Sprintf("bonds_honor/character/chr_sd_%02d_01.png", cuid1)
	charaPath2 := fmt.Sprintf("bonds_honor/character/chr_sd_%02d_01.png", cuid2)
	req.CharaIconPath = &charaPath1
	req.CharaIconPath2 = &charaPath2

	cuid1Text := strconv.Itoa(cuid1)
	cuid2Text := strconv.Itoa(cuid2)
	req.CharaID = &cuid1Text
	req.CharaID2 = &cuid2Text

	maskPath := fmt.Sprintf("honor/mask_degree_%s.png", mode)
	req.MaskImgPath = &maskPath

	framePath := fmt.Sprintf("honor/frame_degree_%s_%d.png", string(mode[0]), mapHonorRarity(honorInfo.HonorRarity))
	req.FrameImgPath = &framePath

	if req.IsMainHonor {
		wordID := bondsHonorWordID
		if wordID == 0 {
			wordID = honorInfo.ID
		}
		var bundleName string
		if absInt(honorInfo.ID-wordID) < 100 {
			bundleName = fmt.Sprintf("honorname_%02d%02d_%02d_01", cid1, cid2, wordID%100)
		} else if wordID%10 == 1 {
			bundleName = fmt.Sprintf("honorname_%02d%02d_default_%02d%02d_01", cid1, cid2, cuid1, cid2)
		} else {
			bundleName = fmt.Sprintf("honorname_%02d%02d_default_%02d%02d_01", cid1, cid2, cid2, cuid1)
		}
		wordPath := fmt.Sprintf("bonds_honor/word/%s.png", bundleName)
		req.WordImgPath = &wordPath
	}

	lvImg := "honor/icon_degreeLv.png"
	lv6Img := "honor/icon_degreeLv6.png"
	req.LvImgPath = &lvImg
	req.Lv6ImgPath = &lv6Img
	return nil
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
