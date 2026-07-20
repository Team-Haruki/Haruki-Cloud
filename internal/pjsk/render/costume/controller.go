package costume

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/filteralias"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	rendercard "haruki-cloud/internal/pjsk/render/card"
	"haruki-cloud/internal/pjsk/render/masterdata"
	regionsource "haruki-cloud/internal/pjsk/render/source"
	"haruki-cloud/utils/logger"
)

type Controller struct {
	sources   *regionsource.Registry[DataSource]
	drawing   *drawing.HarukiDrawingClient
	assets    *assets.AssetHelper
	preview3D *Preview3DService
	ctx       context.Context
}

var costumePartOrder = []string{"body", "head", "hair"}
var costumePreview3DLogger = logger.NewLoggerFromGlobal("Costume3DPreview")
var costumeUnitAliases = buildCostumeUnitAliases()

var mikuCharacter3DIDsByUnit = map[string]int{
	"piapro":         21,
	"idol":           22,
	"light_sound":    23,
	"street":         24,
	"theme_park":     25,
	"school_refusal": 26,
}

func NewController(defaultSource DataSource, drawingClient *drawing.HarukiDrawingClient, assetHelper *assets.AssetHelper) *Controller {
	if assetHelper == nil {
		assetHelper = assets.NewAssetHelper("", nil)
	}
	controller := &Controller{
		sources: regionsource.NewRegistry[DataSource](renderregion.JP),
		drawing: drawingClient,
		assets:  assetHelper,
		ctx:     context.Background(),
	}
	controller.RegisterSource(defaultSource)
	return controller
}

func (c *Controller) RegisterSource(source DataSource) {
	if c == nil {
		return
	}
	c.sources.RegisterSource(source)
}

func (c *Controller) Set3DPreviewConfig(cfg Preview3DConfig) {
	if c == nil {
		return
	}
	if strings.TrimSpace(cfg.StaticOutputDir) == "" {
		cfg.StaticOutputDir = c.default3DPreviewStaticOutputDir(cfg.StaticRelativeDir)
	}
	c.preview3D = NewPreview3DService(cfg)
}

func (c *Controller) default3DPreviewStaticOutputDir(staticRelativeDir string) string {
	if c == nil || c.assets == nil {
		return ""
	}
	root := strings.TrimSpace(c.assets.Primary())
	if root == "" || strings.HasPrefix(strings.ToLower(root), "http://") || strings.HasPrefix(strings.ToLower(root), "https://") {
		return ""
	}
	if filepath.Clean(root) == "." {
		return ""
	}
	rel := strings.Trim(strings.TrimSpace(staticRelativeDir), "/")
	if rel == "" {
		rel = defaultPreview3DStaticRelativeDir
	}
	return filepath.Join(root, filepath.FromSlash(rel))
}

func (c *Controller) WithContext(ctx context.Context) *Controller {
	if c == nil {
		return nil
	}
	clone := *c
	clone.drawing = c.drawing.WithContext(ctx)
	clone.preview3D = c.preview3D
	clone.ctx = ctx
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

func (c *Controller) BuildCostumeListRequest(query ListQuery) (*drawing.CostumeListRequest, error) {
	region, source, err := c.resolveSource(query.Region)
	if err != nil {
		return nil, err
	}
	parsed, err := normalizeListQuery(query)
	if err != nil {
		return nil, err
	}
	filter, err := c.buildFilter(parsed)
	if err != nil {
		return nil, err
	}
	if parsed.Character3DID > 0 && (parsed.PartType == "head" || parsed.PartType == "hair") {
		filter.CharacterID = 0
		filter.CharacterIDs = nil
	}
	filter.ColorID = 1

	items, err := source.FilterCostumes(filter)
	if err != nil {
		return nil, err
	}
	var hairIDs map[int]int
	var accessoryItems []costumeAccessoryListItem
	accessoryListMode := parsed.PartType == "head"
	mixedAccessoryListMode := parsed.PartType == "" && c.preview3D != nil
	if parsed.PartType == "hair" && parsed.Character3DID > 0 {
		if c.preview3D == nil {
			return nil, fmt.Errorf("3d preview service is not configured")
		}
		hairIDs, err = c.preview3D.HairIDsForRole(c.ctx, region.String(), parsed.Character3DID)
		if err != nil {
			return nil, err
		}
		filtered := items[:0]
		for _, item := range items {
			if item != nil && hairIDs[item.ID] > 0 {
				filtered = append(filtered, item)
			}
		}
		items = filtered
		sort.Slice(items, func(i, j int) bool { return hairIDs[items[i].ID] < hairIDs[items[j].ID] })
	} else if parsed.PartType == "head" {
		if c.preview3D == nil {
			return nil, fmt.Errorf("3d preview service is not configured")
		}
		catalog, err := c.preview3D.AccessoryCatalog(c.ctx, region.String(), parsed.Character3DID)
		if err != nil {
			return nil, err
		}
		accessoryItems = buildCostumeAccessoryListItems(source, items, catalog, parsed.Character3DID > 0)
	} else if mixedAccessoryListMode {
		var headItems []*masterdata.Costume3d
		var hairItems []*masterdata.Costume3d
		baseItems := make([]*masterdata.Costume3d, 0, len(items))
		for _, item := range items {
			if item == nil {
				continue
			}
			switch item.PartType {
			case "head":
				headItems = append(headItems, item)
			case "hair":
				hairItems = append(hairItems, item)
			default:
				baseItems = append(baseItems, item)
			}
		}
		if parsed.Character3DID > 0 {
			componentFilter := filter
			componentFilter.CharacterID = 0
			componentFilter.CharacterIDs = nil
			componentFilter.PartType = "head"
			headItems, err = source.FilterCostumes(componentFilter)
			if err != nil {
				return nil, err
			}
			componentFilter.PartType = "hair"
			hairItems, err = source.FilterCostumes(componentFilter)
			if err != nil {
				return nil, err
			}
			hairIDs, err = c.preview3D.HairIDsForRole(c.ctx, region.String(), parsed.Character3DID)
			if err != nil {
				return nil, err
			}
		}
		catalog, catalogErr := c.preview3D.AccessoryCatalog(c.ctx, region.String(), parsed.Character3DID)
		if catalogErr != nil {
			return nil, catalogErr
		}
		accessoryItems = make([]costumeAccessoryListItem, 0, len(baseItems)+len(hairItems)+len(catalog))
		for _, item := range baseItems {
			accessoryItems = append(accessoryItems, costumeAccessoryListItem{costume: item})
		}
		for _, item := range hairItems {
			if parsed.Character3DID > 0 && hairIDs[item.ID] <= 0 {
				continue
			}
			accessoryItems = append(accessoryItems, costumeAccessoryListItem{costume: item})
		}
		accessoryItems = append(accessoryItems, buildCostumeAccessoryListItems(source, headItems, catalog, parsed.Character3DID > 0)...)
		sortCostumeLogicalListItems(accessoryItems)
	} else {
		sortCostumesForDisplay(items)
	}
	if len(parsed.AccessoryIDs) > 0 {
		if !accessoryListMode {
			return nil, fmt.Errorf("accessory id filter requires an accessory list")
		}
		allowed := make(map[int]struct{}, len(parsed.AccessoryIDs))
		for _, accessoryID := range parsed.AccessoryIDs {
			if accessoryID > 0 {
				allowed[accessoryID] = struct{}{}
			}
		}
		filtered := accessoryItems[:0]
		for _, item := range accessoryItems {
			if _, ok := allowed[item.accessoryID]; ok {
				filtered = append(filtered, item)
			}
		}
		accessoryItems = filtered
	}

	pageSize := parsed.PageSize
	if pageSize <= 0 {
		pageSize = DefaultPageSize
	}
	if pageSize > MaxPageSize {
		pageSize = MaxPageSize
	}
	page := parsed.Page
	if page <= 0 {
		page = 1
	}
	total := len(items)
	var pageItems []*masterdata.Costume3d
	var pageAccessoryItems []costumeAccessoryListItem
	var totalPages int
	if accessoryListMode {
		total = len(accessoryItems)
		pageAccessoryItems, totalPages = paginateCostumeAccessoryListItems(accessoryItems, pageSize, page)
		pageItems = make([]*masterdata.Costume3d, 0, len(pageAccessoryItems))
		for _, item := range pageAccessoryItems {
			pageItems = append(pageItems, item.costume)
		}
	} else if mixedAccessoryListMode {
		total = len(accessoryItems)
		pageAccessoryItems, totalPages = paginateCostumeLogicalListItems(accessoryItems, parsed, pageSize, page)
		pageItems = make([]*masterdata.Costume3d, 0, len(pageAccessoryItems))
		for _, item := range pageAccessoryItems {
			pageItems = append(pageItems, item.costume)
		}
	} else {
		pageItems, totalPages = paginateCostumeListItems(items, parsed, pageSize, page)
	}
	if page > totalPages {
		page = totalPages
	}

	sourceCards, err := c.sourceCardsForCostumes(source, pageItems)
	if err != nil {
		return nil, err
	}
	costumes := make([]drawing.CostumeBasic, 0, len(pageItems))
	for index, item := range pageItems {
		basic := c.buildCostumeBasic(region, source, item, nil, sourceCards)
		if hairIDs[item.ID] > 0 {
			basic.HairID = hairIDs[item.ID]
			basic.Character3DID = parsed.Character3DID
			basic.Character3DIDs = []int{parsed.Character3DID}
			c.apply3DRoleToCostumeBasic(source, &basic, parsed.Character3DID)
		}
		if accessoryListMode || mixedAccessoryListMode {
			accessoryItem := pageAccessoryItems[index]
			if accessoryItem.accessoryID > 0 {
				basic.AccessoryID = accessoryItem.accessoryID
				basic.Character3DIDs = append([]int(nil), accessoryItem.character3DIDs...)
			}
			if parsed.Character3DID > 0 {
				basic.Character3DID = parsed.Character3DID
				basic.Character3DIDs = []int{parsed.Character3DID}
				c.apply3DRoleToCostumeBasic(source, &basic, parsed.Character3DID)
			}
		}
		costumes = append(costumes, basic)
	}

	title := buildListTitle(parsed)
	return &drawing.CostumeListRequest{
		Region:      region.String(),
		Title:       title,
		Page:        page,
		PageSize:    pageSize,
		Total:       total,
		TotalPages:  totalPages,
		FilterLabel: buildFilterLabel(parsed),
		Costumes:    costumes,
	}, nil
}

func paginateCostumeListItems(items []*masterdata.Costume3d, query ListQuery, pageSize int, page int) ([]*masterdata.Costume3d, int) {
	totalPages := 1
	if len(items) > 0 {
		totalPages = (len(items) + pageSize - 1) / pageSize
	}
	if page > totalPages {
		page = totalPages
	}
	if page <= 0 {
		page = 1
	}
	if shouldBalanceCostumeListByPart(query) {
		return paginateCostumeListItemsByPart(items, pageSize, page), totalPages
	}
	start := (page - 1) * pageSize
	if start > len(items) {
		start = len(items)
	}
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}
	return items[start:end], totalPages
}

