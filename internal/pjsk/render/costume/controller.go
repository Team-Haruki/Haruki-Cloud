package costume

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"haruki-cloud/internal/observability/commandtrace"
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
	clone.assets = c.assets.WithContext(ctx)
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
	build := &costumeListBuild{
		controller:        c,
		region:            region,
		source:            source,
		query:             parsed,
		filter:            filter,
		items:             items,
		accessoryListMode: parsed.PartType == "head",
		mixedListMode:     parsed.PartType == "" && c.preview3D != nil,
	}
	if err := build.prepareItems(); err != nil {
		return nil, err
	}
	if err := build.applyAccessoryIDFilter(); err != nil {
		return nil, err
	}
	page := build.paginate()
	costumes, err := build.costumes(page)
	if err != nil {
		return nil, err
	}
	return &drawing.CostumeListRequest{
		Region:      region.String(),
		Title:       buildListTitle(parsed),
		Page:        page.number,
		PageSize:    page.size,
		Total:       page.total,
		TotalPages:  page.totalPages,
		FilterLabel: buildFilterLabel(parsed),
		Costumes:    costumes,
	}, nil
}

type costumeListBuild struct {
	controller        *Controller
	region            renderregion.Value
	source            DataSource
	query             ListQuery
	filter            Filter
	items             []*masterdata.Costume3d
	hairIDs           map[int]int
	accessoryItems    []costumeAccessoryListItem
	accessoryListMode bool
	mixedListMode     bool
}

type costumeListPage struct {
	number         int
	size           int
	total          int
	totalPages     int
	items          []*masterdata.Costume3d
	accessoryItems []costumeAccessoryListItem
}

func (b *costumeListBuild) prepareItems() error {
	switch {
	case b.query.PartType == "hair" && b.query.Character3DID > 0:
		return b.prepareHairItems()
	case b.query.PartType == "head":
		return b.prepareAccessoryItems()
	case b.mixedListMode:
		return b.prepareMixedItems()
	default:
		sortCostumesForDisplay(b.items)
		return nil
	}
}

func (b *costumeListBuild) prepareHairItems() error {
	if b.controller.preview3D == nil {
		return fmt.Errorf("3d preview service is not configured")
	}
	hairIDs, err := b.controller.preview3D.HairIDsForRole(b.controller.ctx, b.region.String(), b.query.Character3DID)
	if err != nil {
		return err
	}
	b.hairIDs = hairIDs
	filtered := b.items[:0]
	for _, item := range b.items {
		if item != nil && hairIDs[item.ID] > 0 {
			filtered = append(filtered, item)
		}
	}
	b.items = filtered
	sort.Slice(b.items, func(i, j int) bool { return hairIDs[b.items[i].ID] < hairIDs[b.items[j].ID] })
	return nil
}

func (b *costumeListBuild) prepareAccessoryItems() error {
	if b.controller.preview3D == nil {
		return fmt.Errorf("3d preview service is not configured")
	}
	catalog, err := b.controller.preview3D.AccessoryCatalog(b.controller.ctx, b.region.String(), b.query.Character3DID)
	if err != nil {
		return err
	}
	b.accessoryItems = buildCostumeAccessoryListItems(b.source, b.items, catalog, b.query.Character3DID > 0)
	return nil
}

func (b *costumeListBuild) prepareMixedItems() error {
	baseItems, headItems, hairItems := splitCostumeListItems(b.items)
	if b.query.Character3DID > 0 {
		var err error
		headItems, hairItems, b.hairIDs, err = b.roleComponentItems()
		if err != nil {
			return err
		}
	}
	catalog, err := b.controller.preview3D.AccessoryCatalog(b.controller.ctx, b.region.String(), b.query.Character3DID)
	if err != nil {
		return err
	}
	b.accessoryItems = b.composeMixedItems(baseItems, headItems, hairItems, catalog)
	sortCostumeLogicalListItems(b.accessoryItems)
	return nil
}

func splitCostumeListItems(items []*masterdata.Costume3d) (base, head, hair []*masterdata.Costume3d) {
	base = make([]*masterdata.Costume3d, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		switch item.PartType {
		case "head":
			head = append(head, item)
		case "hair":
			hair = append(hair, item)
		default:
			base = append(base, item)
		}
	}
	return base, head, hair
}

func (b *costumeListBuild) roleComponentItems() ([]*masterdata.Costume3d, []*masterdata.Costume3d, map[int]int, error) {
	filter := b.filter
	filter.CharacterID = 0
	filter.CharacterIDs = nil
	filter.PartType = "head"
	headItems, err := b.source.FilterCostumes(filter)
	if err != nil {
		return nil, nil, nil, err
	}
	filter.PartType = "hair"
	hairItems, err := b.source.FilterCostumes(filter)
	if err != nil {
		return nil, nil, nil, err
	}
	hairIDs, err := b.controller.preview3D.HairIDsForRole(b.controller.ctx, b.region.String(), b.query.Character3DID)
	return headItems, hairItems, hairIDs, err
}

func (b *costumeListBuild) composeMixedItems(baseItems, headItems, hairItems []*masterdata.Costume3d, catalog []preview3DAccessoryCatalogEntry) []costumeAccessoryListItem {
	items := make([]costumeAccessoryListItem, 0, len(baseItems)+len(hairItems)+len(catalog))
	for _, item := range baseItems {
		items = append(items, costumeAccessoryListItem{costume: item})
	}
	for _, item := range hairItems {
		if b.query.Character3DID == 0 || b.hairIDs[item.ID] > 0 {
			items = append(items, costumeAccessoryListItem{costume: item})
		}
	}
	return append(items, buildCostumeAccessoryListItems(b.source, headItems, catalog, b.query.Character3DID > 0)...)
}

func (b *costumeListBuild) applyAccessoryIDFilter() error {
	if len(b.query.AccessoryIDs) == 0 {
		return nil
	}
	if !b.accessoryListMode {
		return fmt.Errorf("accessory id filter requires an accessory list")
	}
	allowed := positiveIntSet(b.query.AccessoryIDs)
	filtered := b.accessoryItems[:0]
	for _, item := range b.accessoryItems {
		if _, ok := allowed[item.accessoryID]; ok {
			filtered = append(filtered, item)
		}
	}
	b.accessoryItems = filtered
	return nil
}

func positiveIntSet(values []int) map[int]struct{} {
	result := make(map[int]struct{}, len(values))
	for _, value := range values {
		if value > 0 {
			result[value] = struct{}{}
		}
	}
	return result
}

