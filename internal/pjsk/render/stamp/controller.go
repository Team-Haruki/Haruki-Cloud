package stamp

import (
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strings"

	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	regionsource "haruki-cloud/internal/pjsk/render/source"
	"haruki-cloud/utils/drawing"
)

type Controller struct {
	sources *regionsource.Registry[DataSource]
	drawing *drawing.HarukiDrawingClient
	assets  *assets.AssetHelper
}

const stampPageSize = 25

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

func (c *Controller) BuildStampListRequest(query ListQuery) (*drawing.StampListRequest, error) {
	requests, err := c.BuildStampListRequests(query)
	if err != nil {
		return nil, err
	}
	return requests[0], nil
}

func (c *Controller) BuildStampListRequests(query ListQuery) ([]*drawing.StampListRequest, error) {
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
		return nil, fmt.Errorf("页数错误，当前仅有%d页", totalPages)
	}

	buildPage := func(pageNum int) *drawing.StampListRequest {
		start := (pageNum - 1) * stampPageSize
		end := start + stampPageSize
		if end > len(items) {
			end = len(items)
		}
		pageItems := append([]drawing.StampData(nil), items[start:end]...)
		pageText := fmt.Sprintf("第 %d / %d 页", pageNum, totalPages)
		return &drawing.StampListRequest{
			PromptMessage: &prompt,
			PageMessage:   &pageText,
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
	if c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	req, err := c.BuildStampListRequest(query)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateStampList(req)
}

func (c *Controller) RenderStampListPages(query ListQuery) ([][]byte, error) {
	if c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	requests, err := c.BuildStampListRequests(query)
	if err != nil {
		return nil, err
	}
	results := make([][]byte, 0, len(requests))
	for _, req := range requests {
		data, renderErr := c.drawing.GenerateStampList(req)
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
		return nil, "", fmt.Errorf("stamp data source not configured")
	}

	stamps, err := src.GetStamps()
	if err != nil {
		return nil, "", err
	}
	if len(stamps) == 0 {
		return nil, "", fmt.Errorf("no stamp data available")
	}

	filter := make(map[int]struct{}, len(query.IDs))
	for _, id := range query.IDs {
		if id > 0 {
			filter[id] = struct{}{}
		}
	}

	items := make([]drawing.StampData, 0, len(stamps))
	for _, item := range stamps {
		if len(filter) > 0 {
			if _, ok := filter[item.ID]; !ok {
				continue
			}
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
		return nil, "", fmt.Errorf("no stamps matched the query")
	}

	prompt := strings.TrimSpace(query.PromptMessage)
	if prompt == "" {
		prompt = strings.Join([]string{
			`发送"/stamp 序号"获取单张表情`,
			`发送"/stamp 序号 序号"获取多张表情`,
			`发送"/stamp page 2"查看指定页`,
			`发送"/stamp all"返回全部页`,
		}, "\n")
	}

	return items, prompt, nil
}

func (c *Controller) resolveStampImage(item masterdata.Stamp, region renderregion.Value) (string, bool) {
	relCandidates := []string{
		filepath.Join("stamp", item.AssetBundleName, item.AssetBundleName+".png"),
		filepath.Join("stamp", item.AssetBundleName+"_rip", item.AssetBundleName+".png"),
	}
	expanded := make([]string, 0, len(relCandidates)*2)
	for _, rel := range relCandidates {
		expanded = append(expanded,
			filepath.ToSlash(filepath.Join(assets.RegionAssetDirByMode(region.String(), assets.RegionAssetStartApp), rel)),
			filepath.ToSlash(filepath.Join(assets.RegionAssetDirByMode(region.String(), assets.RegionAssetOnDemand), rel)),
		)
	}
	if c.assets != nil {
		if existing := c.assets.FirstExisting(expanded...); existing != "" {
			return filepath.ToSlash(existing), true
		}
		primary := c.assets.Primary()
		if primary != "" && primary != "." {
			return "", false
		}
	}
	// Runtime fallback when no local asset root is configured.
	return expanded[0], true
}
