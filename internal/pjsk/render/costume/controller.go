package costume

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"haruki-cloud/internal/pjsk/drawing"
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
	c.preview3D = NewPreview3DService(cfg)
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
	parsed := normalizeListQuery(query)
	filter, err := c.buildFilter(parsed)
	if err != nil {
		return nil, err
	}
	filter.ColorID = 1

	items, err := source.FilterCostumes(filter)
	if err != nil {
		return nil, err
	}
	sortCostumesForDisplay(items)

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
	pageItems, totalPages := paginateCostumeListItems(items, parsed, pageSize, page)
	if page > totalPages {
		page = totalPages
	}

	sourceCards, err := c.sourceCardsForCostumes(source, pageItems)
	if err != nil {
		return nil, err
	}
	costumes := make([]drawing.CostumeBasic, 0, len(pageItems))
	for _, item := range pageItems {
		basic := c.buildCostumeBasic(region, source, item, nil, sourceCards)
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

func shouldBalanceCostumeListByPart(query ListQuery) bool {
	return strings.TrimSpace(query.PartType) == "" && strings.TrimSpace(query.Character) != ""
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
	costumeID := query.ID
	if costumeID <= 0 {
		costumeID, _ = ParseExplicitCostumeID(query.Query)
	}
	var costumeInfo *masterdata.Costume3d
	if costumeID > 0 {
		costumeInfo, err = source.GetCostumeByID(costumeID)
		if err != nil {
			return nil, err
		}
	} else {
		costumeInfo, err = c.resolveSingleCostumeByQuery(source, query.Query)
		if err != nil {
			return nil, err
		}
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
	if previewPath, err := c.resolve3DPreviewPath(region, costumeInfo); err != nil {
		costumePreview3DLogger.Warnf("3d preview skipped: region=%s costume_id=%d err=%v", region.String(), costumeInfo.ID, err)
	} else if previewPath != "" {
		costumeBasic.PreviewImagePath = &previewPath
	}
	character, _ := source.GetCharacterByID(costumeInfo.CharacterID)
	return &drawing.CostumeDetailRequest{
		Region:            region.String(),
		Costume:           costumeBasic,
		CharacterIconPath: c.buildCharacterIconPath(costumeInfo.CharacterID, characterUnit(character)),
		UnitLogoPath:      c.buildUnitLogoPath(characterUnit(character)),
	}, nil
}

func (c *Controller) resolve3DPreviewPath(region renderregion.Value, costumeInfo *masterdata.Costume3d) (string, error) {
	if c == nil || c.preview3D == nil || costumeInfo == nil {
		return "", nil
	}
	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return c.preview3D.ResolvePreviewPath(ctx, region.String(), costumeInfo.ID)
}

func (c *Controller) RenderCostumeDetail(query Query) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildCostumeDetailRequest(query)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateCostumeDetail(payload)
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

func (c *Controller) resolveSingleCostumeByQuery(source DataSource, raw string) (*masterdata.Costume3d, error) {
	parsed := normalizeListQuery(ListQuery{Query: raw})
	filter, err := c.buildFilter(parsed)
	if err != nil {
		return nil, err
	}
	filter.ColorID = 1
	filter.Limit = 2
	items, err := source.FilterCostumes(filter)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("no costume matched %q", strings.TrimSpace(raw))
	}
	if len(items) > 1 {
		return nil, fmt.Errorf("matched multiple costumes; use costume list first and query by costume id")
	}
	return items[0], nil
}

func (c *Controller) buildFilter(query ListQuery) (Filter, error) {
	filter := Filter{
		PartType: query.PartType,
		Keyword:  strings.TrimSpace(query.Keyword),
	}
	if characterID, ok := resolveCharacterID(query.Character); ok {
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

func parseComboQuery(query ComboQuery) (ComboQuery, error) {
	parsed := query
	pending := ""
	ordered := make([]int, 0)
	for _, token := range strings.Fields(query.Query) {
		lower := strings.ToLower(strings.TrimSpace(token))
		if lower == "" {
			continue
		}
		if unit, ok := parseComboUnitToken(lower); ok {
			parsed.Unit = unit
			continue
		}
		if label, id, ok := parseComboLabeledID(lower); ok {
			if err := assignComboID(&parsed, label, id); err != nil {
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
				if err := assignComboID(&parsed, pending, id); err != nil {
					return ComboQuery{}, err
				}
				pending = ""
				continue
			}
			ordered = append(ordered, id)
			continue
		}
		return ComboQuery{}, fmt.Errorf("无法识别组合参数：%s", token)
	}
	if pending != "" {
		return ComboQuery{}, fmt.Errorf("组合参数 %s 缺少 ID", pending)
	}
	for index, id := range ordered {
		switch index {
		case 0:
			if parsed.BodyCostume3DID != 0 {
				return ComboQuery{}, fmt.Errorf("组合里重复指定服装")
			}
			parsed.BodyCostume3DID = id
		case 1:
			if parsed.HairCostume3DID != 0 {
				return ComboQuery{}, fmt.Errorf("组合里重复指定发型")
			}
			parsed.HairCostume3DID = id
		default:
			parsed.AccessoryCostumeIDs = append(parsed.AccessoryCostumeIDs, id)
		}
	}
	if parsed.BodyCostume3DID <= 0 && parsed.HairCostume3DID <= 0 && len(parsed.AccessoryCostumeIDs) == 0 {
		return ComboQuery{}, fmt.Errorf("组合至少需要一个 3D 部件 ID")
	}
	parsed.Unit, _ = parseComboUnitToken(strings.TrimSpace(parsed.Unit))
	return parsed, nil
}

func parseComboLabeledID(token string) (string, int, bool) {
	labels := []string{
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
	case "body", "costume", "服装", "衣装", "衣服":
		return "body", true
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

func assignComboID(query *ComboQuery, label string, id int) error {
	if id <= 0 {
		return fmt.Errorf("组合部件 ID 无效")
	}
	switch label {
	case "body":
		if query.BodyCostume3DID != 0 {
			return fmt.Errorf("组合里重复指定服装")
		}
		query.BodyCostume3DID = id
	case "hair":
		if query.HairCostume3DID != 0 {
			return fmt.Errorf("组合里重复指定发型")
		}
		query.HairCostume3DID = id
	case "accessory":
		query.AccessoryCostumeIDs = append(query.AccessoryCostumeIDs, id)
	default:
		return fmt.Errorf("无法识别组合部件类型：%s", label)
	}
	return nil
}

func parseComboUnitToken(token string) (string, bool) {
	token = strings.TrimSpace(strings.ToLower(token))
	token = strings.TrimPrefix(token, "unit=")
	token = strings.TrimPrefix(token, "unit:")
	token = strings.TrimPrefix(token, "组合=")
	switch token {
	case "light_sound", "ln", "leo_need", "leoneed":
		return "light_sound", true
	case "idol", "mmj", "more_more_jump":
		return "idol", true
	case "street", "vbs", "vivid_bad_squad":
		return "street", true
	case "theme_park", "wxs", "wonderlands_showtime":
		return "theme_park", true
	case "school_refusal", "n25", "25", "25ji", "nightcord":
		return "school_refusal", true
	case "piapro", "vs", "virtual_singer":
		return "piapro", true
	default:
		return "", false
	}
}

func normalizeListQuery(query ListQuery) ListQuery {
	parsed := query
	keywords := make([]string, 0)
	partLocked := false
	for _, token := range strings.Fields(query.Query) {
		lower := strings.ToLower(strings.TrimSpace(token))
		if lower == "" {
			continue
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
		if _, ok := resolveCharacterID(lower); ok {
			parsed.Character = lower
			continue
		}
		keywords = append(keywords, token)
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
	return parsed
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
	sb.WriteString("\n详情：/查服装 ID")
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
	if query.Keyword != "" {
		parts = append(parts, query.Keyword)
	}
	return strings.Join(parts, " / ")
}