type costumeAccessoryListItem struct {
	costume        *masterdata.Costume3d
	accessoryID    int
	character3DIDs []int
}

func buildCostumeAccessoryListItems(source DataSource, items []*masterdata.Costume3d, catalog []preview3DAccessoryCatalogEntry, useRegistryRepresentative bool) []costumeAccessoryListItem {
	byRawID := make(map[int]*masterdata.Costume3d, len(items))
	for _, item := range items {
		if item != nil {
			byRawID[item.ID] = item
		}
	}
	result := make([]costumeAccessoryListItem, 0, len(catalog))
	for _, entry := range catalog {
		var matched *masterdata.Costume3d
		for _, rawID := range entry.Costume3DIDs {
			if candidate := byRawID[rawID]; candidate != nil {
				matched = candidate
				break
			}
		}
		if matched == nil {
			continue
		}
		representative := byRawID[entry.RepresentativeCostume3DID]
		if representative == nil && useRegistryRepresentative && source != nil {
			representative, _ = source.GetCostumeByID(entry.RepresentativeCostume3DID)
		}
		if representative == nil {
			representative = matched
		}
		result = append(result, costumeAccessoryListItem{
			costume:        representative,
			accessoryID:    entry.AccessoryID,
			character3DIDs: append([]int(nil), entry.Character3DIDs...),
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].accessoryID < result[j].accessoryID })
	return result
}

func paginateCostumeAccessoryListItems(items []costumeAccessoryListItem, pageSize int, page int) ([]costumeAccessoryListItem, int) {
	totalPages := 1
	if len(items) > 0 {
		totalPages = (len(items) + pageSize - 1) / pageSize
	}
	if page > totalPages {
		page = totalPages
	}
	if page <= 0 {
		page = 1
	}
	start := (page - 1) * pageSize
	end := min(start+pageSize, len(items))
	return items[start:end], totalPages
}

func sortCostumeLogicalListItems(items []costumeAccessoryListItem) {
	sort.SliceStable(items, func(i, j int) bool {
		left := items[i]
		right := items[j]
		leftTime := costumeSortTime(left.costume)
		rightTime := costumeSortTime(right.costume)
		if leftTime != rightTime {
			return leftTime > rightTime
		}
		if left.costume.Seq != right.costume.Seq {
			return left.costume.Seq > right.costume.Seq
		}
		if left.costume.ID != right.costume.ID {
			return left.costume.ID > right.costume.ID
		}
		return left.accessoryID < right.accessoryID
	})
}

func paginateCostumeLogicalListItems(items []costumeAccessoryListItem, query ListQuery, pageSize int, page int) ([]costumeAccessoryListItem, int) {
	totalPages := 1
	if len(items) > 0 {
		totalPages = (len(items) + pageSize - 1) / pageSize
	}
	if page > totalPages {
		page = totalPages
	}
	if page <= 0 {
		page = 1
	}
	if shouldBalanceCostumeListByPart(query) {
		return paginateCostumeLogicalListItemsByPart(items, pageSize, page), totalPages
	}
	start := (page - 1) * pageSize
	end := min(start+pageSize, len(items))
	return items[start:end], totalPages
}

func paginateCostumeLogicalListItemsByPart(items []costumeAccessoryListItem, pageSize int, page int) []costumeAccessoryListItem {
	groups := make(map[string][]costumeAccessoryListItem)
	order := make([]string, 0, len(costumePartOrder))
	seen := make(map[string]struct{})
	for _, item := range items {
		if item.costume == nil {
			continue
		}
		partType := strings.TrimSpace(item.costume.PartType)
		groups[partType] = append(groups[partType], item)
		if _, ok := seen[partType]; !ok {
			seen[partType] = struct{}{}
			order = append(order, partType)
		}
	}
	ordered := make([]string, 0, len(order))
	for _, partType := range costumePartOrder {
		if _, ok := groups[partType]; ok {
			ordered = append(ordered, partType)
		}
	}
	for _, partType := range order {
		if !containsString(ordered, partType) {
			ordered = append(ordered, partType)
		}
	}
	offsets := make(map[string]int, len(groups))
	var current []costumeAccessoryListItem
	for currentPage := 1; currentPage <= page; currentPage++ {
		current = make([]costumeAccessoryListItem, 0, pageSize)
		for len(current) < pageSize {
			added := false
			for _, partType := range ordered {
				group := groups[partType]
				if offsets[partType] >= len(group) {
					continue
				}
				current = append(current, group[offsets[partType]])
				offsets[partType]++
				added = true
				if len(current) >= pageSize {
					break
				}
			}
			if !added {
				break
			}
		}
	}
	return current
}

func shouldBalanceCostumeListByPart(query ListQuery) bool {
	return strings.TrimSpace(query.PartType) == "" && (strings.TrimSpace(query.Character) != "" || query.Character3DID > 0)
}

func paginateCostumeListItemsByPart(items []*masterdata.Costume3d, pageSize int, page int) []*masterdata.Costume3d {
	groups := make(map[string][]*masterdata.Costume3d)
	order := make([]string, 0, len(costumePartOrder))
	seen := make(map[string]struct{})
	for _, item := range items {
		if item == nil {
			continue
		}
		partType := strings.TrimSpace(item.PartType)
		groups[partType] = append(groups[partType], item)
		if _, ok := seen[partType]; !ok {
			seen[partType] = struct{}{}
			order = append(order, partType)
		}
	}
	ordered := make([]string, 0, len(order))
	for _, partType := range costumePartOrder {
		if _, ok := groups[partType]; ok {
			ordered = append(ordered, partType)
		}
	}
	for _, partType := range order {
		if !containsString(ordered, partType) {
			ordered = append(ordered, partType)
		}
	}
	offsets := make(map[string]int, len(groups))
	var current []*masterdata.Costume3d
	for currentPage := 1; currentPage <= page; currentPage++ {
		current = make([]*masterdata.Costume3d, 0, pageSize)
		for len(current) < pageSize {
			added := false
			for _, partType := range ordered {
				group := groups[partType]
				if offsets[partType] >= len(group) {
					continue
				}
				current = append(current, group[offsets[partType]])
				offsets[partType]++
				added = true
				if len(current) >= pageSize {
					break
				}
			}
			if !added {
				break
			}
		}
	}
	return current
}

func (c *Controller) RenderCostumeList(query ListQuery) ([]byte, error) {
	data, _, err := c.RenderCostumeListWithRequest(query)
	return data, err
}

