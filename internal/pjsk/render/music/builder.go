package music

import (
	"fmt"
	"path/filepath"
	"strings"

	"haruki-cloud/internal/pjsk/render/assets"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

type Builder struct {
	source   DataSource
	fallback DataSource
	assets   *assets.AssetHelper
}

func NewBuilder(source DataSource, fallback DataSource, assetHelper *assets.AssetHelper) *Builder {
	if assetHelper == nil {
		assetHelper = assets.NewAssetHelper("", nil)
	}
	return &Builder{
		source:   source,
		fallback: fallback,
		assets:   assetHelper,
	}
}

func (b *Builder) GetDifficultyLevel(musicID int, difficulty string) int {
	difficulties, err := b.source.GetMusicDifficulties(musicID)
	if err != nil {
		return 0
	}
	for _, diff := range difficulties {
		if strings.EqualFold(strings.TrimSpace(diff.MusicDifficulty), difficulty) {
			return diff.PlayLevel
		}
	}
	return 0
}

func (b *Builder) BuildMusicJacketPath(assetName string, region renderregion.Value) string {
	if strings.TrimSpace(assetName) == "" {
		return ""
	}
	return assets.ResolveRegionAssetPath(b.assets, region.String(), filepath.Join("music", "jacket", assetName, assetName+".png"))
}

func (b *Builder) BuildCharacterIconPath(characterID int, _ renderregion.Value) string {
	if nickname, ok := assets.CharacterIDToNickname[characterID]; ok {
		return assets.ResolveAssetPath(b.assets, assets.StaticImagesDir, filepath.Join("chara_icon", nickname+".png"))
	}
	return assets.ResolveAssetPath(b.assets, assets.StaticImagesDir, filepath.Join("chara_icon", fmt.Sprintf("chr_icon_%d.png", characterID)))
}
