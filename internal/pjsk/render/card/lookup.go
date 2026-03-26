package card

import (
	"fmt"
	"path/filepath"
	"strings"

	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

type ImageResult struct {
	Card  *masterdata.Card
	Paths []string
}

func (c *Controller) ResolveCardImages(query Query) (*ImageResult, error) {
	if c == nil {
		return nil, fmt.Errorf("card controller is not configured")
	}

	_, source, _, err := c.resolveBuilder(query.Region)
	if err != nil {
		return nil, err
	}
	searcher := NewSearchService(source, NewParser(c.nicknames))
	cardInfo, err := searcher.Search(query.Query)
	if err != nil {
		return nil, fmt.Errorf("failed to search card: %w", err)
	}

	paths := resolveCardOriginalImagePaths(c.assets, cardInfo)
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

func resolveCardOriginalImagePaths(helper *assets.AssetHelper, card *masterdata.Card) []string {
	if card == nil || strings.TrimSpace(card.AssetBundleName) == "" {
		return nil
	}

	base := filepath.Join("character", "member", card.AssetBundleName)
	ripBase := filepath.Join("character", "member", card.AssetBundleName+"_rip")

	type candidate struct {
		primary  string
		fallback string
	}
	candidates := []candidate{
		{
			primary:  filepath.Join(base, "card_normal.png"),
			fallback: filepath.Join(ripBase, "card_normal.png"),
		},
	}
	if card.CardRarityType == "rarity_3" || card.CardRarityType == "rarity_4" {
		candidates = append(candidates, candidate{
			primary:  filepath.Join(base, "card_after_training.png"),
			fallback: filepath.Join(ripBase, "card_after_training.png"),
		})
	}

	seen := make(map[string]struct{}, len(candidates))
	paths := make([]string, 0, len(candidates))
	for _, item := range candidates {
		path := ""
		if helper != nil {
			if local := helper.FirstExisting(item.primary, item.fallback); local != "" {
				path = filepath.ToSlash(local)
			} else {
				path = assets.ResolveAssetPath(helper, "", item.primary)
			}
		} else {
			path = filepath.ToSlash(item.primary)
		}
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