func (c *Controller) RenderCostumeListWithRequest(query ListQuery) ([]byte, *drawing.CostumeListRequest, error) {
	if c == nil || c.drawing == nil {
		return nil, nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildCostumeListRequest(query)
	if err != nil {
		return nil, nil, err
	}
	data, err := c.drawing.GenerateCostumeList(payload)
	if err != nil {
		return nil, payload, err
	}
	return data, payload, nil
}

func (c *Controller) BuildCostumeDetailRequest(query Query) (*drawing.CostumeDetailRequest, error) {
	region, source, err := c.resolveSource(query.Region)
	if err != nil {
		return nil, err
	}
	costumeInfo, err := c.resolveCostumeInfo(region, source, query)
	if err != nil {
		return nil, err
	}
	if expectedPart, ok := normalizePartType(query.ExpectedPartType); ok && costumeInfo.PartType != expectedPart {
		return nil, fmt.Errorf("costume %d is %s, not %s", costumeInfo.ID, partTypeName(costumeInfo.PartType), partTypeName(expectedPart))
	}
	variants, err := source.GetCostumeVariants(costumeInfo.GroupID, costumeInfo.PartType, costumeInfo.CharacterID)
	if err != nil || len(variants) == 0 {
		variants = []*masterdata.Costume3d{costumeInfo}
	}
	sort.Slice(variants, func(i, j int) bool {
		if variants[i].ColorID == variants[j].ColorID {
			return variants[i].ID < variants[j].ID
		}
		return variants[i].ColorID < variants[j].ColorID
	})
	sourceCards, err := c.sourceCardsForCostumes(source, variants)
	if err != nil {
		return nil, err
	}
	costumeBasic := c.buildCostumeBasic(region, source, costumeInfo, variants, sourceCards)
	displayCharacterID := costumeInfo.CharacterID
	if query.Character3DID > 0 {
		costumeBasic.Character3DID = query.Character3DID
		costumeBasic.Character3DIDs = []int{query.Character3DID}
		c.apply3DRoleToCostumeBasic(source, &costumeBasic, query.Character3DID)
		displayCharacterID = costumeBasic.CharacterID
		switch costumeInfo.PartType {
		case "body":
			if c.preview3D == nil {
				break
			}
			outfitIDs, err := c.preview3D.OutfitIDsForRole(c.ctx, region.String(), query.Character3DID)
			if err != nil {
				return nil, err
			}
			outfitID := outfitIDs[costumeInfo.ID]
			if outfitID <= 0 {
				return nil, fmt.Errorf("服装 %d 不适用于角色ID %d", costumeInfo.ID, query.Character3DID)
			}
			if query.OutfitID > 0 && query.OutfitID != outfitID {
				return nil, fmt.Errorf("服装ID %d 不适用于角色ID %d", query.OutfitID, query.Character3DID)
			}
			costumeBasic.OutfitID = outfitID
		case "head":
			if c.preview3D == nil {
				return nil, fmt.Errorf("3d preview service is not configured")
			}
			accessoryIDs, err := c.preview3D.AccessoryIDsForRole(c.ctx, region.String(), query.Character3DID)
			if err != nil {
				return nil, err
			}
			resolvedIDs := accessoryIDs[costumeInfo.ID]
			if len(resolvedIDs) == 0 {
				return nil, fmt.Errorf("饰品 %d 不适用于角色ID %d", costumeInfo.ID, query.Character3DID)
			}
			if query.AccessoryID > 0 {
				if !slices.Contains(resolvedIDs, query.AccessoryID) {
					return nil, fmt.Errorf("饰品ID %d 不适用于角色ID %d", query.AccessoryID, query.Character3DID)
				}
				costumeBasic.AccessoryID = query.AccessoryID
				break
			}
			if len(resolvedIDs) > 1 {
				return nil, fmt.Errorf("饰品原始ID %d 对角色ID %d 对应多个独立饰品（ID：%s），请明确填写饰品ID", costumeInfo.ID, query.Character3DID, joinCostumeIDs(resolvedIDs))
			}
			costumeBasic.AccessoryID = resolvedIDs[0]
		case "hair":
			if c.preview3D == nil {
				return nil, fmt.Errorf("3d preview service is not configured")
			}
			hairIDs, err := c.preview3D.HairIDsForRole(c.ctx, region.String(), query.Character3DID)
			if err != nil {
				return nil, err
			}
			costumeBasic.HairID = hairIDs[costumeInfo.ID]
			if costumeBasic.HairID <= 0 {
				return nil, fmt.Errorf("发型 %d 不适用于角色ID %d", costumeInfo.ID, query.Character3DID)
			}
		}
	}
	character, _ := source.GetCharacterByID(displayCharacterID)
	return &drawing.CostumeDetailRequest{
		Region:            region.String(),
		Costume:           costumeBasic,
		CharacterIconPath: c.buildCharacterIconPath(displayCharacterID, characterUnit(character)),
		UnitLogoPath:      c.buildUnitLogoPath(characterUnit(character)),
	}, nil
}

func (c *Controller) resolve3DPreviewPath(region renderregion.Value, costumeInfo *masterdata.Costume3d, query Query) (string, error) {
	if c == nil || c.preview3D == nil || costumeInfo == nil {
		return "", nil
	}
	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return c.preview3D.ResolveQueryPreviewPath(ctx, region.String(), costumeInfo.ID, query)
}

func (c *Controller) ensure3DPreviewCapture(region renderregion.Value, costumeInfo *masterdata.Costume3d, query Query) error {
	if c == nil || c.preview3D == nil || costumeInfo == nil {
		return nil
	}
	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return c.preview3D.EnsureQueryPreviewCapture(ctx, region.String(), costumeInfo.ID, query)
}

func (c *Controller) RenderCostumeDetail(query Query) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	region, source, err := c.resolveSource(query.Region)
	if err != nil {
		return nil, err
	}
	costumeInfo, err := c.resolveCostumeInfo(region, source, query)
	if err != nil {
		return nil, err
	}
	if expectedPart, ok := normalizePartType(query.ExpectedPartType); ok && costumeInfo.PartType != expectedPart {
		return nil, fmt.Errorf("costume %d is %s, not %s", costumeInfo.ID, partTypeName(costumeInfo.PartType), partTypeName(expectedPart))
	}
	payload, err := c.BuildCostumeDetailRequest(query)
	if err != nil {
		return nil, err
	}
	cachePayload := c.costumeDetailCacheRequest(payload)
	return c.drawing.GenerateCostumeDetailWithPrepare(cachePayload, payload, func(prepared any) error {
		previewPath, err := c.resolve3DPreviewPath(region, costumeInfo, query)
		if err != nil {
			costumePreview3DLogger.Warnf("3d preview skipped: region=%s costume_id=%d err=%v", region.String(), costumeInfo.ID, err)
			return nil
		}
		if previewPath == "" {
			return nil
		}
		setCostumeDetailPreviewPath(prepared, previewPath)
		return c.ensure3DPreviewCapture(region, costumeInfo, query)
	})
}

func (c *Controller) RenderCostumeCombo(query ComboQuery) ([]byte, error) {
	if c == nil || c.preview3D == nil {
		return nil, fmt.Errorf("3d preview service is not configured")
	}
	parsed, err := parseComboQuery(query)
	if err != nil {
		return nil, err
	}
	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return c.preview3D.CaptureTemporaryCombo(ctx, parsed.Region, parsed)
}

func (c *Controller) resolveSource(regionText string) (renderregion.Value, DataSource, error) {
	if c == nil {
		return renderregion.Unknown, nil, fmt.Errorf("costume controller is not configured")
	}
	region := c.sources.ResolveRegion(renderregion.Normalize(regionText))
	source, ok := c.sources.SourceForRegion(region)
	if !ok || source == nil {
		return region, nil, fmt.Errorf("costume source is not configured for region %s", region)
	}
	return region, source, nil
}

func (c *Controller) resolveCostumeInfo(region renderregion.Value, source DataSource, query Query) (*masterdata.Costume3d, error) {
	if query.OutfitID > 0 || query.AccessoryID > 0 {
		return c.resolveNormalizedCostume(region, source, query)
	}
	costumeID := query.ID
	if costumeID <= 0 {
		costumeID, _ = ParseExplicitCostumeID(query.Query)
	}
	if costumeID > 0 {
		return source.GetCostumeByID(costumeID)
	}
	return c.resolveSingleCostumeByQuery(region, source, query)
}

func (c *Controller) resolveNormalizedCostume(region renderregion.Value, source DataSource, query Query) (*masterdata.Costume3d, error) {
	characterID, ok := characterIDFor3DRole(query.Character3DID)
	if !ok {
		return nil, fmt.Errorf("角色ID必须在1到31之间")
	}
	colorID := query.ColorID
	if colorID == 0 {
		colorID = 1
	}
	if query.AccessoryID > 0 {
		if c.preview3D == nil {
			return nil, fmt.Errorf("3d preview service is not configured")
		}
		rawID, err := c.preview3D.AccessoryCostume3DIDForRole(c.ctx, region.String(), query.AccessoryID, colorID, query.Character3DID)
		if err != nil {
			return nil, err
		}
		return source.GetCostumeByID(rawID)
	}
	if c.preview3D != nil {
		rawID, err := c.preview3D.OutfitCostume3DIDForRole(c.ctx, region.String(), query.OutfitID, colorID, query.Character3DID)
		if err != nil {
			return nil, err
		}
		return source.GetCostumeByID(rawID)
	}
	items, err := source.FilterCostumes(Filter{
		PartType:    "body",
		CharacterID: characterID,
		ColorID:     colorID,
	})
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if normalizedOutfitID(item) == query.OutfitID {
			return item, nil
		}
	}
	return nil, fmt.Errorf("找不到服装ID %d、角色ID %d、颜色ID %d 的组合", query.OutfitID, query.Character3DID, colorID)
}

