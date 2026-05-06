package gacha

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

const gachaEndPaddingMillis = int64(time.Minute / time.Millisecond)
const defaultGachaListPageSize = 100

var (
	gachaRereleasePrefixes = []string{"[it's back]", "[재등장]", "[复刻]", "[復刻]"}
	gachaRecallPrefixes    = []string{"[回响]"}
)

func NewBuilder(source DataSource, assetHelper *assets.AssetHelper) *Builder {
	return &Builder{
		source: source,
		assets: assetHelper,
	}
}

func (b *Builder) BuildGachaListRequest(query ListQuery) (*drawing.GachaListRequest, error) {
	page := query.Page
	if page < 0 {
		page = 0
	}
	pageSize := query.PageSize
	if pageSize <= 0 {
		pageSize = defaultGachaListPageSize
	}

	now := time.Now()
	var filtered []*masterdata.Gacha
	all := b.source.GetGachas()

	cardFilter := query.CardID
	keyword := strings.ToLower(strings.TrimSpace(query.Keyword))

	for _, item := range all {
		if query.Year > 0 && time.UnixMilli(item.StartAt).Year() != query.Year {
			continue
		}
		if !query.IncludeFuture && time.UnixMilli(item.StartAt).After(now) {
			continue
		}
		if !query.IncludePast && time.UnixMilli(item.EndAt).Before(now) {
			continue
		}
		if cardFilter > 0 && !gachaContainsCard(item, cardFilter) {
			continue
		}
		if keyword != "" && !strings.Contains(strings.ToLower(item.Name), keyword) {
			continue
		}
		if query.IsRerelease && !hasAnyPrefixFold(item.Name, gachaRereleasePrefixes) {
			continue
		}
		if query.IsRecall && !hasAnyPrefixFold(item.Name, gachaRecallPrefixes) {
			continue
		}
		if query.OnlyCurrent {
			if time.UnixMilli(item.StartAt).After(now) || time.UnixMilli(item.EndAt).Before(now) {
				continue
			}
		}
		filtered = append(filtered, item)
	}

	if len(filtered) == 0 {
		return nil, fmt.Errorf("no gacha data matched filters")
	}

	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].StartAt == filtered[j].StartAt {
			return filtered[i].ID < filtered[j].ID
		}
		return filtered[i].StartAt < filtered[j].StartAt
	})

	region := query.Region
	if region.IsZero() {
		region = b.source.DefaultRegion()
	}

	briefs := make([]drawing.GachaBrief, 0, len(filtered))
	logos := make(map[int]string, len(filtered))
	banners := make(map[int]string, len(filtered))
	for _, item := range filtered {
		briefs = append(briefs, drawing.GachaBrief{
			ID:        item.ID,
			Name:      item.Name,
			GachaType: item.GachaType,
			StartAt:   item.StartAt,
			EndAt:     item.EndAt,
			AssetName: item.AssetBundleName,
		})
		logos[item.ID] = b.buildGachaLogoPath(item, region)
		banners[item.ID] = b.buildGachaBannerPath(item, region)
	}

	totalPages := 1
	if len(briefs) > 0 {
		totalPages = (len(briefs) + pageSize - 1) / pageSize
	}
	currentPage := page
	if currentPage <= 0 {
		currentPage = totalPages
	}
	if currentPage > totalPages {
		currentPage = totalPages
	}
	startIndex := (currentPage - 1) * pageSize
	endIndex := startIndex + pageSize
	if endIndex > len(briefs) {
		endIndex = len(briefs)
	}
	briefs = briefs[startIndex:endIndex]

	pagedLogos := make(map[int]string, len(briefs))
	pagedBanners := make(map[int]string, len(briefs))
	for _, brief := range briefs {
		pagedLogos[brief.ID] = logos[brief.ID]
		pagedBanners[brief.ID] = banners[brief.ID]
	}

	return &drawing.GachaListRequest{
		Gachas:       briefs,
		PageSize:     pageSize,
		Region:       region.String(),
		GachaLogos:   pagedLogos,
		GachaBanners: pagedBanners,
		CurrentPage:  currentPage,
		TotalPage:    totalPages,
		PrePaginated: true,
		Filter: drawing.GachaFilter{
			Page: currentPage,
		},
	}, nil
}

func hasAnyPrefixFold(text string, prefixes []string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	for _, prefix := range prefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}
