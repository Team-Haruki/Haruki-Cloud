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

func (b *Builder) buildBondsHonorRequest(req *drawing.HonorRequest, honorInfo *masterdata.BondsHonor, honorLevel int, bondsHonorViewType string, bondsHonorWordID int, region renderregion.Value) error {
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

	resolveGameAsset := func(relPaths ...string) string {
		return assets.ResolveRegionAssetPath(b.assets, region.String(), relPaths...)
	}

	displaySlots := []struct {
		unitID      int
		characterID int
	}{
		{unitID: cuid1, characterID: cid1},
		{unitID: cuid2, characterID: cid2},
	}
	if bondsHonorViewTypeIsReverse(bondsHonorViewType) {
		displaySlots[0], displaySlots[1] = displaySlots[1], displaySlots[0]
	}
	applyBondsHonorDisplaySlots(req, resolveGameAsset, displaySlots, bgSuffix)

	maskPath := fmt.Sprintf("%s/honor/mask_degree_%s.png", assets.StaticImagesDir, mode)
	req.MaskImgPath = &maskPath

	framePath := fmt.Sprintf("%s/honor/frame_degree_%s_%d.png", assets.StaticImagesDir, string(mode[0]), mapHonorRarity(honorInfo.HonorRarity))
	req.FrameImgPath = &framePath

	if req.IsMainHonor {
		wordID := bondsHonorWordID
		if wordID == 0 {
			wordID = honorInfo.ID
		}
		viewSuffix := "01"
		if bondsHonorViewTypeIsReverse(bondsHonorViewType) {
			viewSuffix = "02"
		}
		var bundleName string
		if absInt(honorInfo.ID-wordID) < 100 {
			bundleName = fmt.Sprintf("honorname_%02d%02d_%02d_%s", cid1, cid2, wordID%100, viewSuffix)
		} else if wordID%10 == 1 {
			bundleName = fmt.Sprintf("honorname_%02d%02d_default_%02d%02d_%s", cid1, cid2, cid1, cid2, viewSuffix)
		} else {
			bundleName = fmt.Sprintf("honorname_%02d%02d_default_%02d%02d_%s", cid1, cid2, cid2, cid1, viewSuffix)
		}
		wordPath := resolveGameAsset(fmt.Sprintf("bonds_honor/word/%s.png", bundleName))
		req.WordImgPath = &wordPath
	}

	lvImg := filepath.ToSlash(filepath.Join(assets.StaticImagesDir, "honor", "icon_degreeLv.png"))
	lv6Img := filepath.ToSlash(filepath.Join(assets.StaticImagesDir, "honor", "icon_degreeLv6.png"))
	req.LvImgPath = &lvImg
	req.Lv6ImgPath = &lv6Img
	return nil
}

func applyBondsHonorDisplaySlots(req *drawing.HonorRequest, resolveGameAsset func(...string) string, displaySlots []struct {
	unitID      int
	characterID int
}, bgSuffix string) {
	if req == nil || len(displaySlots) < 2 {
		return
	}

	bgPath1 := fmt.Sprintf("%s/honor/bonds/%d%s.png", assets.StaticImagesDir, displaySlots[0].characterID, bgSuffix)
	bgPath2 := fmt.Sprintf("%s/honor/bonds/%d%s.png", assets.StaticImagesDir, displaySlots[1].characterID, bgSuffix)
	req.BondsBgPath = &bgPath1
	req.BondsBgPath2 = &bgPath2

	charaPath1 := resolveGameAsset(fmt.Sprintf("bonds_honor/character/chr_sd_%02d_01.png", displaySlots[0].unitID))
	charaPath2 := resolveGameAsset(fmt.Sprintf("bonds_honor/character/chr_sd_%02d_01.png", displaySlots[1].unitID))
	req.CharaIconPath = &charaPath1
	req.CharaIconPath2 = &charaPath2

	charaID1 := strconv.Itoa(displaySlots[0].unitID)
	charaID2 := strconv.Itoa(displaySlots[1].unitID)
	req.CharaID = &charaID1
	req.CharaID2 = &charaID2
}

func bondsHonorViewTypeIsReverse(viewType string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(viewType)), "reverse")
}