func (b *costumeListBuild) paginate() costumeListPage {
	pageSize := boundedCostumePageSize(b.query.PageSize)
	pageNumber := b.query.Page
	if pageNumber <= 0 {
		pageNumber = 1
	}
	page := costumeListPage{number: pageNumber, size: pageSize, total: len(b.items)}
	switch {
	case b.accessoryListMode:
		page.total = len(b.accessoryItems)
		page.accessoryItems, page.totalPages = paginateCostumeAccessoryListItems(b.accessoryItems, pageSize, pageNumber)
		page.items = costumeItemsFromAccessoryItems(page.accessoryItems)
	case b.mixedListMode:
		page.total = len(b.accessoryItems)
		page.accessoryItems, page.totalPages = paginateCostumeLogicalListItems(b.accessoryItems, b.query, pageSize, pageNumber)
		page.items = costumeItemsFromAccessoryItems(page.accessoryItems)
	default:
		page.items, page.totalPages = paginateCostumeListItems(b.items, b.query, pageSize, pageNumber)
	}
	if page.number > page.totalPages {
		page.number = page.totalPages
	}
	return page
}

func boundedCostumePageSize(pageSize int) int {
	if pageSize <= 0 {
		return DefaultPageSize
	}
	if pageSize > MaxPageSize {
		return MaxPageSize
	}
	return pageSize
}

func costumeItemsFromAccessoryItems(items []costumeAccessoryListItem) []*masterdata.Costume3d {
	result := make([]*masterdata.Costume3d, 0, len(items))
	for _, item := range items {
		result = append(result, item.costume)
	}
	return result
}

func (b *costumeListBuild) costumes(page costumeListPage) ([]drawing.CostumeBasic, error) {
	sourceCards, err := b.controller.sourceCardsForCostumes(b.source, page.items)
	if err != nil {
		return nil, err
	}
	costumes := make([]drawing.CostumeBasic, 0, len(page.items))
	for index, item := range page.items {
		costumes = append(costumes, b.costumeBasic(item, pageAccessoryItem(page.accessoryItems, index), sourceCards))
	}
	return costumes, nil
}

func pageAccessoryItem(items []costumeAccessoryListItem, index int) *costumeAccessoryListItem {
	if index < 0 || index >= len(items) {
		return nil
	}
	return &items[index]
}

func (b *costumeListBuild) costumeBasic(item *masterdata.Costume3d, accessoryItem *costumeAccessoryListItem, sourceCards map[int][]int) drawing.CostumeBasic {
	basic := b.controller.buildCostumeBasic(b.region, b.source, item, nil, sourceCards)
	if b.hairIDs[item.ID] > 0 {
		basic.HairID = b.hairIDs[item.ID]
		b.applyRole(&basic)
	}
	if accessoryItem != nil {
		if accessoryItem.accessoryID > 0 {
			basic.AccessoryID = accessoryItem.accessoryID
			basic.Character3DIDs = append([]int(nil), accessoryItem.character3DIDs...)
		}
		if b.query.Character3DID > 0 {
			b.applyRole(&basic)
		}
	}
	return basic
}

func (b *costumeListBuild) applyRole(basic *drawing.CostumeBasic) {
	basic.Character3DID = b.query.Character3DID
	basic.Character3DIDs = []int{b.query.Character3DID}
	b.controller.apply3DRoleToCostumeBasic(b.source, basic, b.query.Character3DID)
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
	byRawID := costumeItemsByID(items)
	result := make([]costumeAccessoryListItem, 0, len(catalog))
	for _, entry := range catalog {
		matched := firstCostumeItemByID(byRawID, entry.Costume3DIDs)
		if matched == nil {
			continue
		}
		result = append(result, costumeAccessoryListItem{
			costume:        resolveCostumeAccessoryRepresentative(source, byRawID, entry.RepresentativeCostume3DID, matched, useRegistryRepresentative),
			accessoryID:    entry.AccessoryID,
			character3DIDs: append([]int(nil), entry.Character3DIDs...),
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].accessoryID < result[j].accessoryID })
	return result
}

func costumeItemsByID(items []*masterdata.Costume3d) map[int]*masterdata.Costume3d {
	byID := make(map[int]*masterdata.Costume3d, len(items))
	for _, item := range items {
		if item != nil {
			byID[item.ID] = item
		}
	}
	return byID
}

func firstCostumeItemByID(items map[int]*masterdata.Costume3d, ids []int) *masterdata.Costume3d {
	for _, id := range ids {
		if item := items[id]; item != nil {
			return item
		}
	}
	return nil
}

func resolveCostumeAccessoryRepresentative(source DataSource, items map[int]*masterdata.Costume3d, representativeID int, fallback *masterdata.Costume3d, useRegistry bool) *masterdata.Costume3d {
	if representative := items[representativeID]; representative != nil {
		return representative
	}
	if useRegistry && source != nil {
		if representative, _ := source.GetCostumeByID(representativeID); representative != nil {
			return representative
		}
	}
	return fallback
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
	return paginateCostumeItemsByPart(items, pageSize, page, func(item costumeAccessoryListItem) (string, bool) {
		if item.costume == nil {
			return "", false
		}
		return item.costume.PartType, true
	})
}

func shouldBalanceCostumeListByPart(query ListQuery) bool {
	return strings.TrimSpace(query.PartType) == "" && (strings.TrimSpace(query.Character) != "" || query.Character3DID > 0)
}

func paginateCostumeListItemsByPart(items []*masterdata.Costume3d, pageSize int, page int) []*masterdata.Costume3d {
	return paginateCostumeItemsByPart(items, pageSize, page, func(item *masterdata.Costume3d) (string, bool) {
		if item == nil {
			return "", false
		}
		return item.PartType, true
	})
}

func paginateCostumeItemsByPart[T any](items []T, pageSize int, page int, partFor func(T) (string, bool)) []T {
	groups, encountered := groupCostumeItemsByPart(items, partFor)
	ordered := orderedCostumePartTypes(groups, encountered)
	offsets := make(map[string]int, len(groups))
	var current []T
	for currentPage := 1; currentPage <= page; currentPage++ {
		current = buildBalancedCostumePage(groups, ordered, offsets, pageSize)
	}
	return current
}

func groupCostumeItemsByPart[T any](items []T, partFor func(T) (string, bool)) (map[string][]T, []string) {
	groups := make(map[string][]T)
	encountered := make([]string, 0, len(costumePartOrder))
	seen := make(map[string]struct{})
	for _, item := range items {
		partType, ok := partFor(item)
		if !ok {
			continue
		}
		partType = strings.TrimSpace(partType)
		groups[partType] = append(groups[partType], item)
		if _, known := seen[partType]; !known {
			seen[partType] = struct{}{}
			encountered = append(encountered, partType)
		}
	}
	return groups, encountered
}

