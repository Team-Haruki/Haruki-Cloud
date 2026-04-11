package card

import (
	"fmt"
	"path/filepath"
	"strings"

	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

type ImageResult struct {
	Card  *masterdata.Card
	Paths []string
}

func (c *Controller) ResolveCardImages(query Query) (*ImageResult, error) {
	if c == nil {
		return nil, fmt.Errorf("card controller is not configured")
	}

	region, source, _, err := c.resolveBuilder(query.Region)
	if err != nil {
		return nil, err
	}
	searcher := NewSearchService(source, NewParser(c.nicknames))
	cardInfo, err := searcher.Search(query.Query)
	if err != nil {
		return nil, fmt.Errorf("failed to search card: %w", err)
	}

	paths := resolveCardOriginalImagePaths(c.assets, region, cardInfo)
	if len(paths) == 0 {
		return nil, fmt.Errorf("card %d does not have original image assets", cardInfo.ID)
	}

	copy := *cardInfo
	if cardInfo.CardParameters != nil {
		copy.CardParameters = append([]masterdata.CardParameter(nil), cardInfo.CardParameters...)
	}
	return &ImageResult{
		Card:  &copy,
		Paths: paths,
	}, nil
}

func resolveCardOriginalImagePaths(helper *assets.AssetHelper, region renderregion.Value, card *masterdata.Card) []string {
	if card == nil || strings.TrimSpace(card.AssetBundleName) == "" {
		return nil
	}

	base := filepath.Join("character", "member", card.AssetBundleName)

	type candidate struct {
		primary  string
		fallback string
	}
	var candidates []candidate

	if !onlyHasAfterTrainingCard(card) {
		candidates = append(candidates, candidate{
			primary: filepath.Join(base, "card_normal.png"),
		})
	}
	if card.CardRarityType == "rarity_3" || card.CardRarityType == "rarity_4" {
		candidates = append(candidates, candidate{
			primary: filepath.Join(base, "card_after_training.png"),
		})
	}

	seen := make(map[string]struct{}, len(candidates))
	paths := make([]string, 0, len(candidates))
	for _, item := range candidates {
		path := resolveCardOriginalImagePath(helper, region, item.primary, item.fallback)
		if strings.TrimSpace(path) == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
	}
	return paths
}

func resolveCardOriginalImagePath(helper *assets.AssetHelper, region renderregion.Value, relPaths ...string) string {
	candidates := make([]string, 0, len(relPaths)*3)
	for _, rel := range relPaths {
		cleanRel := filepath.ToSlash(strings.TrimSpace(rel))
		if cleanRel == "" {
			continue
		}
		for _, base := range assets.RegionAssetDirs(region.String()) {
			candidates = append(candidates, filepath.ToSlash(filepath.Join(base, cleanRel)))
		}
		// Keep compatibility with older local layouts used by tests and ad-hoc assets.
		candidates = append(candidates, cleanRel)
	}
	if helper != nil {
		if local := helper.FirstExisting(candidates...); local != "" {
			return filepath.ToSlash(local)
		}
	}
	return assets.ResolveRegionAssetPath(helper, region.String(), relPaths...)
}