func setCostumeDetailPreviewPath(prepared any, previewPath string) {
	root, ok := prepared.(map[string]any)
	if !ok {
		return
	}
	costume, ok := root["costume"].(map[string]any)
	if !ok {
		return
	}
	costume["preview_image_path"] = previewPath
}

type costumeDetailCacheRequest struct {
	Region                  string               `json:"region"`
	Costume                 drawing.CostumeBasic `json:"costume"`
	CharacterIconPath       string               `json:"character_icon_path,omitempty"`
	UnitLogoPath            string               `json:"unit_logo_path,omitempty"`
	Preview3DCacheSignature string               `json:"preview_3d_cache_signature,omitempty"`
}

func (c *Controller) costumeDetailCacheRequest(req *drawing.CostumeDetailRequest) any {
	if req == nil {
		return req
	}
	return costumeDetailCacheRequest{
		Region:                  req.Region,
		Costume:                 req.Costume,
		CharacterIconPath:       req.CharacterIconPath,
		UnitLogoPath:            req.UnitLogoPath,
		Preview3DCacheSignature: c.preview3D.CacheSignature(),
	}
}

func (c *Controller) resolveSingleCostumeByQuery(region renderregion.Value, source DataSource, query Query) (*masterdata.Costume3d, error) {
	lookup := ListQuery{Query: query.Query}
	partType, hasPartType := normalizePartType(query.ExpectedPartType)
	namedLookup := hasPartType && query.Character3DID > 0
	if namedLookup {
		lookup = ListQuery{
			PartType:      partType,
			Character3DID: query.Character3DID,
			Keyword:       strings.TrimSpace(query.Query),
		}
	}
	parsed, err := normalizeListQuery(lookup)
	if err != nil {
		return nil, err
	}
	filter, err := c.buildFilter(parsed)
	if err != nil {
		return nil, err
	}
	if namedLookup && (c.preview3D != nil || partType == "head" || partType == "hair") {
		filter.CharacterID = 0
		filter.CharacterIDs = nil
	}
	filter.ColorID = query.ColorID
	if filter.ColorID <= 0 {
		filter.ColorID = 1
	}
	if !namedLookup {
		filter.Limit = 2
	}
	items, err := source.FilterCostumes(filter)
	if err != nil {
		return nil, err
	}
	logicalIDs := make(map[int][]int)
	if namedLookup && (partType == "head" || partType == "hair") && c.preview3D == nil {
		return nil, fmt.Errorf("3d preview service is not configured")
	}
	if namedLookup && c.preview3D != nil {
		switch partType {
		case "body":
			var outfitIDs map[int]int
			outfitIDs, err = c.preview3D.OutfitIDsForRole(c.ctx, region.String(), query.Character3DID)
			for rawID, outfitID := range outfitIDs {
				logicalIDs[rawID] = []int{outfitID}
			}
		case "head":
			logicalIDs, err = c.preview3D.AccessoryIDsForRole(c.ctx, region.String(), query.Character3DID)
		case "hair":
			var hairIDs map[int]int
			hairIDs, err = c.preview3D.HairIDsForRole(c.ctx, region.String(), query.Character3DID)
			for rawID, hairID := range hairIDs {
				logicalIDs[rawID] = []int{hairID}
			}
		}
		if err != nil {
			return nil, err
		}
	}
	if namedLookup && len(logicalIDs) > 0 {
		usable := items[:0]
		for _, item := range items {
			if item != nil && len(logicalIDs[item.ID]) > 0 {
				usable = append(usable, item)
			}
		}
		items = usable
	}
	if namedLookup {
		needle := strings.TrimSpace(query.Query)
		foldedNeedle := strings.ToLower(needle)
		nameMatches := items[:0]
		exactMatches := make([]*masterdata.Costume3d, 0, len(items))
		for _, item := range items {
			if item == nil || !strings.Contains(strings.ToLower(item.Name), foldedNeedle) {
				continue
			}
			nameMatches = append(nameMatches, item)
			if strings.EqualFold(strings.TrimSpace(item.Name), needle) {
				exactMatches = append(exactMatches, item)
			}
		}
		items = nameMatches
		if len(exactMatches) > 0 {
			items = exactMatches
		}
	}
	if len(items) == 0 {
		if namedLookup {
			return nil, fmt.Errorf("找不到角色ID %d 的%s名称“%s”", query.Character3DID, partTypeName(partType), strings.TrimSpace(query.Query))
		}
		return nil, fmt.Errorf("no costume matched %q", strings.TrimSpace(query.Query))
	}
	if namedLookup {
		uniqueIDs := make(map[int]struct{})
		for _, item := range items {
			if partType == "body" && len(logicalIDs) == 0 {
				if logicalID := normalizedOutfitID(item); logicalID > 0 {
					uniqueIDs[logicalID] = struct{}{}
				}
				continue
			}
			for _, logicalID := range logicalIDs[item.ID] {
				if logicalID > 0 {
					uniqueIDs[logicalID] = struct{}{}
				}
			}
		}
		ids := make([]int, 0, len(uniqueIDs))
		for id := range uniqueIDs {
			ids = append(ids, id)
		}
		sort.Ints(ids)
		if len(ids) == 1 && c.preview3D != nil {
			var rawID int
			switch partType {
			case "body":
				rawID, err = c.preview3D.OutfitCostume3DIDForRole(c.ctx, region.String(), ids[0], filter.ColorID, query.Character3DID)
			case "head":
				rawID, err = c.preview3D.AccessoryCostume3DIDForRole(c.ctx, region.String(), ids[0], filter.ColorID, query.Character3DID)
			case "hair":
				rawID, err = c.preview3D.HairCostume3DIDForRole(c.ctx, region.String(), ids[0], query.Character3DID)
			}
			if err != nil {
				return nil, err
			}
			return source.GetCostumeByID(rawID)
		}
		if len(ids) > 1 {
			return nil, fmt.Errorf("角色ID %d 匹配到多个%s“%s”（ID：%s），请明确填写组件ID", query.Character3DID, partTypeName(partType), strings.TrimSpace(query.Query), joinCostumeIDs(ids))
		}
		if len(items) > 1 {
			if len(ids) == 1 {
				sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
				return items[0], nil
			}
		}
	}
	if len(items) > 1 {
		return nil, fmt.Errorf("matched multiple costumes; use costume list first and query by costume id")
	}
	return items[0], nil
}

func ParseNamedLookupQuery(raw string, partType string) (Query, bool, error) {
	partType, ok := normalizePartType(partType)
	if !ok {
		return Query{}, false, fmt.Errorf("查询类型必须是服装、头饰或发型")
	}
	fields := strings.Fields(strings.TrimSpace(raw))
	if len(fields) == 0 {
		return Query{}, false, nil
	}
	nameFields := make([]string, 0, len(fields))
	roleID := 0
	roleSet := false
	var roleAlias character3DAliasSelection
	setRole := func(id int) error {
		if roleSet {
			return fmt.Errorf("角色ID重复")
		}
		roleID = id
		roleSet = true
		return nil
	}
	for index := 0; index < len(fields); index++ {
		token := fields[index]
		lower := strings.ToLower(strings.TrimSpace(token))
		if label, id, labeled := parseComboLabeledID(lower); labeled && label == "role" {
			if err := setRole(id); err != nil {
				return Query{}, true, err
			}
			continue
		}
		if label, labeled := normalizeComboLabel(lower); labeled && label == "role" {
			if index+1 >= len(fields) {
				return Query{}, true, fmt.Errorf("角色后必须填写1到31之间的ID")
			}
			next := fields[index+1]
			if id, ok := ParseExplicitCostumeID(next); ok {
				if err := setRole(id); err != nil {
					return Query{}, true, err
				}
			} else if characterID, ok := parseCharacter3DAliasToken(next); ok {
				if err := roleAlias.setCharacter(characterID); err != nil {
					return Query{}, true, err
				}
			} else {
				return Query{}, true, fmt.Errorf("角色后必须填写1到31之间的ID或精确角色名称")
			}
			index++
			continue
		}
		nameFields = append(nameFields, token)
	}
	if roleAlias.characterID == 21 && len(nameFields) > 0 {
		if unit, ok := parseCostumeUnitAlias(nameFields[len(nameFields)-1]); ok {
			roleAlias.setUnit(unit)
			nameFields = nameFields[:len(nameFields)-1]
		}
	}
	if !roleSet && roleAlias.characterID == 0 && len(nameFields) > 0 {
		last := len(nameFields) - 1
		if characterID, ok := parseCharacter3DAliasToken(nameFields[last]); ok {
			_ = roleAlias.setCharacter(characterID)
			nameFields = nameFields[:last]
		} else if unit, ok := parseCostumeUnitAlias(nameFields[last]); ok && last > 0 {
			if characterID, ok := parseCharacter3DAliasToken(nameFields[last-1]); ok && characterID == 21 {
				_ = roleAlias.setCharacter(characterID)
				roleAlias.setUnit(unit)
				nameFields = nameFields[:last-1]
			}
		}
	}
	if roleAlias.characterID != 0 {
		if err := roleAlias.apply(&roleID); err != nil {
			return Query{}, true, err
		}
		roleSet = true
	}
	if !roleSet {
		return Query{}, false, nil
	}
	if roleID < 1 || roleID > 31 {
		return Query{}, true, fmt.Errorf("角色ID必须在1到31之间")
	}
	name := strings.TrimSpace(strings.Join(nameFields, " "))
	if name == "" {
		return Query{}, true, fmt.Errorf("请在角色ID之外填写组件名称")
	}
	return Query{
		Query:            name,
		ExpectedPartType: partType,
		Character3DID:    roleID,
		ColorID:          1,
	}, true, nil
}

