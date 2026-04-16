package honor

import (
	"fmt"
	"path/filepath"
	"strconv"

	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/drawing"
)

func (b *Builder) buildBondsHonorRequest(req *drawing.HonorRequest, honorInfo *masterdata.BondsHonor, honorLevel, bondsHonorWordID int, region renderregion.Value) error {
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

	bgPath1 := fmt.Sprintf("%s/honor/bonds/%d%s.png", assets.StaticImagesDir, cid1, bgSuffix)
	bgPath2 := fmt.Sprintf("%s/honor/bonds/%d%s.png", assets.StaticImagesDir, cid2, bgSuffix)
	req.BondsBgPath = &bgPath1
	req.BondsBgPath2 = &bgPath2

	resolveGameAsset := func(relPaths ...string) string {
		return assets.ResolveRegionAssetPath(b.assets, region.String(), relPaths...)
	}

	charaPath1 := resolveGameAsset(fmt.Sprintf("bonds_honor/character/chr_sd_%02d_01.png", cuid1))
	charaPath2 := resolveGameAsset(fmt.Sprintf("bonds_honor/character/chr_sd_%02d_01.png", cuid2))
	req.CharaIconPath = &charaPath1
	req.CharaIconPath2 = &charaPath2

	cuid1Text := strconv.Itoa(cuid1)
	cuid2Text := strconv.Itoa(cuid2)
	req.CharaID = &cuid1Text
	req.CharaID2 = &cuid2Text

	maskPath := fmt.Sprintf("%s/honor/mask_degree_%s.png", assets.StaticImagesDir, mode)
	req.MaskImgPath = &maskPath

	framePath := fmt.Sprintf("%s/honor/frame_degree_%s_%d.png", assets.StaticImagesDir, string(mode[0]), mapHonorRarity(honorInfo.HonorRarity))
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
		wordPath := resolveGameAsset(fmt.Sprintf("bonds_honor/word/%s.png", bundleName))
		req.WordImgPath = &wordPath
	}

	lvImg := filepath.ToSlash(filepath.Join(assets.StaticImagesDir, "honor", "icon_degreeLv.png"))
	lv6Img := filepath.ToSlash(filepath.Join(assets.StaticImagesDir, "honor", "icon_degreeLv6.png"))
	req.LvImgPath = &lvImg
	req.Lv6ImgPath = &lv6Img
	return nil
}