func orderedCostumePartTypes[T any](groups map[string][]T, encountered []string) []string {
	ordered := make([]string, 0, len(encountered))
	for _, partType := range costumePartOrder {
		if len(groups[partType]) > 0 {
			ordered = append(ordered, partType)
		}
	}
	for _, partType := range encountered {
		if !containsString(ordered, partType) {
			ordered = append(ordered, partType)
		}
	}
	return ordered
}

func buildBalancedCostumePage[T any](groups map[string][]T, ordered []string, offsets map[string]int, pageSize int) []T {
	current := make([]T, 0, pageSize)
	for len(current) < pageSize {
		previousLength := len(current)
		current = appendCostumePartRound(current, groups, ordered, offsets, pageSize)
		if len(current) == previousLength {
			break
		}
	}
	return current
}

func appendCostumePartRound[T any](current []T, groups map[string][]T, ordered []string, offsets map[string]int, pageSize int) []T {
	for _, partType := range ordered {
		if len(current) >= pageSize {
			break
		}
		group := groups[partType]
		index := offsets[partType]
		if index >= len(group) {
			continue
		}
		current = append(current, group[index])
		offsets[partType] = index + 1
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
	finishBuild := commandtrace.MeasureOperation(c.ctx, payloadBuildStage)
	payload, err := c.BuildCostumeListRequest(query)
	finishBuild()
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
	return c.buildResolvedCostumeDetailRequest(region, source, costumeInfo, query)
}

func (c *Controller) buildResolvedCostumeDetailRequest(region renderregion.Value, source DataSource, costumeInfo *masterdata.Costume3d, query Query) (*drawing.CostumeDetailRequest, error) {
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
	displayCharacterID, err := c.applyCostumeDetailRole(region, source, costumeInfo, query, &costumeBasic)
	if err != nil {
		return nil, err
	}
	character, _ := source.GetCharacterByID(displayCharacterID)
	return &drawing.CostumeDetailRequest{
		Region:            region.String(),
		Costume:           costumeBasic,
		CharacterIconPath: c.buildCharacterIconPath(displayCharacterID, characterUnit(character)),
		UnitLogoPath:      c.buildUnitLogoPath(characterUnit(character)),
	}, nil
}

func (c *Controller) applyCostumeDetailRole(region renderregion.Value, source DataSource, costumeInfo *masterdata.Costume3d, query Query, basic *drawing.CostumeBasic) (int, error) {
	if query.Character3DID <= 0 {
		return costumeInfo.CharacterID, nil
	}
	basic.Character3DID = query.Character3DID
	basic.Character3DIDs = []int{query.Character3DID}
	c.apply3DRoleToCostumeBasic(source, basic, query.Character3DID)
	err := c.applyCostumeDetailPartRole(region, costumeInfo, query, basic)
	return basic.CharacterID, err
}

func (c *Controller) applyCostumeDetailPartRole(region renderregion.Value, costumeInfo *masterdata.Costume3d, query Query, basic *drawing.CostumeBasic) error {
	switch costumeInfo.PartType {
	case "body":
		return c.applyCostumeDetailBodyRole(region, costumeInfo, query, basic)
	case "head":
		return c.applyCostumeDetailAccessoryRole(region, costumeInfo, query, basic)
	case "hair":
		return c.applyCostumeDetailHairRole(region, costumeInfo, query, basic)
	default:
		return nil
	}
}

func (c *Controller) applyCostumeDetailBodyRole(region renderregion.Value, costumeInfo *masterdata.Costume3d, query Query, basic *drawing.CostumeBasic) error {
	if c.preview3D == nil {
		return nil
	}
	outfitIDs, err := c.preview3D.OutfitIDsForRole(c.ctx, region.String(), query.Character3DID)
	if err != nil {
		return err
	}
	outfitID := outfitIDs[costumeInfo.ID]
	if outfitID <= 0 {
		return fmt.Errorf("服装 %d 不适用于角色ID %d", costumeInfo.ID, query.Character3DID)
	}
	if query.OutfitID > 0 && query.OutfitID != outfitID {
		return fmt.Errorf("服装ID %d 不适用于角色ID %d", query.OutfitID, query.Character3DID)
	}
	basic.OutfitID = outfitID
	return nil
}

func (c *Controller) applyCostumeDetailAccessoryRole(region renderregion.Value, costumeInfo *masterdata.Costume3d, query Query, basic *drawing.CostumeBasic) error {
	if c.preview3D == nil {
		return fmt.Errorf("3d preview service is not configured")
	}
	accessoryIDs, err := c.preview3D.AccessoryIDsForRole(c.ctx, region.String(), query.Character3DID)
	if err != nil {
		return err
	}
	resolvedIDs := accessoryIDs[costumeInfo.ID]
	if len(resolvedIDs) == 0 {
		return fmt.Errorf("饰品 %d 不适用于角色ID %d", costumeInfo.ID, query.Character3DID)
	}
	return selectCostumeDetailAccessory(resolvedIDs, costumeInfo.ID, query, basic)
}

func selectCostumeDetailAccessory(resolvedIDs []int, costumeID int, query Query, basic *drawing.CostumeBasic) error {
	if query.AccessoryID > 0 {
		if !slices.Contains(resolvedIDs, query.AccessoryID) {
			return fmt.Errorf("饰品ID %d 不适用于角色ID %d", query.AccessoryID, query.Character3DID)
		}
		basic.AccessoryID = query.AccessoryID
		return nil
	}
	if len(resolvedIDs) > 1 {
		return fmt.Errorf("饰品原始ID %d 对角色ID %d 对应多个独立饰品（ID：%s），请明确填写饰品ID", costumeID, query.Character3DID, joinCostumeIDs(resolvedIDs))
	}
	basic.AccessoryID = resolvedIDs[0]
	return nil
}

func (c *Controller) applyCostumeDetailHairRole(region renderregion.Value, costumeInfo *masterdata.Costume3d, query Query, basic *drawing.CostumeBasic) error {
	if c.preview3D == nil {
		return fmt.Errorf("3d preview service is not configured")
	}
	hairIDs, err := c.preview3D.HairIDsForRole(c.ctx, region.String(), query.Character3DID)
	if err != nil {
		return err
	}
	basic.HairID = hairIDs[costumeInfo.ID]
	if basic.HairID <= 0 {
		return fmt.Errorf("发型 %d 不适用于角色ID %d", costumeInfo.ID, query.Character3DID)
	}
	return nil
}

func (c *Controller) resolve3DPreviewPath(ctx context.Context, region renderregion.Value, costumeInfo *masterdata.Costume3d, query Query) (string, error) {
	if c == nil || c.preview3D == nil || costumeInfo == nil {
		return "", nil
	}
	if ctx == nil {
		ctx = c.ctx
		if ctx == nil {
			ctx = context.Background()
		}
	}
	return c.preview3D.ResolveQueryPreviewPath(ctx, region.String(), costumeInfo.ID, query)
}

func (c *Controller) ensure3DPreviewCapture(ctx context.Context, region renderregion.Value, costumeInfo *masterdata.Costume3d, query Query) error {
	if c == nil || c.preview3D == nil || costumeInfo == nil {
		return nil
	}
	if ctx == nil {
		ctx = c.ctx
		if ctx == nil {
			ctx = context.Background()
		}
	}
	return c.preview3D.EnsureQueryPreviewCapture(ctx, region.String(), costumeInfo.ID, query)
}

func (c *Controller) RenderCostumeDetail(query Query) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	finishBuild := commandtrace.MeasureOperation(c.ctx, payloadBuildStage)
	region, source, err := c.resolveSource(query.Region)
	if err != nil {
		finishBuild()
		return nil, err
	}
	costumeInfo, err := c.resolveCostumeInfo(region, source, query)
	if err != nil {
		finishBuild()
		return nil, err
	}
	if expectedPart, ok := normalizePartType(query.ExpectedPartType); ok && costumeInfo.PartType != expectedPart {
		finishBuild()
		return nil, fmt.Errorf("costume %d is %s, not %s", costumeInfo.ID, partTypeName(costumeInfo.PartType), partTypeName(expectedPart))
	}
	payload, err := c.buildResolvedCostumeDetailRequest(region, source, costumeInfo, query)
	if err != nil {
		finishBuild()
		return nil, err
	}
	cachePayload := c.costumeDetailCacheRequest(payload)
	finishBuild()
	return c.drawing.GenerateCostumeDetailWithContextPrepare(cachePayload, payload, func(renderCtx context.Context, prepared any) error {
		previewPath, err := c.resolve3DPreviewPath(renderCtx, region, costumeInfo, query)
		if err != nil {
			costumePreview3DLogger.WarnContext(renderCtx, "3d preview skipped",
				"region", region.String(),
				"costume_id", costumeInfo.ID,
				"error_type", fmt.Sprintf("%T", err),
			)
			return nil
		}
		if previewPath == "" {
			return nil
		}
		setCostumeDetailPreviewPath(prepared, previewPath)
		return c.ensure3DPreviewCapture(renderCtx, region, costumeInfo, query)
	})
}