func (c *Controller) buildFilter(query ListQuery) (Filter, error) {
	filter := Filter{
		PartType: query.PartType,
		Keyword:  strings.TrimSpace(query.Keyword),
	}
	if query.Character3DID > 0 {
		characterID, ok := characterIDFor3DRole(query.Character3DID)
		if !ok {
			return Filter{}, fmt.Errorf("角色ID必须在1到31之间")
		}
		filter.CharacterID = characterID
	} else if characterID, ok := resolveCharacterID(query.Character); ok {
		filter.CharacterID = characterID
	}
	if filter.CharacterID == 0 && strings.TrimSpace(query.Character) != "" {
		filter.Keyword = compactQuery(filter.Keyword, query.Character)
	}
	if ids := characterIDsForGender(query.Gender); len(ids) > 0 {
		filter.CharacterIDs = ids
	}
	if filter.PartType == "" && len(filter.CharacterIDs) > 0 {
		filter.PartType = "body"
	}
	if filter.CharacterID > 0 && len(filter.CharacterIDs) > 0 {
		for _, id := range filter.CharacterIDs {
			if id == filter.CharacterID {
				return filter, nil
			}
		}
		return Filter{}, fmt.Errorf("character and gender filters do not overlap")
	}
	return filter, nil
}

func (c *Controller) buildCostumeBasic(region renderregion.Value, source DataSource, costumeInfo *masterdata.Costume3d, variants []*masterdata.Costume3d, sourceCards map[int][]int) drawing.CostumeBasic {
	if costumeInfo == nil {
		return drawing.CostumeBasic{}
	}
	character, _ := source.GetCharacterByID(costumeInfo.CharacterID)
	basic := drawing.CostumeBasic{
		CostumeID:          costumeInfo.ID,
		CostumeGroupID:     costumeInfo.GroupID,
		Character3DIDs:     character3DIDsForCharacter(costumeInfo.CharacterID),
		Name:               costumeDisplayName(costumeInfo),
		PartType:           costumeInfo.PartType,
		PartName:           partTypeName(costumeInfo.PartType),
		Costume3DType:      costumeInfo.Costume3DType,
		CharacterID:        costumeInfo.CharacterID,
		CharacterName:      characterName(character, costumeInfo.CharacterID),
		CharacterGender:    characterGender(character),
		Rarity:             costumeInfo.Rarity,
		HowToObtain:        costumeInfo.HowToObtain,
		Designer:           costumeInfo.Designer,
		AssetBundleName:    costumeInfo.AssetBundleName,
		ColorID:            costumeInfo.ColorID,
		ColorName:          costumeInfo.ColorName,
		PublishedAt:        costumeInfo.PublishedAt,
		ArchivePublishedAt: costumeInfo.ArchivePublishedAt,
		ThumbnailPath:      c.buildThumbnailPath(region, costumeInfo),
		SourceCardIDs:      sourceCards[costumeInfo.ID],
	}
	if costumeInfo.PartType == "body" {
		basic.OutfitID = normalizedOutfitID(costumeInfo)
	}
	if len(variants) > 0 {
		basic.SourceCardIDs = uniqueCostumeSourceCardIDs(variants, sourceCards)
		basic.Variants = make([]drawing.CostumeColorVariant, 0, len(variants))
		for _, variant := range variants {
			if variant == nil {
				continue
			}
			basic.Variants = append(basic.Variants, drawing.CostumeColorVariant{
				CostumeID:       variant.ID,
				ColorID:         variant.ColorID,
				ColorName:       variant.ColorName,
				AssetBundleName: variant.AssetBundleName,
				ThumbnailPath:   c.buildThumbnailPath(region, variant),
				SourceCardIDs:   sourceCards[variant.ID],
			})
		}
	}
	return basic
}

func (c *Controller) apply3DRoleToCostumeBasic(source DataSource, basic *drawing.CostumeBasic, character3DID int) {
	if basic == nil {
		return
	}
	characterID, ok := characterIDFor3DRole(character3DID)
	if !ok {
		return
	}
	character, _ := source.GetCharacterByID(characterID)
	basic.CharacterID = characterID
	basic.CharacterName = characterName(character, characterID)
	basic.CharacterGender = characterGender(character)
}

func (c *Controller) sourceCardsForCostumes(source DataSource, costumes []*masterdata.Costume3d) (map[int][]int, error) {
	ids := make([]int, 0, len(costumes))
	seen := make(map[int]struct{}, len(costumes))
	for _, costumeInfo := range costumes {
		if costumeInfo == nil || costumeInfo.ID <= 0 {
			continue
		}
		if _, ok := seen[costumeInfo.ID]; ok {
			continue
		}
		seen[costumeInfo.ID] = struct{}{}
		ids = append(ids, costumeInfo.ID)
	}
	if len(ids) == 0 {
		return map[int][]int{}, nil
	}
	return source.GetCostumeSourceCardIDs(ids)
}

func (c *Controller) buildThumbnailPath(region renderregion.Value, costumeInfo *masterdata.Costume3d) string {
	assetBundleName := buildCostumeAssetBundleName(costumeInfo)
	if assetBundleName == "" {
		return ""
	}
	return assets.ResolveRegionAssetPath(c.assets, region.String(), filepath.Join("thumbnail", "costume", assetBundleName+".png"))
}

func (c *Controller) buildCharacterIconPath(characterID int, unit string) string {
	switch characterID {
	case 27:
		return assets.ResolveAssetPath(c.assets, assets.StaticImagesDir, filepath.Join("chara_icon", "miku_light_sound.png"))
	case 28:
		return assets.ResolveAssetPath(c.assets, assets.StaticImagesDir, filepath.Join("chara_icon", "miku_idol.png"))
	case 29:
		return assets.ResolveAssetPath(c.assets, assets.StaticImagesDir, filepath.Join("chara_icon", "miku_street.png"))
	case 30:
		return assets.ResolveAssetPath(c.assets, assets.StaticImagesDir, filepath.Join("chara_icon", "miku_theme_park.png"))
	case 31:
		return assets.ResolveAssetPath(c.assets, assets.StaticImagesDir, filepath.Join("chara_icon", "miku_school_refusal.png"))
	}
	if nickname, ok := assets.CharacterIDToNickname[characterID]; ok {
		return assets.ResolveAssetPath(c.assets, assets.StaticImagesDir, filepath.Join("chara_icon", nickname+".png"))
	}
	return assets.ResolveAssetPath(c.assets, assets.StaticImagesDir, filepath.Join("chara_icon", fmt.Sprintf("chr_icon_%d.png", characterID)))
}

func (c *Controller) buildUnitLogoPath(unit string) string {
	if strings.TrimSpace(unit) == "" {
		return ""
	}
	return assets.ResolveAssetPath(c.assets, assets.StaticImagesDir, fmt.Sprintf("logo_%s.png", unit))
}

func ParseExplicitCostumeID(query string) (int, bool) {
	text := strings.TrimSpace(strings.ToLower(query))
	if text == "" {
		return 0, false
	}
	text = strings.TrimPrefix(text, "id")
	text = strings.TrimPrefix(text, "#")
	value, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil || value <= 0 {
		return 0, false
	}
	return value, true
}

