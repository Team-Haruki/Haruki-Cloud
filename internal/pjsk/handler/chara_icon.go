package handler

import (
	"fmt"
	"path/filepath"

	"haruki-cloud/internal/pjsk/render/assets"
)

func charaIconPath(helper *assets.AssetHelper, charID int) string {
	if nickname, ok := assets.CharacterIDToNickname[charID]; ok {
		return assets.ResolveAssetPath(helper, assets.StaticImagesDir,
			filepath.Join("chara_icon", nickname+".png"),
			filepath.Join("chara_icon", fmt.Sprintf("chr_icon_%d.png", charID)))
	}
	return assets.ResolveAssetPath(helper, assets.StaticImagesDir,
		filepath.Join("chara_icon", fmt.Sprintf("chr_icon_%d.png", charID)))
}
