package common

import (
	"fmt"
	"path/filepath"
	"strings"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

func ResolveCardMemberImagePath(helper *assets.AssetHelper, region renderregion.Value, assetBundleName string, fileName string) string {
	assetBundleName = strings.TrimSpace(assetBundleName)
	fileName = strings.TrimSpace(fileName)
	if assetBundleName == "" || fileName == "" {
		return ""
	}

	relPaths := []string{
		filepath.Join("character", "member", assetBundleName, fileName),
	}
	if !strings.HasSuffix(assetBundleName, "_rip") {
		relPaths = append(relPaths, filepath.Join("character", "member", assetBundleName+"_rip", fileName))
	}
	return assets.ResolveRegionAssetPath(helper, region.String(), relPaths...)
}

func ResolveCardThumbnailPath(helper *assets.AssetHelper, region renderregion.Value, assetBundleName string, trainedArt bool) string {
	assetBundleName = strings.TrimSpace(assetBundleName)
	if assetBundleName == "" {
		return ""
	}

	fileSuffix := "_normal.png"
	memberFile := "card_normal.png"
	if trainedArt {
		fileSuffix = "_after_training.png"
		memberFile = "card_after_training.png"
	}

	relPaths := []string{
		filepath.Join("thumbnail", "chara", assetBundleName+fileSuffix),
		filepath.Join("character", "member", assetBundleName, memberFile),
	}
	if !strings.HasSuffix(assetBundleName, "_rip") {
		relPaths = append(relPaths, filepath.Join("character", "member", assetBundleName+"_rip", memberFile))
	}
	return assets.ResolveRegionAssetPath(helper, region.String(), relPaths...)
}

func BuildCardThumbnail(helper *assets.AssetHelper, card *masterdata.Card, region renderregion.Value, opts ThumbnailOptions) drawing.CardFullThumbnailRequest {
	thumbPath := opts.ThumbnailPath
	if thumbPath == "" {
		thumbPath = ResolveCardThumbnailPath(helper, region, card.AssetBundleName, opts.TrainedArt)
	} else {
		thumbPath = filepath.ToSlash(thumbPath)
	}

	rareImg := opts.RareImgPath
	if rareImg == "" {
		fileName := "rare_star_normal.png"
		if opts.AfterTraining {
			fileName = "rare_star_after_training.png"
		}
		rareImg = assets.ResolveAssetPath(helper, assets.StaticImagesDir, filepath.Join("card", fileName))
	} else {
		rareImg = filepath.ToSlash(rareImg)
	}

	birthdayIcon := opts.BirthdayIconPath
	if birthdayIcon == nil && card.CardRarityType == "rarity_birthday" {
		birthdayIcon = new(assets.ResolveAssetPath(helper, assets.StaticImagesDir, filepath.Join("card", "rare_birthday.png")))
	}

	trainRank := opts.TrainRank
	trainRankImgPath := opts.TrainRankImgPath
	if trainRank == nil {
		trainRank = new(0)
	} else if *trainRank > 0 && trainRankImgPath == nil {
		trainRankImgPath = new(assets.ResolveAssetPath(helper, assets.StaticImagesDir, filepath.Join("card", fmt.Sprintf("train_rank_%d.png", *trainRank))))
	}

	framePath := assets.ResolveAssetPath(helper, assets.StaticImagesDir, filepath.Join("card", fmt.Sprintf("frame_%s.png", card.CardRarityType)))
	attrPath := assets.ResolveAssetPath(helper, assets.StaticImagesDir, filepath.Join("card", fmt.Sprintf("attr_%s.png", strings.ToLower(card.Attr))))

	return drawing.CardFullThumbnailRequest{
		CardID:            card.ID,
		CardThumbnailPath: thumbPath,
		Rare:              card.CardRarityType,
		FrameImgPath:      framePath,
		AttrImgPath:       attrPath,
		RareImgPath:       rareImg,
		TrainRank:         trainRank,
		TrainRankImgPath:  trainRankImgPath,
		Level:             opts.Level,
		BirthdayIconPath:  birthdayIcon,
		IsAfterTraining:   new(opts.AfterTraining),
		CustomText:        opts.CustomText,
		CardLevel:         opts.CardLevel,
		IsPcard:           opts.IsPcard,
	}
}