func ParseLookupQuery(raw string, partType string) (Query, bool, error) {
	partType, ok := normalizePartType(partType)
	if !ok || (partType != "body" && partType != "head") {
		return Query{}, false, fmt.Errorf("查询类型必须是服装或饰品")
	}
	fields := strings.Fields(strings.TrimSpace(raw))
	if len(fields) == 0 {
		return Query{}, false, nil
	}
	if !lookupQueryContainsComponentID(fields) {
		return Query{}, false, nil
	}
	query := Query{Query: raw, ExpectedPartType: partType, ColorID: 1}
	recognized := false
	colorSet := false
	pendingRole := false
	var roleAlias character3DAliasSelection
	for _, token := range fields {
		lower := strings.ToLower(strings.TrimSpace(token))
		if pendingRole {
			if characterID, ok := parseCharacter3DAliasToken(lower); ok {
				if err := roleAlias.setCharacter(characterID); err != nil {
					return Query{}, true, err
				}
				pendingRole = false
				continue
			}
			if id, ok := ParseExplicitCostumeID(lower); ok {
				if query.Character3DID != 0 {
					return Query{}, true, fmt.Errorf("角色ID重复")
				}
				query.Character3DID = id
				pendingRole = false
				continue
			}
			return Query{}, true, fmt.Errorf("角色后必须填写1到31之间的ID或精确角色名称")
		}
		if label, id, labeled := parseComboLabeledID(lower); labeled {
			recognized = true
			switch label {
			case "outfit":
				if partType != "body" || query.OutfitID != 0 {
					return Query{}, true, fmt.Errorf("服装查询参数重复或类型不匹配")
				}
				query.OutfitID = id
			case "accessory":
				if partType != "head" || query.AccessoryID != 0 {
					return Query{}, true, fmt.Errorf("饰品查询参数重复或类型不匹配")
				}
				query.AccessoryID = id
			case "role":
				if query.Character3DID != 0 {
					return Query{}, true, fmt.Errorf("角色ID重复")
				}
				query.Character3DID = id
			case "color", "outfit_color", "accessory_color":
				if colorSet {
					return Query{}, true, fmt.Errorf("颜色ID重复")
				}
				query.ColorID = id
				colorSet = true
			default:
				return Query{}, true, fmt.Errorf("查服装和查饰品不接受%s参数", token)
			}
			continue
		}
		if label, labeled := normalizeComboLabel(lower); labeled && label == "role" {
			recognized = true
			pendingRole = true
			continue
		}
		if characterID, ok := parseCharacter3DAliasToken(lower); ok {
			if err := roleAlias.setCharacter(characterID); err != nil {
				return Query{}, true, err
			}
			continue
		}
		id, numeric := ParseExplicitCostumeID(lower)
		if numeric {
			recognized = true
			shortID := query.OutfitID
			if partType == "head" {
				shortID = query.AccessoryID
			}
			switch {
			case shortID == 0:
				if partType == "body" {
					query.OutfitID = id
				} else {
					query.AccessoryID = id
				}
			case query.Character3DID == 0 && roleAlias.characterID == 0:
				query.Character3DID = id
			case !colorSet:
				query.ColorID = id
				colorSet = true
			default:
				return Query{}, true, fmt.Errorf("查询参数过多")
			}
			continue
		}
		if unit, ok := parseCostumeUnitAlias(lower); ok {
			roleAlias.setUnit(unit)
			continue
		}
		if !recognized {
			return Query{}, false, nil
		}
		return Query{}, true, fmt.Errorf("无法识别查询参数：%s", token)
	}
	if pendingRole {
		return Query{}, true, fmt.Errorf("角色后必须填写1到31之间的ID或精确角色名称")
	}
	shortID := query.OutfitID
	label := "服装"
	if partType == "head" {
		shortID = query.AccessoryID
		label = "饰品"
	}
	if shortID <= 0 {
		if !recognized {
			return Query{}, false, nil
		}
		return Query{}, true, fmt.Errorf("请填写%sID", label)
	}
	if err := roleAlias.apply(&query.Character3DID); err != nil {
		return Query{}, true, err
	}
	if query.Character3DID < 1 || query.Character3DID > 31 {
		return Query{}, true, fmt.Errorf("请填写1到31之间的角色ID或精确角色名称")
	}
	if query.ColorID < 1 || query.ColorID > 4 {
		return Query{}, true, fmt.Errorf("颜色ID必须在1到4之间，颜色1为原版")
	}
	return query, true, nil
}

func lookupQueryContainsComponentID(fields []string) bool {
	for _, token := range fields {
		token = strings.ToLower(strings.TrimSpace(token))
		if _, ok := ParseExplicitCostumeID(token); ok {
			return true
		}
		label, _, ok := parseComboLabeledID(token)
		if ok && (label == "outfit" || label == "accessory" || label == "hair") {
			return true
		}
	}
	return false
}

func normalizedOutfitID(costumeInfo *masterdata.Costume3d) int {
	if costumeInfo == nil || costumeInfo.PartType != "body" || costumeInfo.GroupID < 1000 {
		return 0
	}
	return costumeInfo.GroupID / 1000
}

func characterIDFor3DRole(character3DID int) (int, bool) {
	switch {
	case character3DID >= 1 && character3DID <= 20:
		return character3DID, true
	case character3DID >= 21 && character3DID <= 26:
		return 21, true
	case character3DID >= 27 && character3DID <= 31:
		return character3DID - 5, true
	default:
		return 0, false
	}
}

func character3DIDsForCharacter(characterID int) []int {
	switch {
	case characterID >= 1 && characterID <= 20:
		return []int{characterID}
	case characterID == 21:
		return []int{21, 22, 23, 24, 25, 26}
	case characterID >= 22 && characterID <= 26:
		return []int{characterID + 5}
	default:
		return nil
	}
}

type character3DAliasSelection struct {
	characterID     int
	unit            string
	conflictingUnit bool
}

func (selection *character3DAliasSelection) setCharacter(characterID int) error {
	if selection.characterID != 0 {
		return fmt.Errorf("重复指定角色")
	}
	selection.characterID = characterID
	return nil
}

func (selection *character3DAliasSelection) setUnit(unit string) {
	if selection.unit != "" && selection.unit != unit {
		selection.conflictingUnit = true
		return
	}
	selection.unit = unit
}

func (selection character3DAliasSelection) apply(character3DID *int) error {
	resolved, ok, err := selection.resolve()
	if err != nil || !ok {
		return err
	}
	if *character3DID != 0 {
		return fmt.Errorf("重复指定角色")
	}
	*character3DID = resolved
	return nil
}

func (selection character3DAliasSelection) resolve() (int, bool, error) {
	switch {
	case selection.characterID == 0:
		return 0, false, nil
	case selection.characterID >= 1 && selection.characterID <= 20:
		return selection.characterID, true, nil
	case selection.characterID == 21:
		if selection.conflictingUnit {
			return 0, true, fmt.Errorf("使用 Miku 时只能指定一个团队")
		}
		character3DID, ok := mikuCharacter3DIDsByUnit[selection.unit]
		if !ok {
			return 0, true, fmt.Errorf("使用 Miku 时请同时填写团队：vs、mmj、ln、vbs、wxs或n25")
		}
		return character3DID, true, nil
	case selection.characterID >= 22 && selection.characterID <= 26:
		return selection.characterID + 5, true, nil
	default:
		return 0, true, fmt.Errorf("无法把角色名称映射到3D角色")
	}
}

func buildCostumeUnitAliases() map[string]string {
	aliases := filteralias.UnitMap()
	aliases["n25"] = "school_refusal"
	aliases["leo_need"] = "light_sound"
	aliases["wonderlands_showtime"] = "theme_park"
	aliases["virtual_singer"] = "piapro"
	return aliases
}

func parseCostumeUnitAlias(token string) (string, bool) {
	token = strings.TrimSpace(strings.ToLower(token))
	for _, prefix := range []string{"unit=", "unit:", "team=", "team:", "团队=", "团队:", "组合="} {
		if strings.HasPrefix(token, prefix) {
			token = strings.TrimSpace(strings.TrimPrefix(token, prefix))
			break
		}
	}
	unit, ok := costumeUnitAliases[token]
	return unit, ok
}

func parseCharacter3DAliasToken(token string) (int, bool) {
	token = strings.TrimSpace(strings.ToLower(token))
	if characterID, ok := rendercard.ResolveDefaultCharacterNickname(token); ok {
		return characterID, true
	}
	for _, prefix := range []string{"character3d", "character", "角色模型", "角色id", "角色"} {
		if !strings.HasPrefix(token, prefix) {
			continue
		}
		alias := strings.TrimSpace(strings.TrimPrefix(token, prefix))
		if alias == "" {
			return 0, false
		}
		return rendercard.ResolveDefaultCharacterNickname(alias)
	}
	return 0, false
}

func parseComboQuery(query ComboQuery) (ComboQuery, error) {
	parsed := query
	pending := ""
	lastColorTarget := ""
	var roleAlias character3DAliasSelection
	for _, token := range strings.Fields(query.Query) {
		lower := strings.ToLower(strings.TrimSpace(token))
		if lower == "" {
			continue
		}
		if pending == "role" {
			if characterID, ok := parseCharacter3DAliasToken(lower); ok {
				if err := roleAlias.setCharacter(characterID); err != nil {
					return ComboQuery{}, err
				}
				pending = ""
				lastColorTarget = ""
				continue
			}
		}
		if pending == "" {
			if characterID, ok := parseCharacter3DAliasToken(lower); ok {
				if err := roleAlias.setCharacter(characterID); err != nil {
					return ComboQuery{}, err
				}
				lastColorTarget = ""
				continue
			}
			if unit, ok := parseCostumeUnitAlias(lower); ok {
				if _, numeric := ParseExplicitCostumeID(lower); !numeric || lastColorTarget == "" {
					roleAlias.setUnit(unit)
					lastColorTarget = ""
					continue
				}
			}
		}
		if label, id, ok := parseComboLabeledID(lower); ok {
			if err := assignComboValue(&parsed, label, id, &lastColorTarget); err != nil {
				return ComboQuery{}, err
			}
			pending = ""
			continue
		}
		if label, ok := normalizeComboLabel(lower); ok {
			pending = label
			continue
		}
		if id, ok := ParseExplicitCostumeID(lower); ok {
			if pending != "" {
				if err := assignComboValue(&parsed, pending, id, &lastColorTarget); err != nil {
					return ComboQuery{}, err
				}
				pending = ""
				continue
			}
			if lastColorTarget != "" {
				if err := assignComboColor(&parsed, lastColorTarget, id); err != nil {
					return ComboQuery{}, err
				}
				lastColorTarget = ""
				continue
			}
			return ComboQuery{}, fmt.Errorf("组合参数 %s 缺少服装、饰品、发型或角色标签", token)
		}
		return ComboQuery{}, fmt.Errorf("无法识别组合参数：%s", token)
	}
	if pending != "" {
		if pending == "role" {
			return ComboQuery{}, fmt.Errorf("组合参数角色缺少ID或精确角色名称")
		}
		return ComboQuery{}, fmt.Errorf("组合参数 %s 缺少 ID", pending)
	}
	if err := roleAlias.apply(&parsed.Character3DID); err != nil {
		return ComboQuery{}, err
	}
	if parsed.Character3DID < 1 || parsed.Character3DID > 31 {
		return ComboQuery{}, fmt.Errorf("组合必须填写1到31之间的角色ID或精确角色名称")
	}
	if parsed.OutfitColorID == 0 {
		parsed.OutfitColorID = 1
	}
	if parsed.AccessoryColorID == 0 {
		parsed.AccessoryColorID = 1
	}
	return parsed, nil
}

