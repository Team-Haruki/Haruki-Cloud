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

func (b *Builder) buildBondsHonorRequest(req *drawing.HonorRequest, honorInfo *masterdata.BondsHonor, honorLevel int, bondsHonorViewType string, bondsHonorWordID int, useUnitVirtualSinger bool, region renderregion.Value) error {
	req.HonorType = new("bonds")
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
	if honorInfo.ConfigurableUnitVirtualSinger && bondsHonorUsesUnitVirtualSinger(bondsHonorViewType, useUnitVirtualSinger) {
		applyUnitVirtualSingerDisplaySlots(b.source, displaySlots)
	}
	if bondsHonorViewTypeIsReverse(bondsHonorViewType) {
		displaySlots[0], displaySlots[1] = displaySlots[1], displaySlots[0]
	}
	applyBondsHonorDisplaySlots(req, resolveGameAsset, displaySlots, bgSuffix)

	req.MaskImgPath = new(fmt.Sprintf("%s/honor/mask_degree_%s.png", assets.StaticImagesDir, mode))

	req.FrameImgPath = new(fmt.Sprintf("%s/honor/frame_degree_%s_%d.png", assets.StaticImagesDir, string(mode[0]), mapHonorRarity(honorInfo.HonorRarity)))

	if req.IsMainHonor {
		bundleName := b.bondsHonorWordBundleName(honorInfo, bondsHonorWordID, cid1, cid2)
		req.WordImgPath = new(resolveGameAsset(fmt.Sprintf("bonds_honor/word/%s.png", bundleName)))
	}

	req.LvImgPath = new(filepath.ToSlash(filepath.Join(assets.StaticImagesDir, "honor", "icon_degreeLv.png")))
	req.Lv6ImgPath = new(filepath.ToSlash(filepath.Join(assets.StaticImagesDir, "honor", "icon_degreeLv6.png")))
	return nil
}

func (b *Builder) bondsHonorWordBundleName(honorInfo *masterdata.BondsHonor, wordID, cid1, cid2 int) string {
	if wordID == 0 {
		wordID = honorInfo.ID
	}
	if wordInfo, err := b.source.GetBondsHonorWordByID(wordID); err == nil && wordInfo != nil && strings.TrimSpace(wordInfo.AssetBundleName) != "" {
		return fmt.Sprintf("%s_%02d", strings.TrimSpace(wordInfo.AssetBundleName), max(1, honorInfo.ID%100))
	}
	if absInt(honorInfo.ID-wordID) < 100 {
		return fmt.Sprintf("honorname_%02d%02d_%02d_01", cid1, cid2, wordID%100)
	}
	if wordID%10 == 1 {
		return fmt.Sprintf("honorname_%02d%02d_default_%02d%02d_01", cid1, cid2, cid1, cid2)
	}
	return fmt.Sprintf("honorname_%02d%02d_default_%02d%02d_01", cid1, cid2, cid2, cid1)
}

func applyUnitVirtualSingerDisplaySlots(source DataSource, displaySlots []struct {
	unitID      int
	characterID int
}) {
	if source == nil || len(displaySlots) < 2 {
		return
	}
	leftUnitID := displaySlots[0].unitID
	rightUnitID := displaySlots[1].unitID
	displaySlots[0].unitID = resolveUnitVirtualSingerUnitID(source, leftUnitID, rightUnitID)
	displaySlots[1].unitID = resolveUnitVirtualSingerUnitID(source, rightUnitID, leftUnitID)
}

func resolveUnitVirtualSingerUnitID(source DataSource, candidateUnitID, pairedUnitID int) int {
	candidate, ok := source.GetGameCharacterUnitByID(candidateUnitID)
	if !ok || candidate == nil || candidate.GameCharacterID < 21 {
		return candidateUnitID
	}
	paired, ok := source.GetGameCharacterUnitByID(pairedUnitID)
	if !ok || paired == nil || strings.TrimSpace(paired.Unit) == "" || paired.Unit == "piapro" {
		return candidateUnitID
	}
	for unitID := 27; unitID <= 56; unitID++ {
		unit, ok := source.GetGameCharacterUnitByID(unitID)
		if ok && unit != nil && unit.GameCharacterID == candidate.GameCharacterID && unit.Unit == paired.Unit {
			return unit.ID
		}
	}
	return candidateUnitID
}

func applyBondsHonorDisplaySlots(req *drawing.HonorRequest, resolveGameAsset func(...string) string, displaySlots []struct {
	unitID      int
	characterID int
}, bgSuffix string) {
	if req == nil || len(displaySlots) < 2 {
		return
	}

	req.BondsBgPath = new(fmt.Sprintf("%s/honor/bonds/%d%s.png", assets.StaticImagesDir, displaySlots[0].characterID, bgSuffix))
	req.BondsBgPath2 = new(fmt.Sprintf("%s/honor/bonds/%d%s.png", assets.StaticImagesDir, displaySlots[1].characterID, bgSuffix))

	req.CharaIconPath = new(resolveGameAsset(fmt.Sprintf("bonds_honor/character/chr_sd_%02d_01.png", displaySlots[0].unitID)))
	req.CharaIconPath2 = new(resolveGameAsset(fmt.Sprintf("bonds_honor/character/chr_sd_%02d_01.png", displaySlots[1].unitID)))

	req.CharaID = new(strconv.Itoa(displaySlots[0].unitID))
	req.CharaID2 = new(strconv.Itoa(displaySlots[1].unitID))
}

func bondsHonorViewTypeIsReverse(viewType string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(viewType)), "reverse")
}

func bondsHonorUsesUnitVirtualSinger(viewType string, explicit bool) bool {
	return explicit || strings.Contains(strings.ToLower(strings.TrimSpace(viewType)), "unit_virtual_singer")
}
