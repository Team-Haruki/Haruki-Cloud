package card

import (
	"fmt"
	"strings"

	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/common"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/region"
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

	type candidate struct {
		fileName string
	}
	var candidates []candidate

	if !onlyHasAfterTrainingCard(card) {
		candidates = append(candidates, candidate{
			fileName: "card_normal.png",
		})
	}
	if card.CardRarityType == "rarity_3" || card.CardRarityType == "rarity_4" {
		candidates = append(candidates, candidate{
			fileName: "card_after_training.png",
		})
	}

	seen := make(map[string]struct{}, len(candidates))
	paths := make([]string, 0, len(candidates))
	for _, item := range candidates {
		path := common.ResolveCardMemberImagePath(helper, region, card.AssetBundleName, item.fileName)
		if strings.TrimSpace(path) == "" {
			continue
		}
		// Resolve to absolute path when the file exists locally so that callers
		// using os.ReadFile (card-image mode, no CDN) receive a usable path.
		if helper != nil {
			if resolved := helper.FirstExisting(path); resolved != "" {
				path = resolved
			}
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
	}
	return paths
}