func parseComboLabeledID(token string) (string, int, bool) {
	labels := []string{
		"outfit_color", "costume_color", "服装颜色", "衣装颜色",
		"accessory_color", "饰品颜色", "头饰颜色", "配饰颜色",
		"character3d", "character", "角色模型", "角色", "角色id",
		"color", "颜色", "颜色id",
		"head_optional", "headoptional", "追加饰品", "可选饰品",
		"accessory", "accessories", "饰品", "头饰", "配饰",
		"hairstyle", "hair", "发型", "头发",
		"costume", "body", "衣装", "服装", "衣服",
	}
	for _, prefix := range labels {
		if !strings.HasPrefix(token, prefix) {
			continue
		}
		id, ok := ParseExplicitCostumeID(strings.TrimSpace(strings.TrimPrefix(token, prefix)))
		if !ok {
			continue
		}
		label, _ := normalizeComboLabel(prefix)
		return label, id, true
	}
	return "", 0, false
}

func normalizeComboLabel(token string) (string, bool) {
	switch strings.TrimSpace(strings.ToLower(token)) {
	case "character3d", "character", "角色模型", "角色", "角色id":
		return "role", true
	case "outfit_color", "costume_color", "服装颜色", "衣装颜色":
		return "outfit_color", true
	case "accessory_color", "饰品颜色", "头饰颜色", "配饰颜色":
		return "accessory_color", true
	case "color", "颜色", "颜色id":
		return "color", true
	case "body", "costume", "服装", "衣装", "衣服":
		return "outfit", true
	case "hair", "hairstyle", "发型", "头发":
		return "hair", true
	case "head", "accessory", "accessories", "饰品", "头饰", "配饰":
		return "accessory", true
	case "head_optional", "headoptional", "追加饰品", "可选饰品":
		return "accessory", true
	default:
		return "", false
	}
}

func assignComboValue(query *ComboQuery, label string, id int, lastColorTarget *string) error {
	if id <= 0 {
		return fmt.Errorf("组合部件 ID 无效")
	}
	switch label {
	case "outfit":
		if query.OutfitID != 0 {
			return fmt.Errorf("组合里重复指定服装")
		}
		query.OutfitID = id
		*lastColorTarget = "outfit"
	case "role":
		if id > 31 {
			return fmt.Errorf("角色ID必须在1到31之间")
		}
		if query.Character3DID != 0 {
			return fmt.Errorf("组合里重复指定角色")
		}
		query.Character3DID = id
		*lastColorTarget = ""
	case "color":
		if *lastColorTarget == "" {
			return fmt.Errorf("颜色ID必须紧跟在服装或饰品后面")
		}
		if err := assignComboColor(query, *lastColorTarget, id); err != nil {
			return err
		}
		*lastColorTarget = ""
	case "outfit_color":
		return assignComboColor(query, "outfit", id)
	case "accessory_color":
		return assignComboColor(query, "accessory", id)
	case "hair":
		if query.HairID != 0 || query.HairCostume3DID != 0 {
			return fmt.Errorf("组合里重复指定发型")
		}
		query.HairID = id
		*lastColorTarget = ""
	case "accessory":
		if query.AccessoryID != 0 {
			return fmt.Errorf("组合里重复指定饰品")
		}
		query.AccessoryID = id
		*lastColorTarget = "accessory"
	default:
		return fmt.Errorf("无法识别组合部件类型：%s", label)
	}
	return nil
}

func assignComboColor(query *ComboQuery, target string, id int) error {
	if id < 1 || id > 4 {
		return fmt.Errorf("颜色ID必须在1到4之间，颜色1为原版")
	}
	switch target {
	case "outfit":
		if query.OutfitColorID != 0 {
			return fmt.Errorf("组合里重复指定服装颜色")
		}
		query.OutfitColorID = id
	case "accessory":
		if query.AccessoryColorID != 0 {
			return fmt.Errorf("组合里重复指定饰品颜色")
		}
		query.AccessoryColorID = id
	default:
		return fmt.Errorf("颜色ID必须紧跟在服装或饰品后面")
	}
	return nil
}

func normalizeListQuery(query ListQuery) (ListQuery, error) {
	parsed := query
	keywords := make([]string, 0)
	partLocked := false
	pendingRole := false
	var roleAlias character3DAliasSelection
	fields := strings.Fields(query.Query)
	hasMikuAlias := false
	for _, token := range fields {
		if characterID, ok := parseCharacter3DAliasToken(token); ok && characterID == 21 {
			hasMikuAlias = true
			break
		}
	}
	for _, token := range fields {
		lower := strings.ToLower(strings.TrimSpace(token))
		if lower == "" {
			continue
		}
		if pendingRole {
			if characterID, ok := parseCharacter3DAliasToken(lower); ok {
				if err := roleAlias.setCharacter(characterID); err != nil {
					return ListQuery{}, err
				}
				pendingRole = false
				continue
			}
			if id, ok := ParseExplicitCostumeID(lower); ok {
				if parsed.Character3DID != 0 {
					return ListQuery{}, fmt.Errorf("角色ID重复")
				}
				parsed.Character3DID = id
				pendingRole = false
				continue
			}
			return ListQuery{}, fmt.Errorf("角色后必须填写1到31之间的ID或精确角色名称")
		}
		if gender, partType, ok := normalizeGenderPart(lower); ok {
			parsed.Gender = gender
			parsed.PartType = partType
			partLocked = true
			continue
		}
		if page, ok := parsePageToken(lower); ok {
			parsed.Page = page
			continue
		}
		if pageSize, ok := parsePageSizeToken(lower); ok {
			parsed.PageSize = pageSize
			continue
		}
		if partType, ok := normalizePartType(lower); ok {
			if partLocked {
				continue
			}
			parsed.PartType = partType
			continue
		}
		if gender, ok := normalizeGender(lower); ok {
			parsed.Gender = gender
			continue
		}
		if label, id, ok := parseComboLabeledID(lower); ok && label == "role" {
			if parsed.Character3DID != 0 {
				return ListQuery{}, fmt.Errorf("角色ID重复")
			}
			parsed.Character3DID = id
			continue
		}
		if label, ok := normalizeComboLabel(lower); ok && label == "role" {
			pendingRole = true
			continue
		}
		if characterID, ok := parseCharacter3DAliasToken(lower); ok {
			if err := roleAlias.setCharacter(characterID); err != nil {
				return ListQuery{}, err
			}
			continue
		}
		if value, err := strconv.Atoi(lower); err == nil && value > 0 && value <= 31 {
			parsed.Character = lower
			continue
		}
		if unit, ok := parseCostumeUnitAlias(lower); ok && hasMikuAlias {
			roleAlias.setUnit(unit)
			continue
		}
		keywords = append(keywords, token)
	}
	if pendingRole {
		return ListQuery{}, fmt.Errorf("角色后必须填写1到31之间的ID或精确角色名称")
	}
	if roleAlias.characterID != 0 {
		if err := roleAlias.apply(&parsed.Character3DID); err != nil {
			return ListQuery{}, err
		}
	}
	if explicitPart, ok := normalizePartType(query.PartType); ok {
		parsed.PartType = explicitPart
	}
	if explicitGender, explicitPart, ok := normalizeGenderPart(query.Gender); ok {
		parsed.Gender = explicitGender
		if parsed.PartType == "" {
			parsed.PartType = explicitPart
		}
	} else if explicitGender, ok := normalizeGender(query.Gender); ok {
		parsed.Gender = explicitGender
	}
	if strings.TrimSpace(query.Keyword) != "" {
		keywords = append(keywords, query.Keyword)
	}
	parsed.Keyword = strings.TrimSpace(strings.Join(keywords, " "))
	return parsed, nil
}

func parsePageToken(token string) (int, bool) {
	token = strings.TrimPrefix(token, "page")
	token = strings.TrimPrefix(token, "p")
	token = strings.TrimPrefix(token, "第")
	token = strings.TrimSuffix(token, "页")
	page, err := strconv.Atoi(token)
	if err != nil || page <= 0 {
		return 0, false
	}
	return page, true
}

