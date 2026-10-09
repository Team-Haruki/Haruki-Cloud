package stamp

import (
	"context"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"haruki-cloud/internal/i18n"
	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	regionsource "haruki-cloud/internal/pjsk/render/source"
	"haruki-cloud/utils/usererror"
)

func NewController(defaultSource DataSource, drawingClient *drawing.HarukiDrawingClient, assetHelper *assets.AssetHelper) *Controller {
	if assetHelper == nil {
		assetHelper = assets.NewAssetHelper("", nil)
	}
	ctrl := &Controller{
		sources: regionsource.NewRegistry[DataSource](renderregion.JP),
		drawing: drawingClient,
		assets:  assetHelper,
	}
	ctrl.RegisterSource(defaultSource)
	return ctrl
}

func (c *Controller) RegisterSource(src DataSource) {
	c.sources.RegisterSource(src)
}

func (c *Controller) WithContext(ctx context.Context) *Controller {
	if c == nil {
		return nil
	}
	clone := *c
	clone.requestCtx = ctx
	clone.drawing = c.drawing.WithContext(ctx)
	clone.assets = c.assets.WithContext(ctx)
	clone.sources = regionsource.NewRegistry[DataSource](c.sources.ResolveRegion(renderregion.Unknown))
	for _, source := range c.sources.OrderedSources() {
		if contextual, ok := any(source).(contextualDataSource); ok {
			clone.sources.RegisterSource(contextual.WithContext(ctx))
			continue
		}
		clone.sources.RegisterSource(source)
	}
	return &clone
}

func (c *Controller) BuildStampListRequest(query ListQuery) (*drawing.StampListRequest, error) {
	requests, err := c.BuildStampListRequests(query)
	if err != nil {
		return nil, err
	}
	return requests[0], nil
}

func (c *Controller) BuildStampListRequests(query ListQuery) ([]*drawing.StampListRequest, error) {
	finishBuild := commandtrace.MeasureOperation(c.requestCtx, "payload.build")
	defer finishBuild()
	items, prompt, err := c.collectStampItems(query)
	if err != nil {
		return nil, err
	}

	totalPages := int(math.Ceil(float64(len(items)) / float64(stampPageSize)))
	if totalPages <= 0 {
		totalPages = 1
	}

	page := query.Page
	if page <= 0 {
		page = 1
	}
	if page > totalPages {
		return nil, usererror.OutOfRange(i18n.M("common.param_name.page"), 1, totalPages)
	}

	buildPage := func(pageNum int) *drawing.StampListRequest {
		start := (pageNum - 1) * stampPageSize
		end := start + stampPageSize
		if end > len(items) {
			end = len(items)
		}
		pageItems := slices.Clone(items[start:end])
		return &drawing.StampListRequest{
			PromptMessage: &prompt,
			PageMessage:   new(i18n.PageLabel(pageNum, totalPages).String()),
			Stamps:        pageItems,
		}
	}

	if query.All {
		requests := make([]*drawing.StampListRequest, 0, totalPages)
		for pageNum := 1; pageNum <= totalPages; pageNum++ {
			requests = append(requests, buildPage(pageNum))
		}
		return requests, nil
	}

	return []*drawing.StampListRequest{buildPage(page)}, nil
}

func (c *Controller) RenderStampList(query ListQuery) ([]byte, error) {
	image, err := c.RenderStampListImage(query)
	if err != nil {
		return nil, err
	}
	return image.Bytes(c.requestCtx)
}

func (c *Controller) RenderStampListImage(query ListQuery) (drawing.ImageResult, error) {
	if c.drawing == nil {
		return drawing.ImageResult{}, drawing.ErrNotConfigured
	}
	req, err := c.BuildStampListRequest(query)
	if err != nil {
		return drawing.ImageResult{}, err
	}
	return c.drawing.GenerateStampListImage(req)
}

func (c *Controller) RenderStampListPages(query ListQuery) ([][]byte, error) {
	images, err := c.RenderStampListPagesImage(query)
	if err != nil {
		return nil, err
	}
	results := make([][]byte, 0, len(images))
	for _, image := range images {
		data, err := image.Bytes(c.requestCtx)
		if err != nil {
			return nil, err
		}
		results = append(results, data)
	}
	return results, nil
}

