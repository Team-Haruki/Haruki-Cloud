package honor

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"haruki-cloud/internal/pjsk/render/masterdata"
)

// MasterRowSource is the optional DataSource extension that serves generic
// master rows (honorBackgrounds, honorWords, honorGroups). A source without
// it, or a region whose tables are absent, renders honors as before JP
// 7.0.0.
type MasterRowSource interface {
	HonorMasterRows(filename string) (map[int]map[string]any, bool)
}

// honorLayer is one honorBackgrounds / honorWords row.
type honorLayer struct {
	id              int
	seq             int
	groupID         int
	assetBundleName string
}

func (b *Builder) honorMasterRows(filename string) map[int]map[string]any {
	rows, ok := b.source.(MasterRowSource)
	if !ok {
		return nil
	}
	items, served := rows.HonorMasterRows(filename)
	if !served {
		return nil
	}
	return items
}

// resolveHonorLayer mirrors the 7.0.0 client (UIPartsHonorImage): the
// selected row when it belongs to the honor's group, otherwise the group's
// first row by seq; none when the region has no rows for the group.
func (b *Builder) resolveHonorLayer(filename string, groupID int, selectedID *int) (honorLayer, bool) {
	rows := b.honorMasterRows(filename)
	if len(rows) == 0 || groupID <= 0 {
		return honorLayer{}, false
	}
	if selectedID != nil && *selectedID > 0 {
		if layer, ok := parseHonorLayer(rows[*selectedID]); ok && layer.groupID == groupID {
			return layer, true
		}
	}
	candidates := make([]honorLayer, 0, 2)
	for _, row := range rows {
		if layer, ok := parseHonorLayer(row); ok && layer.groupID == groupID {
			candidates = append(candidates, layer)
		}
	}
	if len(candidates) == 0 {
		return honorLayer{}, false
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].seq != candidates[j].seq {
			return candidates[i].seq < candidates[j].seq
		}
		return candidates[i].id < candidates[j].id
	})
	return candidates[0], true
}

func parseHonorLayer(row map[string]any) (honorLayer, bool) {
	if row == nil {
		return honorLayer{}, false
	}
	layer := honorLayer{
		id:              rowInt(row["id"]),
		seq:             rowInt(row["seq"]),
		groupID:         rowInt(row["honorGroupId"]),
		assetBundleName: strings.TrimSpace(rowString(row["assetbundleName"])),
	}
	return layer, layer.id > 0 && layer.assetBundleName != ""
}

// honorGroupMedalDisplayed reads honorGroups.isMedalDisplayed (JP 7.0.0);
// false when the region's rows lack the column.
func (b *Builder) honorGroupMedalDisplayed(groupID int) bool {
	row := b.honorMasterRows("honorGroups.json")[groupID]
	if row == nil {
		return false
	}
	switch v := row["isMedalDisplayed"].(type) {
	case bool:
		return v
	case string:
		parsed, _ := strconv.ParseBool(strings.TrimSpace(v))
		return parsed
	default:
		return rowInt(v) != 0
	}
}

// honorMedalTier mirrors UIPartsHonorMedal: levelled honors of a group with
// isMedalDisplayed show a medal from level 11, one tier per ten levels,
// capped at 9. 0 means no medal.
func honorMedalTier(honorInfo *masterdata.Honor, level int, medalDisplayed bool) int {
	if honorInfo == nil || len(honorInfo.Levels) == 0 || !medalDisplayed || level < 11 {
		return 0
	}
	return min((level-1)/10, 9)
}

// honorBackgroundImagePath is the honor_background bundle's degree image:
// bundle honor_background/<assetbundleName> exported as a directory holding
// degree_main.png / degree_sub.png like honor/<assetbundleName>.
func honorBackgroundImagePath(layer honorLayer, mode string) string {
	return fmt.Sprintf("honor_background/%s/degree_%s.png", layer.assetBundleName, mode)
}

// honorWordImagePath is the honor_word bundle for a rarity: the client loads
// "honor_word/{0}_{1}" with {1} = rarity index + 1 (low 1 .. highest 4, no
// zero padding) and takes the bundle's single texture, assumed exported as
// honor_word/<assetbundleName>_<n>.png.
func honorWordImagePath(layer honorLayer, rarity string) string {
	return fmt.Sprintf("honor_word/%s_%d.png", layer.assetBundleName, mapHonorRarity(rarity))
}

func rowInt(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int32:
		return int(v)
	case int64:
		return int(v)
	case float64:
		return int(v)
	case float32:
		return int(v)
	case interface{ Int64() (int64, error) }:
		n, err := v.Int64()
		if err != nil {
			return 0
		}
		return int(n)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(v))
		return n
	default:
		return 0
	}
}

func rowString(value any) string {
	if s, ok := value.(string); ok {
		return s
	}
	return ""
}