func parsePageSizeToken(token string) (int, bool) {
	switch token {
	case "all", "full", "全部", "拉满":
		return MaxPageSize, true
	}
	for _, prefix := range []string{"pagesize", "size", "ps", "limit", "每页", "页大小"} {
		if strings.HasPrefix(token, prefix) {
			return parsePositiveBoundedInt(strings.TrimPrefix(token, prefix), MaxPageSize)
		}
	}
	for _, suffix := range []string{"条/页", "个/页", "项/页"} {
		if strings.HasSuffix(token, suffix) {
			return parsePositiveBoundedInt(strings.TrimSuffix(token, suffix), MaxPageSize)
		}
	}
	return 0, false
}

func parsePositiveBoundedInt(token string, maxValue int) (int, bool) {
	value, err := strconv.Atoi(strings.TrimSpace(token))
	if err != nil || value <= 0 {
		return 0, false
	}
	if maxValue > 0 && value > maxValue {
		value = maxValue
	}
	return value, true
}

func normalizePartType(token string) (string, bool) {
	switch token {
	case "body", "costume", "costumes", "衣装", "服装", "衣服", "服饰":
		return "body", true
	case "head", "accessory", "accessories", "饰品", "头饰", "配饰":
		return "head", true
	case "hair", "hairstyle", "发型", "头发":
		return "hair", true
	default:
		return "", false
	}
}

func normalizeGenderPart(token string) (string, string, bool) {
	switch token {
	case "男装", "男服装", "男衣装", "男衣服", "男性服装", "男性衣装":
		return "male", "body", true
	case "女装", "女服装", "女衣装", "女衣服", "女性服装", "女性衣装":
		return "female", "body", true
	case "男饰品", "男头饰", "男配饰", "男性饰品", "男性头饰":
		return "male", "head", true
	case "女饰品", "女头饰", "女配饰", "女性饰品", "女性头饰":
		return "female", "head", true
	case "男发型", "男头发", "男性发型":
		return "male", "hair", true
	case "女发型", "女头发", "女性发型":
		return "female", "hair", true
	default:
		return "", "", false
	}
}

func normalizeGender(token string) (string, bool) {
	switch token {
	case "male", "boy", "boys", "男", "男性":
		return "male", true
	case "female", "girl", "girls", "女", "女性":
		return "female", true
	case "secret", "其他":
		return "secret", true
	default:
		return "", false
	}
}

func characterIDsForGender(gender string) []int {
	switch strings.TrimSpace(gender) {
	case "male":
		return []int{11, 12, 13, 16, 23, 26}
	case "female":
		return []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 14, 15, 17, 18, 19, 20, 21, 22, 24, 25}
	case "secret":
		return []int{20}
	default:
		return nil
	}
}

func resolveCharacterID(query string) (int, bool) {
	text := strings.TrimSpace(query)
	if text == "" {
		return 0, false
	}
	if value, err := strconv.Atoi(text); err == nil && value > 0 && value <= 31 {
		return value, true
	}
	return rendercard.ResolveDefaultCharacterNickname(text)
}

func sortCostumesForDisplay(items []*masterdata.Costume3d) {
	sort.Slice(items, func(i, j int) bool {
		left := costumeSortTime(items[i])
		right := costumeSortTime(items[j])
		if left == right {
			if items[i].Seq == items[j].Seq {
				return items[i].ID > items[j].ID
			}
			return items[i].Seq > items[j].Seq
		}
		return left > right
	})
}

func costumeSortTime(costumeInfo *masterdata.Costume3d) int64 {
	if costumeInfo == nil {
		return 0
	}
	if costumeInfo.PublishedAt > 0 {
		return costumeInfo.PublishedAt
	}
	return costumeInfo.ArchivePublishedAt
}

func buildCostumeAssetBundleName(costumeInfo *masterdata.Costume3d) string {
	if costumeInfo == nil {
		return ""
	}
	override := strings.TrimSpace(costumeInfo.AssetBundleName)
	if strings.Contains(override, "_") {
		return override
	}
	partType := strings.TrimSpace(costumeInfo.PartType)
	if partType == "" {
		return override
	}
	baseName := override
	if baseName == "" {
		baseName = fmt.Sprintf("%04d", costumeInfo.ID/1000)
	}
	var sb strings.Builder
	sb.WriteString("cos")
	sb.WriteString(baseName)
	sb.WriteString("_")
	sb.WriteString(partType)
	if costumeInfo.ColorID >= 2 {
		fmt.Fprintf(&sb, "_%02d", costumeInfo.ColorID-1)
	}
	return sb.String()
}

func costumeDisplayName(costumeInfo *masterdata.Costume3d) string {
	if costumeInfo == nil {
		return ""
	}
	name := strings.TrimSpace(costumeInfo.Name)
	if name == "" {
		name = strings.TrimSpace(costumeInfo.Description)
	}
	if name == "" {
		name = strings.TrimSpace(costumeInfo.AssetBundleName)
	}
	return name
}

func partTypeName(partType string) string {
	switch strings.TrimSpace(partType) {
	case "body":
		return "服装"
	case "head":
		return "饰品"
	case "hair":
		return "发型"
	default:
		return partType
	}
}

func characterName(character *masterdata.Character, fallbackID int) string {
	if character == nil {
		return fmt.Sprintf("角色%d", fallbackID)
	}
	name := strings.TrimSpace(strings.TrimSpace(character.FirstName) + strings.TrimSpace(character.GivenName))
	if name == "" {
		name = strings.TrimSpace(character.GivenName)
	}
	if name == "" {
		return fmt.Sprintf("角色%d", fallbackID)
	}
	return name
}

func characterUnit(character *masterdata.Character) string {
	if character == nil {
		return ""
	}
	return strings.TrimSpace(character.Unit)
}

func characterGender(character *masterdata.Character) string {
	if character == nil {
		return ""
	}
	return strings.TrimSpace(character.Gender)
}

func compactQuery(parts ...string) string {
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			clean = append(clean, strings.TrimSpace(part))
		}
	}
	return strings.Join(clean, " ")
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func uniqueCostumeSourceCardIDs(variants []*masterdata.Costume3d, sourceCards map[int][]int) []int {
	seen := make(map[int]struct{})
	for _, variant := range variants {
		if variant == nil {
			continue
		}
		for _, cardID := range sourceCards[variant.ID] {
			seen[cardID] = struct{}{}
		}
	}
	result := make([]int, 0, len(seen))
	for cardID := range seen {
		result = append(result, cardID)
	}
	sort.Ints(result)
	return result
}

func joinCostumeIDs(ids []int) string {
	labels := make([]string, 0, len(ids))
	for _, id := range ids {
		labels = append(labels, strconv.Itoa(id))
	}
	return strings.Join(labels, "、")
}

func buildListTitle(query ListQuery) *string {
	label := buildFilterLabel(query)
	if label == "" {
		label = "全部服装"
	}
	title := fmt.Sprintf("%s 查询结果", label)
	return &title
}

func BuildListPrompt(payload *drawing.CostumeListRequest) string {
	if payload == nil {
		return ""
	}
	title := "服装查询结果"
	if payload.Title != nil && strings.TrimSpace(*payload.Title) != "" {
		title = strings.TrimSpace(*payload.Title)
	}
	page := payload.Page
	if page <= 0 {
		page = 1
	}
	totalPages := payload.TotalPages
	if totalPages <= 0 {
		totalPages = 1
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s：第 %d/%d 页，本页 %d 项，共 %d 项", title, page, totalPages, len(payload.Costumes), payload.Total)
	sb.WriteString("\n详情：/查服装 服装ID 角色ID/昵称 [颜色ID]；/查饰品 饰品ID 角色ID/昵称 [颜色ID]")
	if len(payload.Costumes) > 0 && payload.Costumes[0].HairID > 0 {
		sb.WriteString("\n试穿：/组合 角色ID/昵称 发型ID")
	}
	if totalPages > 1 {
		nextPage := page + 1
		if nextPage > totalPages {
			nextPage = totalPages
		}
		fmt.Fprintf(&sb, "\n翻页：在原查询后加 p%d；拉满：加 每页%d", nextPage, MaxPageSize)
	}
	return sb.String()
}

func buildFilterLabel(query ListQuery) string {
	parts := make([]string, 0, 4)
	if query.PartType != "" {
		parts = append(parts, partTypeName(query.PartType))
	}
	if query.Gender != "" {
		switch query.Gender {
		case "male":
			parts = append(parts, "男装")
		case "female":
			parts = append(parts, "女装")
		case "secret":
			parts = append(parts, "其他")
		}
	}
	if query.Character != "" {
		parts = append(parts, query.Character)
	}
	if query.Character3DID > 0 {
		parts = append(parts, fmt.Sprintf("角色%d", query.Character3DID))
	}
	if len(query.AccessoryIDs) > 0 {
		parts = append(parts, "ID"+joinCostumeIDs(query.AccessoryIDs))
	}
	if query.Keyword != "" {
		parts = append(parts, query.Keyword)
	}
	return strings.Join(parts, " / ")
}