func (c *Controller) RenderStampListPagesImage(query ListQuery) ([]drawing.ImageResult, error) {
	if c.drawing == nil {
		return nil, drawing.ErrNotConfigured
	}
	requests, err := c.BuildStampListRequests(query)
	if err != nil {
		return nil, err
	}
	results := make([]drawing.ImageResult, 0, len(requests))
	for _, req := range requests {
		data, renderErr := c.drawing.GenerateStampListImage(req)
		if renderErr != nil {
			return nil, renderErr
		}
		results = append(results, data)
	}
	return results, nil
}

func (c *Controller) collectStampItems(query ListQuery) ([]drawing.StampData, string, error) {
	query.Region = c.sources.ResolveRegion(query.Region)
	src, ok := c.sources.SourceForRegion(query.Region)
	if !ok {
		return nil, "", usererror.Misconfigured(errors.New("stamp data source not configured"))
	}

	stamps, err := src.GetStamps()
	if err != nil {
		return nil, "", err
	}
	if len(stamps) == 0 {
		return nil, "", fmt.Errorf("no stamp data available")
	}

	filter := positiveStampIDSet(query.IDs)
	characterFilter := positiveStampIDSet(query.CharacterIDs)
	items := make([]drawing.StampData, 0, len(stamps))
	for _, item := range stamps {
		if !stampMatchesFilters(item, filter, characterFilter) {
			continue
		}
		imagePath, ok := c.resolveStampImage(item, query.Region)
		if !ok {
			continue
		}
		items = append(items, drawing.StampData{
			ID:        item.ID,
			ImagePath: imagePath,
			TextColor: []int{200, 0, 0, 255},
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	if query.Limit > 0 && len(items) > query.Limit {
		items = items[:query.Limit]
	}
	if len(items) == 0 {
		return nil, "", usererror.New(usererror.CodeNotFound, i18n.M("stamp.no_match"))
	}

	return items, stampPrompt(query.PromptMessage), nil
}

func positiveStampIDSet(ids []int) map[int]struct{} {
	result := make(map[int]struct{}, len(ids))
	for _, id := range ids {
		if id > 0 {
			result[id] = struct{}{}
		}
	}
	return result
}

func stampMatchesFilters(item masterdata.Stamp, ids, characterIDs map[int]struct{}) bool {
	if len(ids) > 0 {
		if _, ok := ids[item.ID]; !ok {
			return false
		}
	}
	if len(characterIDs) == 0 {
		return true
	}
	_, first := characterIDs[item.CharacterID]
	_, second := characterIDs[item.CharacterID2]
	return first || second
}

func stampPrompt(value string) string {
	if prompt := strings.TrimSpace(value); prompt != "" {
		return prompt
	}
	return i18n.T("render_stamp.prompt")
}

func (c *Controller) resolveStampImage(item masterdata.Stamp, region renderregion.Value) (string, bool) {
	relCandidates := []string{
		filepath.Join("stamp", item.AssetBundleName, item.AssetBundleName+".png"),
	}
	candidates := slices.Clone(relCandidates)
	for _, rel := range relCandidates {
		for _, mode := range []string{assets.RegionAssetStartApp, assets.RegionAssetOnDemand} {
			drawingPath := filepath.ToSlash(filepath.Join(assets.RegionAssetDirByMode(region.String(), mode), rel))
			candidates = append(candidates, drawingPath, strings.TrimPrefix(drawingPath, "asset/"))
		}
	}
	if c.assets != nil {
		if existing := c.assets.FirstExisting(candidates...); existing != "" {
			return c.makeRelativeAsset(existing), true
		}
	}
	// Fallback when no local asset root holds the stamp: the Drawing side
	// resolves the relative path against its own asset mirror (E1).
	return assets.ResolveRegionAssetPath(c.assets, region.String(), relCandidates...), true
}

func (c *Controller) makeRelativeAsset(target string) string {
	if c.assets == nil {
		return normalizeStampRelativeAsset(target)
	}
	relative := filepath.ToSlash(strings.TrimPrefix(c.assets.RelativePath(target), "./"))
	if relative == "" {
		return normalizeStampRelativeAsset(target)
	}
	return normalizeStampRelativeAsset(relative)
}

func normalizeStampRelativeAsset(path string) string {
	clean := filepath.ToSlash(strings.TrimPrefix(filepath.Clean(path), "./"))
	if clean == "." {
		return ""
	}
	for _, region := range []string{"jp", "cn", "tw", "kr", "en"} {
		prefix := region + "-assets/"
		if strings.HasPrefix(clean, prefix) {
			return filepath.ToSlash(filepath.Join("asset", clean))
		}
	}
	return clean
}
