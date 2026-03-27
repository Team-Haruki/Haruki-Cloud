package common

import (
	"fmt"
	"path/filepath"
	"strings"

	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/utils/drawing"
)

type ThumbnailOptions struct {
	AfterTraining    bool
	TrainedArt       bool
	ThumbnailPath    string
	RareImgPath      string
	TrainRank        *int
	TrainRankImgPath *string
	Level            *int
	BirthdayIconPath *string
	CustomText       *string
	CardLevel        map[string]interface{}
	IsPcard          bool
}

func BuildCardThumbnail(helper *assets.AssetHelper, card *masterdata.Card, region renderregion.Value, opts ThumbnailOptions) drawing.CardFullThumbnailRequest {
	thumbPath := opts.ThumbnailPath
	if thumbPath == "" {
		fileSuffix := "_normal.png"
		memberFile := "card_normal.png"
		if opts.TrainedArt {
			fileSuffix = "_after_training.png"
			memberFile = "card_after_training.png"
		}
		thumbPath = assets.ResolveRegionAssetPath(helper, region.String(),
			filepath.Join("thumbnail", "chara", card.AssetBundleName+fileSuffix),
			filepath.Join("character", "member", card.AssetBundleName, memberFile),
		)
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

	isAfter := opts.AfterTraining
	birthdayIcon := opts.BirthdayIconPath
	if birthdayIcon == nil && card.CardRarityType == "rarity_birthday" {
		path := assets.ResolveAssetPath(helper, assets.StaticImagesDir, filepath.Join("card", "rare_birthday.png"))
		birthdayIcon = &path
	}

	trainRank := opts.TrainRank
	trainRankImgPath := opts.TrainRankImgPath
	if trainRank == nil {
		defaultRank := 0
		trainRank = &defaultRank
	} else if *trainRank > 0 && trainRankImgPath == nil {
		path := assets.ResolveAssetPath(helper, assets.StaticImagesDir, filepath.Join("card", fmt.Sprintf("train_rank_%d.png", *trainRank)))
		trainRankImgPath = &path
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
		IsAfterTraining:   &isAfter,
		CustomText:        opts.CustomText,
		CardLevel:         opts.CardLevel,
		IsPcard:           opts.IsPcard,
	}
}
