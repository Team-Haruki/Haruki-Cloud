package gacha

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
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
	page, pageSize := normalizeGachaListPage(query.Page, query.PageSize)
	filtered := filterGachaListItems(b.source.GetGachas(), query, time.Now())
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

	briefs, logos, banners := b.buildGachaListItems(filtered, region)
	briefs, currentPage, totalPages := paginateGachaList(briefs, pageSize, page)
	pagedLogos, pagedBanners := selectGachaListAssets(briefs, logos, banners)

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

func normalizeGachaListPage(page, pageSize int) (int, int) {
	if page < 0 {
		page = 0
	}
	if pageSize <= 0 {
		pageSize = defaultGachaListPageSize
	}
	return page, pageSize
}

func filterGachaListItems(items []*masterdata.Gacha, query ListQuery, now time.Time) []*masterdata.Gacha {
	filtered := make([]*masterdata.Gacha, 0, len(items))
	keyword := strings.ToLower(strings.TrimSpace(query.Keyword))
	for _, item := range items {
		if gachaMatchesListQuery(item, query, keyword, now) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func gachaMatchesListQuery(item *masterdata.Gacha, query ListQuery, keyword string, now time.Time) bool {
	startAt := time.UnixMilli(item.StartAt)
	endAt := time.UnixMilli(item.EndAt)
	if !gachaMatchesTimeWindow(query, startAt, endAt, now) {
		return false
	}
	if query.CardID > 0 && !gachaContainsCard(item, query.CardID) {
		return false
	}
	if keyword != "" && !strings.Contains(strings.ToLower(item.Name), keyword) {
		return false
	}
	if query.IsRerelease && !hasAnyPrefixFold(item.Name, gachaRereleasePrefixes) {
		return false
	}
	if query.IsRecall && !hasAnyPrefixFold(item.Name, gachaRecallPrefixes) {
		return false
	}
	return true
}

func gachaMatchesTimeWindow(query ListQuery, startAt, endAt, now time.Time) bool {
	if query.Year > 0 && startAt.Year() != query.Year {
		return false
	}
	if !query.IncludeFuture && startAt.After(now) {
		return false
	}
	if !query.IncludePast && endAt.Before(now) {
		return false
	}
	return !query.OnlyCurrent || (!startAt.After(now) && !endAt.Before(now))
}

func (b *Builder) buildGachaListItems(items []*masterdata.Gacha, region renderregion.Value) ([]drawing.GachaBrief, map[int]string, map[int]string) {
	briefs := make([]drawing.GachaBrief, 0, len(items))
	logos := make(map[int]string, len(items))
	banners := make(map[int]string, len(items))
	for _, item := range items {
		briefs = append(briefs, drawing.GachaBrief{
			ID: item.ID, Name: item.Name, GachaType: item.GachaType,
			StartAt: item.StartAt, EndAt: item.EndAt, AssetName: item.AssetBundleName,
		})
		logos[item.ID] = b.buildGachaLogoPath(item, region)
		banners[item.ID] = b.buildGachaBannerPath(item, region)
	}
	return briefs, logos, banners
}

func paginateGachaList(briefs []drawing.GachaBrief, pageSize, page int) ([]drawing.GachaBrief, int, int) {
	totalPages := max(1, (len(briefs)+pageSize-1)/pageSize)
	currentPage := page
	if currentPage <= 0 {
		currentPage = totalPages
	}
	currentPage = min(currentPage, totalPages)
	startIndex := (currentPage - 1) * pageSize
	endIndex := min(startIndex+pageSize, len(briefs))
	return briefs[startIndex:endIndex], currentPage, totalPages
}

func selectGachaListAssets(briefs []drawing.GachaBrief, logos, banners map[int]string) (map[int]string, map[int]string) {
	pagedLogos := make(map[int]string, len(briefs))
	pagedBanners := make(map[int]string, len(briefs))
	for _, brief := range briefs {
		pagedLogos[brief.ID] = logos[brief.ID]
		pagedBanners[brief.ID] = banners[brief.ID]
	}
	return pagedLogos, pagedBanners
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