func (c *Controller) RenderCostumeCombo(query ComboQuery) ([]byte, error) {
	if c == nil || c.preview3D == nil {
		return nil, fmt.Errorf("3d preview service is not configured")
	}
	finishBuild := commandtrace.MeasureOperation(c.ctx, payloadBuildStage)
	parsed, err := parseComboQuery(query)
	finishBuild()
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
	if query.OutfitID > 0 || query.AccessoryID > 0 || query.HairID > 0 {
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
	colorID := normalizedCostumeColorID(query.ColorID)
	switch {
	case query.HairID > 0:
		return c.resolveNormalizedHair(region, source, query)
	case query.AccessoryID > 0:
		return c.resolveNormalizedAccessory(region, source, query, colorID)
	default:
		return c.resolveNormalizedOutfit(region, source, query, characterID, colorID)
	}
}

func normalizedCostumeColorID(colorID int) int {
	if colorID <= 0 {
		return 1
	}
	return colorID
}

func (c *Controller) resolveNormalizedHair(region renderregion.Value, source DataSource, query Query) (*masterdata.Costume3d, error) {
	if c.preview3D == nil {
		return nil, fmt.Errorf("3d preview service is not configured")
	}
	rawID, err := c.preview3D.HairCostume3DIDForRole(c.ctx, region.String(), query.HairID, query.Character3DID)
	if err != nil {
		return nil, err
	}
	return source.GetCostumeByID(rawID)
}

func (c *Controller) resolveNormalizedAccessory(region renderregion.Value, source DataSource, query Query, colorID int) (*masterdata.Costume3d, error) {
	if c.preview3D == nil {
		return nil, fmt.Errorf("3d preview service is not configured")
	}
	rawID, err := c.preview3D.AccessoryCostume3DIDForRole(c.ctx, region.String(), query.AccessoryID, colorID, query.Character3DID)
	if err != nil {
		return nil, err
	}
	return source.GetCostumeByID(rawID)
}

func (c *Controller) resolveNormalizedOutfit(region renderregion.Value, source DataSource, query Query, characterID int, colorID int) (*masterdata.Costume3d, error) {
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
	lookup := singleCostumeLookup{controller: c, region: region, source: source, query: query}
	if err := lookup.prepare(); err != nil {
		return nil, err
	}
	if err := lookup.loadItems(); err != nil {
		return nil, err
	}
	if err := lookup.loadLogicalIDs(); err != nil {
		return nil, err
	}
	lookup.keepUsableItems()
	lookup.keepNameMatches()
	return lookup.resolve()
}

type singleCostumeLookup struct {
	controller *Controller
	region     renderregion.Value
	source     DataSource
	query      Query
	partType   string
	named      bool
	filter     Filter
	items      []*masterdata.Costume3d
	logicalIDs map[int][]int
}

func (s *singleCostumeLookup) prepare() error {
	lookup := ListQuery{Query: s.query.Query}
	s.partType, s.named = normalizePartType(s.query.ExpectedPartType)
	s.named = s.named && s.query.Character3DID > 0
	if s.named {
		lookup = ListQuery{PartType: s.partType, Character3DID: s.query.Character3DID, Keyword: strings.TrimSpace(s.query.Query)}
	}
	parsed, err := normalizeListQuery(lookup)
	if err != nil {
		return err
	}
	s.filter, err = s.controller.buildFilter(parsed)
	if err != nil {
		return err
	}
	s.adjustFilter()
	return nil
}

func (s *singleCostumeLookup) adjustFilter() {
	if s.named && (s.controller.preview3D != nil || s.partType == "head" || s.partType == "hair") {
		s.filter.CharacterID = 0
		s.filter.CharacterIDs = nil
	}
	s.filter.ColorID = normalizedCostumeColorID(s.query.ColorID)
	if !s.named {
		s.filter.Limit = 2
	}
}

func (s *singleCostumeLookup) loadItems() error {
	items, err := s.source.FilterCostumes(s.filter)
	s.items = items
	return err
}

func (s *singleCostumeLookup) loadLogicalIDs() error {
	s.logicalIDs = make(map[int][]int)
	if !s.named {
		return nil
	}
	if (s.partType == "head" || s.partType == "hair") && s.controller.preview3D == nil {
		return fmt.Errorf("3d preview service is not configured")
	}
	if s.controller.preview3D == nil {
		return nil
	}
	return s.fetchLogicalIDs()
}

func (s *singleCostumeLookup) fetchLogicalIDs() error {
	switch s.partType {
	case "body":
		ids, err := s.controller.preview3D.OutfitIDsForRole(s.controller.ctx, s.region.String(), s.query.Character3DID)
		s.logicalIDs = wrapCostumeLogicalIDs(ids)
		return err
	case "head":
		ids, err := s.controller.preview3D.AccessoryIDsForRole(s.controller.ctx, s.region.String(), s.query.Character3DID)
		s.logicalIDs = ids
		return err
	case "hair":
		ids, err := s.controller.preview3D.HairIDsForRole(s.controller.ctx, s.region.String(), s.query.Character3DID)
		s.logicalIDs = wrapCostumeLogicalIDs(ids)
		return err
	default:
		return nil
	}
}

func wrapCostumeLogicalIDs(ids map[int]int) map[int][]int {
	wrapped := make(map[int][]int, len(ids))
	for rawID, logicalID := range ids {
		wrapped[rawID] = []int{logicalID}
	}
	return wrapped
}

func (s *singleCostumeLookup) keepUsableItems() {
	if !s.named || len(s.logicalIDs) == 0 {
		return
	}
	usable := s.items[:0]
	for _, item := range s.items {
		if item != nil && len(s.logicalIDs[item.ID]) > 0 {
			usable = append(usable, item)
		}
	}
	s.items = usable
}

func (s *singleCostumeLookup) keepNameMatches() {
	if !s.named {
		return
	}
	needle := strings.TrimSpace(s.query.Query)
	nameMatches, exactMatches := matchCostumeNames(s.items, needle)
	s.items = nameMatches
	if len(exactMatches) > 0 {
		s.items = exactMatches
	}
}

func matchCostumeNames(items []*masterdata.Costume3d, needle string) ([]*masterdata.Costume3d, []*masterdata.Costume3d) {
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
	return nameMatches, exactMatches
}

func (s *singleCostumeLookup) resolve() (*masterdata.Costume3d, error) {
	if len(s.items) == 0 {
		return nil, s.noMatchError()
	}
	if s.named {
		return s.resolveNamed()
	}
	if len(s.items) > 1 {
		return nil, ambiguousCostumeError()
	}
	return s.items[0], nil
}

func (s *singleCostumeLookup) noMatchError() error {
	needle := strings.TrimSpace(s.query.Query)
	if s.named {
		return fmt.Errorf("找不到角色ID %d 的%s名称“%s”", s.query.Character3DID, partTypeName(s.partType), needle)
	}
	return fmt.Errorf("no costume matched %q", needle)
}

func (s *singleCostumeLookup) resolveNamed() (*masterdata.Costume3d, error) {
	ids := s.uniqueLogicalIDs()
	if len(ids) == 1 && s.controller.preview3D != nil {
		return s.resolveLogicalID(ids[0])
	}
	if len(ids) > 1 {
		return nil, fmt.Errorf("角色ID %d 匹配到多个%s“%s”（ID：%s），请明确填写组件ID", s.query.Character3DID, partTypeName(s.partType), strings.TrimSpace(s.query.Query), joinCostumeIDs(ids))
	}
	if len(s.items) > 1 && len(ids) == 1 {
		sort.Slice(s.items, func(i, j int) bool { return s.items[i].ID < s.items[j].ID })
		return s.items[0], nil
	}
	if len(s.items) > 1 {
		return nil, ambiguousCostumeError()
	}
	return s.items[0], nil
}

func (s *singleCostumeLookup) uniqueLogicalIDs() []int {
	unique := make(map[int]struct{})
	for _, item := range s.items {
		for _, logicalID := range s.logicalIDsForItem(item) {
			if logicalID > 0 {
				unique[logicalID] = struct{}{}
			}
		}
	}
	ids := make([]int, 0, len(unique))
	for id := range unique {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	return ids
}

func (s *singleCostumeLookup) logicalIDsForItem(item *masterdata.Costume3d) []int {
	if s.partType == "body" && len(s.logicalIDs) == 0 {
		return []int{normalizedOutfitID(item)}
	}
	return s.logicalIDs[item.ID]
}

func (s *singleCostumeLookup) resolveLogicalID(logicalID int) (*masterdata.Costume3d, error) {
	rawID, err := s.rawCostumeID(logicalID)
	if err != nil {
		return nil, err
	}
	return s.source.GetCostumeByID(rawID)
}

func (s *singleCostumeLookup) rawCostumeID(logicalID int) (int, error) {
	switch s.partType {
	case "body":
		return s.controller.preview3D.OutfitCostume3DIDForRole(s.controller.ctx, s.region.String(), logicalID, s.filter.ColorID, s.query.Character3DID)
	case "head":
		return s.controller.preview3D.AccessoryCostume3DIDForRole(s.controller.ctx, s.region.String(), logicalID, s.filter.ColorID, s.query.Character3DID)
	case "hair":
		return s.controller.preview3D.HairCostume3DIDForRole(s.controller.ctx, s.region.String(), logicalID, s.query.Character3DID)
	default:
		return 0, fmt.Errorf("unsupported costume part type %q", s.partType)
	}
}

func ambiguousCostumeError() error {
	return fmt.Errorf("matched multiple costumes; use costume list first and query by costume id")
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
	state := namedLookupParseState{nameFields: make([]string, 0, len(fields))}
	for index := 0; index < len(fields); index++ {
		consumed, err := state.apply(fields, index)
		if err != nil {
			return Query{}, true, err
		}
		index += consumed
	}
	return state.finish(partType)
}

type namedLookupParseState struct {
	nameFields []string
	roleID     int
	roleSet    bool
	roleAlias  character3DAliasSelection
}

func (s *namedLookupParseState) apply(fields []string, index int) (int, error) {
	token := fields[index]
	lower := strings.ToLower(strings.TrimSpace(token))
	if label, id, ok := parseComboLabeledID(lower); ok && label == "role" {
		return 0, s.setRole(id)
	}
	if label, ok := normalizeComboLabel(lower); ok && label == "role" {
		if index+1 >= len(fields) {
			return 0, fmt.Errorf("角色后必须填写1到31之间的ID")
		}
		return 1, s.setRoleToken(fields[index+1])
	}
	s.nameFields = append(s.nameFields, token)
	return 0, nil
}

func (s *namedLookupParseState) setRole(id int) error {
	if s.roleSet {
		return fmt.Errorf("角色ID重复")
	}
	s.roleID = id
	s.roleSet = true
	return nil
}

func (s *namedLookupParseState) setRoleToken(token string) error {
	if id, ok := ParseExplicitCostumeID(token); ok {
		return s.setRole(id)
	}
	if characterID, ok := parseCharacter3DAliasToken(token); ok {
		return s.roleAlias.setCharacter(characterID)
	}
	return fmt.Errorf("角色后必须填写1到31之间的ID或精确角色名称")
}

func (s *namedLookupParseState) finish(partType string) (Query, bool, error) {
	s.applyMikuUnitSuffix()
	s.applyTrailingRole()
	if s.roleAlias.characterID != 0 {
		if err := s.roleAlias.apply(&s.roleID); err != nil {
			return Query{}, true, err
		}
		s.roleSet = true
	}
	if !s.roleSet {
		return Query{}, false, nil
	}
	if s.roleID < 1 || s.roleID > 31 {
		return Query{}, true, fmt.Errorf("角色ID必须在1到31之间")
	}
	name := strings.TrimSpace(strings.Join(s.nameFields, " "))
	if name == "" {
		return Query{}, true, fmt.Errorf("请在角色ID之外填写组件名称")
	}
	return Query{Query: name, ExpectedPartType: partType, Character3DID: s.roleID, ColorID: 1}, true, nil
}

func (s *namedLookupParseState) applyMikuUnitSuffix() {
	if s.roleAlias.characterID != 21 || len(s.nameFields) == 0 {
		return
	}
	last := len(s.nameFields) - 1
	if unit, ok := parseCostumeUnitAlias(s.nameFields[last]); ok {
		s.roleAlias.setUnit(unit)
		s.nameFields = s.nameFields[:last]
	}
}

func (s *namedLookupParseState) applyTrailingRole() {
	if s.roleSet || s.roleAlias.characterID != 0 || len(s.nameFields) == 0 {
		return
	}
	last := len(s.nameFields) - 1
	if characterID, ok := parseCharacter3DAliasToken(s.nameFields[last]); ok {
		_ = s.roleAlias.setCharacter(characterID)
		s.nameFields = s.nameFields[:last]
		return
	}
	s.applyTrailingMikuUnit(last)
}

func (s *namedLookupParseState) applyTrailingMikuUnit(last int) {
	if last <= 0 {
		return
	}
	unit, ok := parseCostumeUnitAlias(s.nameFields[last])
	if !ok {
		return
	}
	characterID, ok := parseCharacter3DAliasToken(s.nameFields[last-1])
	if !ok || characterID != 21 {
		return
	}
	_ = s.roleAlias.setCharacter(characterID)
	s.roleAlias.setUnit(unit)
	s.nameFields = s.nameFields[:last-1]
}

func (c *Controller) buildFilter(query ListQuery) (Filter, error) {
	filter := Filter{
		PartType: query.PartType,
		Keyword:  strings.TrimSpace(query.Keyword),
	}
	if err := applyCostumeCharacterFilter(query, &filter); err != nil {
		return Filter{}, err
	}
	if filter.CharacterID == 0 && strings.TrimSpace(query.Character) != "" {
		filter.Keyword = compactQuery(filter.Keyword, query.Character)
	}
	filter.CharacterIDs = characterIDsForGender(query.Gender)
	if filter.PartType == "" && len(filter.CharacterIDs) > 0 {
		filter.PartType = "body"
	}
	if !costumeCharacterFiltersOverlap(filter.CharacterID, filter.CharacterIDs) {
		return Filter{}, fmt.Errorf("character and gender filters do not overlap")
	}
	return filter, nil
}

func applyCostumeCharacterFilter(query ListQuery, filter *Filter) error {
	if query.Character3DID > 0 {
		characterID, ok := characterIDFor3DRole(query.Character3DID)
		if !ok {
			return fmt.Errorf("角色ID必须在1到31之间")
		}
		filter.CharacterID = characterID
	} else if characterID, ok := resolveCharacterID(query.Character); ok {
		filter.CharacterID = characterID
	}
	return nil
}

func costumeCharacterFiltersOverlap(characterID int, characterIDs []int) bool {
	if characterID <= 0 || len(characterIDs) == 0 {
		return true
	}
	for _, candidate := range characterIDs {
		if candidate == characterID {
			return true
		}
	}
	return false
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
	if !ok {
		return Query{}, false, fmt.Errorf("查询类型必须是服装、饰品或发型")
	}
	fields := strings.Fields(strings.TrimSpace(raw))
	if len(fields) == 0 {
		return Query{}, false, nil
	}
	if !lookupQueryContainsComponentID(fields) {
		return Query{}, false, nil
	}
	state := lookupQueryParseState{
		query:    Query{Query: raw, ExpectedPartType: partType, ColorID: 1},
		partType: partType,
	}
	for _, token := range fields {
		handled, err := state.apply(token)
		if err != nil {
			return Query{}, true, err
		}
		if !handled && !state.recognized {
			return Query{}, false, nil
		}
		if !handled {
			return Query{}, true, fmt.Errorf("无法识别查询参数：%s", token)
		}
	}
	query, err := state.finish()
	return query, true, err
}

type lookupQueryParseState struct {
	query       Query
	partType    string
	recognized  bool
	colorSet    bool
	pendingRole bool
	roleAlias   character3DAliasSelection
}

func (s *lookupQueryParseState) apply(token string) (bool, error) {
	lower := strings.ToLower(strings.TrimSpace(token))
	if s.pendingRole {
		return true, s.applyPendingRole(lower)
	}
	if label, id, ok := parseComboLabeledID(lower); ok {
		s.recognized = true
		return true, s.applyLabeledID(label, id, token)
	}
	if label, ok := normalizeComboLabel(lower); ok && label == "role" {
		s.recognized = true
		s.pendingRole = true
		return true, nil
	}
	if characterID, ok := parseCharacter3DAliasToken(lower); ok {
		return true, s.roleAlias.setCharacter(characterID)
	}
	if id, ok := ParseExplicitCostumeID(lower); ok {
		s.recognized = true
		return true, s.applyNumericID(id)
	}
	if unit, ok := parseCostumeUnitAlias(lower); ok {
		s.roleAlias.setUnit(unit)
		return true, nil
	}
	return false, nil
}

func (s *lookupQueryParseState) applyPendingRole(token string) error {
	if characterID, ok := parseCharacter3DAliasToken(token); ok {
		s.pendingRole = false
		return s.roleAlias.setCharacter(characterID)
	}
	id, ok := ParseExplicitCostumeID(token)
	if !ok {
		return fmt.Errorf("角色后必须填写1到31之间的ID或精确角色名称")
	}
	if s.query.Character3DID != 0 {
		return fmt.Errorf("角色ID重复")
	}
	s.query.Character3DID = id
	s.pendingRole = false
	return nil
}

func (s *lookupQueryParseState) applyLabeledID(label string, id int, original string) error {
	switch label {
	case "outfit":
		if s.partType != "body" || s.query.OutfitID != 0 {
			return fmt.Errorf("服装查询参数重复或类型不匹配")
		}
		s.query.OutfitID = id
	case "accessory":
		if s.partType != "head" || s.query.AccessoryID != 0 {
			return fmt.Errorf("饰品查询参数重复或类型不匹配")
		}
		s.query.AccessoryID = id
	case "hair":
		if s.partType != "hair" || s.query.HairID != 0 {
			return fmt.Errorf("发型查询参数重复或类型不匹配")
		}
		s.query.HairID = id
	case "role":
		if s.query.Character3DID != 0 {
			return fmt.Errorf("角色ID重复")
		}
		s.query.Character3DID = id
	case "color", "outfit_color", "accessory_color":
		if s.colorSet {
			return fmt.Errorf("颜色ID重复")
		}
		s.query.ColorID = id
		s.colorSet = true
	default:
		return fmt.Errorf("组件查询不接受%s参数", original)
	}
	return nil
}

func (s *lookupQueryParseState) applyNumericID(id int) error {
	if s.shortID() == 0 {
		s.setShortID(id)
		return nil
	}
	if s.query.Character3DID == 0 && s.roleAlias.characterID == 0 {
		s.query.Character3DID = id
		return nil
	}
	if !s.colorSet {
		s.query.ColorID = id
		s.colorSet = true
		return nil
	}
	return fmt.Errorf("查询参数过多")
}

func (s *lookupQueryParseState) shortID() int {
	switch s.partType {
	case "head":
		return s.query.AccessoryID
	case "hair":
		return s.query.HairID
	default:
		return s.query.OutfitID
	}
}

func (s *lookupQueryParseState) setShortID(id int) {
	switch s.partType {
	case "head":
		s.query.AccessoryID = id
	case "hair":
		s.query.HairID = id
	default:
		s.query.OutfitID = id
	}
}

func (s *lookupQueryParseState) finish() (Query, error) {
	if s.pendingRole {
		return Query{}, fmt.Errorf("角色后必须填写1到31之间的ID或精确角色名称")
	}
	if s.shortID() <= 0 {
		return Query{}, fmt.Errorf("请填写%sID", costumePartLabel(s.partType))
	}
	if err := s.roleAlias.apply(&s.query.Character3DID); err != nil {
		return Query{}, err
	}
	if s.query.Character3DID < 1 || s.query.Character3DID > 31 {
		return Query{}, fmt.Errorf("请填写1到31之间的角色ID或精确角色名称")
	}
	if s.query.ColorID < 1 || s.query.ColorID > 4 {
		return Query{}, fmt.Errorf("颜色ID必须在1到4之间，颜色1为原版")
	}
	return s.query, nil
}

func costumePartLabel(partType string) string {
	switch partType {
	case "head":
		return "饰品"
	case "hair":
		return "发型"
	default:
		return "服装"
	}
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
	state := comboQueryParseState{parsed: query}
	for _, token := range strings.Fields(query.Query) {
		if err := state.apply(token); err != nil {
			return ComboQuery{}, err
		}
	}
	return state.finish()
}

type comboQueryParseState struct {
	parsed          ComboQuery
	pending         string
	lastColorTarget string
	roleAlias       character3DAliasSelection
}

func (s *comboQueryParseState) apply(token string) error {
	lower := strings.ToLower(strings.TrimSpace(token))
	if lower == "" {
		return nil
	}
	if s.pending == "role" {
		if characterID, ok := parseCharacter3DAliasToken(lower); ok {
			if err := s.roleAlias.setCharacter(characterID); err != nil {
				return err
			}
			s.pending = ""
			s.lastColorTarget = ""
			return nil
		}
	}
	if s.pending == "" {
		if handled, err := s.applyRoleAlias(lower); handled {
			return err
		}
	}
	if label, id, ok := parseComboLabeledID(lower); ok {
		if err := assignComboValue(&s.parsed, label, id, &s.lastColorTarget); err != nil {
			return err
		}
		s.pending = ""
		return nil
	}
	if label, ok := normalizeComboLabel(lower); ok {
		s.pending = label
		return nil
	}
	if id, ok := ParseExplicitCostumeID(lower); ok {
		return s.applyExplicitID(token, id)
	}
	return fmt.Errorf("无法识别组合参数：%s", token)
}

func (s *comboQueryParseState) applyRoleAlias(token string) (bool, error) {
	if characterID, ok := parseCharacter3DAliasToken(token); ok {
		s.lastColorTarget = ""
		return true, s.roleAlias.setCharacter(characterID)
	}
	unit, ok := parseCostumeUnitAlias(token)
	if !ok {
		return false, nil
	}
	_, numeric := ParseExplicitCostumeID(token)
	if numeric && s.lastColorTarget != "" {
		return false, nil
	}
	s.roleAlias.setUnit(unit)
	s.lastColorTarget = ""
	return true, nil
}

func (s *comboQueryParseState) applyExplicitID(original string, id int) error {
	if s.pending != "" {
		if err := assignComboValue(&s.parsed, s.pending, id, &s.lastColorTarget); err != nil {
			return err
		}
		s.pending = ""
		return nil
	}
	if s.lastColorTarget != "" {
		if err := assignComboColor(&s.parsed, s.lastColorTarget, id); err != nil {
			return err
		}
		s.lastColorTarget = ""
		return nil
	}
	return fmt.Errorf("组合参数 %s 缺少服装、饰品、发型或角色标签", original)
}

func (s *comboQueryParseState) finish() (ComboQuery, error) {
	if s.pending != "" {
		if s.pending == "role" {
			return ComboQuery{}, fmt.Errorf("组合参数角色缺少ID或精确角色名称")
		}
		return ComboQuery{}, fmt.Errorf("组合参数 %s 缺少 ID", s.pending)
	}
	if err := s.roleAlias.apply(&s.parsed.Character3DID); err != nil {
		return ComboQuery{}, err
	}
	if s.parsed.Character3DID < 1 || s.parsed.Character3DID > 31 {
		return ComboQuery{}, fmt.Errorf("组合必须填写1到31之间的角色ID或精确角色名称")
	}
	if s.parsed.OutfitColorID == 0 {
		s.parsed.OutfitColorID = 1
	}
	if s.parsed.AccessoryColorID == 0 {
		s.parsed.AccessoryColorID = 1
	}
	return s.parsed, nil
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
		return assignComboOutfit(query, id, lastColorTarget)
	case "role":
		return assignComboRole(query, id, lastColorTarget)
	case "color":
		return assignPendingComboColor(query, id, lastColorTarget)
	case "outfit_color":
		return assignComboColor(query, "outfit", id)
	case "accessory_color":
		return assignComboColor(query, "accessory", id)
	case "hair":
		return assignComboHair(query, id, lastColorTarget)
	case "accessory":
		return assignComboAccessory(query, id, lastColorTarget)
	default:
		return fmt.Errorf("无法识别组合部件类型：%s", label)
	}
}

func assignComboOutfit(query *ComboQuery, id int, lastColorTarget *string) error {
	if query.OutfitID != 0 {
		return fmt.Errorf("组合里重复指定服装")
	}
	query.OutfitID = id
	*lastColorTarget = "outfit"
	return nil
}

func assignComboRole(query *ComboQuery, id int, lastColorTarget *string) error {
	if id > 31 {
		return fmt.Errorf("角色ID必须在1到31之间")
	}
	if query.Character3DID != 0 {
		return fmt.Errorf("组合里重复指定角色")
	}
	query.Character3DID = id
	*lastColorTarget = ""
	return nil
}

func assignPendingComboColor(query *ComboQuery, id int, lastColorTarget *string) error {
	if *lastColorTarget == "" {
		return fmt.Errorf("颜色ID必须紧跟在服装或饰品后面")
	}
	if err := assignComboColor(query, *lastColorTarget, id); err != nil {
		return err
	}
	*lastColorTarget = ""
	return nil
}

func assignComboHair(query *ComboQuery, id int, lastColorTarget *string) error {
	if query.HairID != 0 || query.HairCostume3DID != 0 {
		return fmt.Errorf("组合里重复指定发型")
	}
	query.HairID = id
	*lastColorTarget = ""
	return nil
}

func assignComboAccessory(query *ComboQuery, id int, lastColorTarget *string) error {
	if query.AccessoryID != 0 {
		return fmt.Errorf("组合里重复指定饰品")
	}
	query.AccessoryID = id
	*lastColorTarget = "accessory"
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
	fields := strings.Fields(query.Query)
	state := listQueryParseState{parsed: query, hasMikuAlias: listQueryHasMikuAlias(fields)}
	for _, token := range fields {
		if err := state.apply(token); err != nil {
			return ListQuery{}, err
		}
	}
	return state.finish(query)
}

type listQueryParseState struct {
	parsed       ListQuery
	keywords     []string
	partLocked   bool
	pendingRole  bool
	hasMikuAlias bool
	roleAlias    character3DAliasSelection
}

func listQueryHasMikuAlias(fields []string) bool {
	for _, token := range fields {
		if characterID, ok := parseCharacter3DAliasToken(token); ok && characterID == 21 {
			return true
		}
	}
	return false
}

func (s *listQueryParseState) apply(token string) error {
	lower := strings.ToLower(strings.TrimSpace(token))
	if lower == "" {
		return nil
	}
	if s.pendingRole {
		return s.applyPendingRole(lower)
	}
	if s.applyGenderPart(lower) || s.applyPage(lower) || s.applyPageSize(lower) || s.applyPart(lower) || s.applyGender(lower) {
		return nil
	}
	if handled, err := s.applyRole(lower); handled {
		return err
	}
	if value, err := strconv.Atoi(lower); err == nil && value > 0 && value <= 31 {
		s.parsed.Character = lower
		return nil
	}
	if unit, ok := parseCostumeUnitAlias(lower); ok && s.hasMikuAlias {
		s.roleAlias.setUnit(unit)
		return nil
	}
	s.keywords = append(s.keywords, token)
	return nil
}

func (s *listQueryParseState) applyPendingRole(token string) error {
	if characterID, ok := parseCharacter3DAliasToken(token); ok {
		s.pendingRole = false
		return s.roleAlias.setCharacter(characterID)
	}
	id, ok := ParseExplicitCostumeID(token)
	if !ok {
		return fmt.Errorf("角色后必须填写1到31之间的ID或精确角色名称")
	}
	if s.parsed.Character3DID != 0 {
		return fmt.Errorf("角色ID重复")
	}
	s.parsed.Character3DID = id
	s.pendingRole = false
	return nil
}

func (s *listQueryParseState) applyGenderPart(token string) bool {
	gender, partType, ok := normalizeGenderPart(token)
	if !ok {
		return false
	}
	s.parsed.Gender = gender
	s.parsed.PartType = partType
	s.partLocked = true
	return true
}

func (s *listQueryParseState) applyPage(token string) bool {
	page, ok := parsePageToken(token)
	if ok {
		s.parsed.Page = page
	}
	return ok
}

func (s *listQueryParseState) applyPageSize(token string) bool {
	pageSize, ok := parsePageSizeToken(token)
	if ok {
		s.parsed.PageSize = pageSize
	}
	return ok
}

func (s *listQueryParseState) applyPart(token string) bool {
	partType, ok := normalizePartType(token)
	if !ok {
		return false
	}
	if !s.partLocked {
		s.parsed.PartType = partType
	}
	return true
}

func (s *listQueryParseState) applyGender(token string) bool {
	gender, ok := normalizeGender(token)
	if ok {
		s.parsed.Gender = gender
	}
	return ok
}

func (s *listQueryParseState) applyRole(token string) (bool, error) {
	if label, id, ok := parseComboLabeledID(token); ok && label == "role" {
		if s.parsed.Character3DID != 0 {
			return true, fmt.Errorf("角色ID重复")
		}
		s.parsed.Character3DID = id
		return true, nil
	}
	if label, ok := normalizeComboLabel(token); ok && label == "role" {
		s.pendingRole = true
		return true, nil
	}
	characterID, ok := parseCharacter3DAliasToken(token)
	if !ok {
		return false, nil
	}
	return true, s.roleAlias.setCharacter(characterID)
}

func (s *listQueryParseState) finish(query ListQuery) (ListQuery, error) {
	if s.pendingRole {
		return ListQuery{}, fmt.Errorf("角色后必须填写1到31之间的ID或精确角色名称")
	}
	if s.roleAlias.characterID != 0 {
		if err := s.roleAlias.apply(&s.parsed.Character3DID); err != nil {
			return ListQuery{}, err
		}
	}
	s.applyExplicitFilters(query)
	if strings.TrimSpace(query.Keyword) != "" {
		s.keywords = append(s.keywords, query.Keyword)
	}
	s.parsed.Keyword = strings.TrimSpace(strings.Join(s.keywords, " "))
	return s.parsed, nil
}

func (s *listQueryParseState) applyExplicitFilters(query ListQuery) {
	if explicitPart, ok := normalizePartType(query.PartType); ok {
		s.parsed.PartType = explicitPart
	}
	if explicitGender, explicitPart, ok := normalizeGenderPart(query.Gender); ok {
		s.parsed.Gender = explicitGender
		if s.parsed.PartType == "" {
			s.parsed.PartType = explicitPart
		}
	} else if explicitGender, ok := normalizeGender(query.Gender); ok {
		s.parsed.Gender = explicitGender
	}
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
