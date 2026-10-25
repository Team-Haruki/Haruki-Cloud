package mysekai

import (
	"cmp"
	"math"
	"slices"
	"strings"
	"time"

	"haruki-cloud/internal/i18n"
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/utils/usererror"
)

type ConvertRecipeQuery struct {
	Region string `json:"region,omitempty"`
	Query  string `json:"query,omitempty"`
}

type convertRecipeEntry struct {
	categorySeq int
	recipe      drawing.MysekaiConvertRecipe
}

func (c *Controller) BuildConvertRecipeRequest(query ConvertRecipeQuery) (*drawing.MysekaiConvertRecipesRequest, error) {
	return c.buildConvertRecipeRequest(query, time.Now())
}

func (c *Controller) buildConvertRecipeRequest(query ConvertRecipeQuery, now time.Time) (*drawing.MysekaiConvertRecipesRequest, error) {
	c = c.withRegion(query.Region)
	if err := c.ensureMasterdata(); err != nil {
		return nil, err
	}
	rows := c.masterdata.loadList("mysekaiConvertItems.json")
	if len(rows) == 0 {
		return nil, usererror.New(usererror.CodeUnavailable, i18n.M("convert.error.data_unavailable"))
	}
	region := c.resolveRegion(query.Region)
	catalog := loadConvertRecipeCatalog(c.masterdata)
	catalog.resolveImage = func(path string) string { return c.regionPath(region, path) }
	keyword := convertRecipeText(query.Query)
	entries := make([]convertRecipeEntry, 0, len(rows))
	for _, row := range rows {
		id := intNumber(row["id"], 0)
		start, end := int64Number(row["startAt"], 0), int64Number(row["endAt"], 0)
		if id <= 0 || start > now.UnixMilli() || (end > 0 && end <= now.UnixMilli()) {
			continue
		}
		entry := catalog.entry(row)
		if catalog.matchesOutput(entry, row, keyword) {
			entries = append(entries, entry)
		}
	}
	if len(entries) == 0 {
		if keyword == "" {
			return nil, usererror.New(usererror.CodeNotFound, i18n.M("convert.error.none_current"))
		}
		return nil, usererror.New(usererror.CodeNotFound, i18n.M("convert.error.not_found"))
	}
	slices.SortFunc(entries, func(a, b convertRecipeEntry) int {
		if a.categorySeq != b.categorySeq {
			return cmp.Compare(a.categorySeq, b.categorySeq)
		}
		return cmp.Compare(a.recipe.ID, b.recipe.ID)
	})
	request := &drawing.MysekaiConvertRecipesRequest{
		Title:   i18n.T("convert.image.title", i18n.Data{"Region": i18n.RegionLabel(region.String())}),
		Total:   len(entries),
		Filter:  keyword,
		Recipes: make([]drawing.MysekaiConvertRecipe, 0, len(entries)),
	}
	for _, entry := range entries {
		request.Recipes = append(request.Recipes, entry.recipe)
	}
	request.Text = convertRecipeRequestText(request)
	return request, nil
}

func convertRecipeRequestText(request *drawing.MysekaiConvertRecipesRequest) string {
	lines := []i18n.Message{
		i18n.Verbatim(request.Title),
		i18n.M("convert.text.total", i18n.Data{"Count": request.Total}),
		i18n.M("convert.image.note"),
	}
	if request.Filter != "" {
		lines = append(lines, i18n.M("convert.text.filter", i18n.Data{"UserQuery": i18n.UserText(request.Filter)}))
	}
	for _, recipe := range request.Recipes {
		lines = append(lines, i18n.M("convert.text.recipe", i18n.Data{
			"ID": recipe.ID, "Name": recipe.Name, "Category": recipe.Category,
			"Inputs": convertRecipeResourcesText(recipe.Inputs), "Outputs": convertRecipeResourcesText(recipe.Outputs),
			"Duration": recipe.Duration, "Level": convertRecipeLevel(recipe.RequiredLevel), "Limits": recipe.Limits,
		}))
	}
	return i18n.LinesText(lines)
}

func convertRecipeCategoryName(category string) string {
	switch category {
	case "normal":
		return i18n.T("convert.category.normal")
	case "mysekai_material":
		return i18n.T("convert.category.mysekai_material")
	case "mysekai_blueprint":
		return i18n.T("convert.category.blueprint")
	case "mysekai_music_record":
		return i18n.T("convert.category.music_record")
	}
	return ""
}

func convertRecipeDuration(row map[string]any) string {
	minutes := intNumber(row["convertMinutes"], -1)
	if minutes < 0 || int64(minutes) > math.MaxInt64/int64(time.Minute) {
		return i18n.T("convert.image.unknown")
	}
	return i18n.FormatDuration(time.Duration(minutes) * time.Minute).String()
}

func convertRecipeLevel(level int) string {
	if level <= 0 {
		return i18n.T("convert.image.unknown")
	}
	return i18n.T("convert.image.level", i18n.Data{"Level": level})
}

func convertRecipeLimits(row map[string]any) string {
	limit := intNumber(row["convertLimit"], 0)
	total := i18n.T("convert.limit.unlimited")
	if limit > 0 {
		switch cycle := stringValue(row["refreshCycle"]); cycle {
		case "none", "":
			total = i18n.T("convert.limit.total", i18n.Data{"Count": limit})
		case "daily":
			total = i18n.T("convert.limit.daily", i18n.Data{"Count": limit})
		case "weekly":
			total = i18n.T("convert.limit.weekly", i18n.Data{"Count": limit})
		case "monthly":
			total = i18n.T("convert.limit.monthly", i18n.Data{"Count": limit})
		default:
			total = i18n.T("convert.limit.cycle", i18n.Data{"Cycle": cycle, "Count": limit})
		}
	}
	if once := intNumber(row["onceConvertLimit"], 0); once > 0 {
		return i18n.T("convert.limit.combined", i18n.Data{"Count": once, "Total": total})
	}
	return total
}

func convertRecipeText(value string) string {
	return strings.Join(strings.Fields(value), " ")
}
