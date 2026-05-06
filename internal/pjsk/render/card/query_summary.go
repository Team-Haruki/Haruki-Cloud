package card

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	renderregion "haruki-cloud/internal/pjsk/region"
)

func (c *Controller) SummaryForDetail(query Query) string {
	return c.formatQuerySummary(query.Region, "detail", strings.TrimSpace(query.Query), query.StrictFilterOnly, false, false, query.UseAfterTraining, nil)
}

func (c *Controller) SummaryForList(query ListRequest) string {
	return c.formatQuerySummary(query.Region, "list", strings.TrimSpace(query.Query), query.StrictFilterOnly, false, false, nil, query.CardIDs)
}

func (c *Controller) SummaryForBox(query Query) string {
	return c.formatQuerySummary(query.Region, "box", strings.TrimSpace(query.Query), query.StrictFilterOnly, query.ShowID, query.ShowBox, query.UseAfterTraining, nil)
}

func (c *Controller) ShouldShowSummaryForDetail(Query) bool {
	return false
}

func (c *Controller) ShouldShowSummaryForList(query ListRequest) bool {
	if len(query.CardIDs) > 1 {
		return true
	}
	if len(query.CardIDs) == 1 {
		return false
	}
	return c.queryUsesMultiCardSelection("list", strings.TrimSpace(query.Query), query.StrictFilterOnly)
}

func (c *Controller) ShouldShowSummaryForBox(query Query) bool {
	rawQuery := strings.TrimSpace(query.Query)
	if rawQuery == "" {
		return true
	}
	return c.queryUsesMultiCardSelection("box", rawQuery, query.StrictFilterOnly)
}

func (c *Controller) formatQuerySummary(region string, mode string, rawQuery string, strictFilterOnly bool, showID bool, showBox bool, useAfterTraining *bool, cardIDs []int) string {
	parts := make([]string, 0, 8)

	var source DataSource
	if c != nil {
		if resolved, resolvedSource, _, err := c.resolveBuilder(region); err == nil {
			source = resolvedSource
			if label := strings.ToUpper(strings.TrimSpace(resolved.String())); label != "" {
				parts = append(parts, label)
			}
		}
	}
	if len(parts) == 0 {
		if label := strings.ToUpper(strings.TrimSpace(renderregion.Normalize(region).String())); label != "" && label != "UNKNOWN" {
			parts = append(parts, label)
		} else if label := strings.ToUpper(strings.TrimSpace(region)); label != "" {
			parts = append(parts, label)
		}
	}

	parts = append(parts, cardSummaryModeLabel(mode))
	parts = append(parts, c.describeQueryParts(mode, rawQuery, strictFilterOnly, cardIDs, source)...)

	if mode == "box" {
		if showID {
			parts = append(parts, "显示ID")
		}
		if showBox {
			parts = append(parts, "显示持有")
		}
		if useAfterTraining != nil && !*useAfterTraining {
			parts = append(parts, "花前")
		}
	}

	return strings.Join(filterNonEmptyStrings(parts), " / ")
}

func (c *Controller) queryUsesMultiCardSelection(mode string, rawQuery string, strictFilterOnly bool) bool {
	rawQuery = strings.TrimSpace(rawQuery)
	if rawQuery == "" {
		return mode == "box"
	}

	parserNicknames := cloneNicknames(defaultNicknames)
	if c != nil && len(c.nicknames) > 0 {
		parserNicknames = cloneNicknames(c.nicknames)
	}
	parser := NewParser(parserNicknames)

	var (
		info *PjskCardQueryInfo
		err  error
	)
	switch {
	case strictFilterOnly:
		info, err = parser.ParseStrictFilter(rawQuery)
	case mode == "detail":
		info, err = parser.Parse(rawQuery)
	default:
		info, err = parser.ParsePreferFilter(rawQuery)
	}
	if err != nil || info == nil {
		return false
	}
	return info.Type == QueryTypeFilter
}

func (c *Controller) describeQueryParts(mode string, rawQuery string, strictFilterOnly bool, cardIDs []int, source DataSource) []string {
	rawQuery = strings.TrimSpace(rawQuery)
	if rawQuery == "" {
		if len(cardIDs) > 0 {
			return describeExplicitCardIDs(cardIDs)
		}
		if mode == "box" {
			return []string{"全部已上线卡牌"}
		}
		return nil
	}

	parserNicknames := cloneNicknames(defaultNicknames)
	if c != nil && len(c.nicknames) > 0 {
		parserNicknames = cloneNicknames(c.nicknames)
	}
	parser := NewParser(parserNicknames)

	var (
		info *PjskCardQueryInfo
		err  error
	)
	switch {
	case strictFilterOnly:
		info, err = parser.ParseStrictFilter(rawQuery)
	case mode == "detail":
		info, err = parser.Parse(rawQuery)
	default:
		info, err = parser.ParsePreferFilter(rawQuery)
	}
	if err != nil || info == nil {
		return []string{rawQuery}
	}

	parts := describeCardQueryInfo(info, source, parserNicknames)
	if len(parts) == 0 {
		return []string{rawQuery}
	}
	return parts
}

func describeExplicitCardIDs(cardIDs []int) []string {
	if len(cardIDs) == 0 {
		return nil
	}
	cleaned := make([]int, 0, len(cardIDs))
	for _, cardID := range cardIDs {
		if cardID > 0 {
			cleaned = append(cleaned, cardID)
		}
	}
	if len(cleaned) == 0 {
		return nil
	}
	if len(cleaned) == 1 {
		return []string{fmt.Sprintf("卡牌ID%d", cleaned[0])}
	}
	if len(cleaned) <= 5 {
		labels := make([]string, 0, len(cleaned))
		for _, cardID := range cleaned {
			labels = append(labels, strconv.Itoa(cardID))
		}
		return []string{fmt.Sprintf("卡牌ID %s", strings.Join(labels, ", "))}
	}
	return []string{fmt.Sprintf("%d张指定卡牌", len(cleaned))}
}

func describeCardQueryInfo(info *PjskCardQueryInfo, source DataSource, nicknames map[string]int) []string {
	if info == nil {
		return nil
	}

	switch info.Type {
	case QueryTypeID:
		if info.Value > 0 {
			return []string{fmt.Sprintf("卡牌ID%d", info.Value)}
		}
	case QueryTypeLatest:
		if info.Sequence < 0 {
			return []string{fmt.Sprintf("全局最新第%d张", -info.Sequence)}
		}
	case QueryTypeSeq:
		if info.CharacterID > 0 && info.Sequence != 0 {
			name := summaryCharacterLabel(source, nicknames, info.CharacterID)
			if info.Sequence < 0 {
				return []string{fmt.Sprintf("%s最新第%d张", name, -info.Sequence)}
			}
			return []string{fmt.Sprintf("%s第%d张", name, info.Sequence)}
		}
	case QueryTypeFilter:
		parts := make([]string, 0, 10)
		if info.EventID > 0 {
			parts = append(parts, fmt.Sprintf("event%d", info.EventID))
		}
		if info.BanCharID > 0 && info.BanSeq > 0 {
			parts = append(parts, fmt.Sprintf("%s%d箱活", summaryCharacterLabel(source, nicknames, info.BanCharID), info.BanSeq))
		}
		if info.CharacterID > 0 {
			parts = append(parts, summaryCharacterLabel(source, nicknames, info.CharacterID))
		}
		if label := summaryAttributeLabel(info.Attr); label != "" {
			parts = append(parts, label)
		}
		if label := summaryDetailedSkillLabel(info.SkillIDs); label != "" {
			parts = append(parts, label)
		}
		if label := summarySkillTypeLabel(info.SkillType); label != "" {
			parts = append(parts, label)
		}
		if label := summaryUnitFilterLabel(info); label != "" {
			parts = append(parts, label)
		}
		if label := summaryRarityLabel(info.Rarity); label != "" {
			parts = append(parts, label)
		}
		if label := summarySupplyLabel(info.SupplyType); label != "" {
			parts = append(parts, label)
		}
		if info.Year > 0 {
			parts = append(parts, fmt.Sprintf("%d年", info.Year))
		}
		return parts
	}

	if original := strings.TrimSpace(info.Original); original != "" {
		return []string{original}
	}
	return nil
}

func cardSummaryModeLabel(mode string) string {
	switch mode {
	case "detail":
		return "查卡"
	case "list":
		return "卡牌列表"
	case "box":
		return "卡牌一览"
	default:
		return "查卡"
	}
}

func summaryCharacterLabel(source DataSource, nicknames map[string]int, characterID int) string {
	if characterID <= 0 {
		return ""
	}
	if source != nil {
		if character, err := source.GetCharacterByID(characterID); err == nil && character != nil {
			if name := strings.TrimSpace(character.FirstName + character.GivenName); name != "" {
				return name
			}
		}
	}

	best := ""
	for nickname, id := range nicknames {
		if id != characterID {
			continue
		}
		nickname = strings.TrimSpace(nickname)
		if nickname == "" {
			continue
		}
		if best == "" || betterSummaryNickname(nickname, best) {
			best = nickname
		}
	}
	if best != "" {
		return best
	}
	return fmt.Sprintf("角色%d", characterID)
}

func betterSummaryNickname(candidate string, current string) bool {
	candidateASCII := isASCIIOnly(candidate)
	currentASCII := isASCIIOnly(current)
	if candidateASCII != currentASCII {
		return candidateASCII
	}
	candidateLen := len([]rune(candidate))
	currentLen := len([]rune(current))
	if candidateLen != currentLen {
		return candidateLen < currentLen
	}
	return candidate < current
}

func summaryAttributeLabel(attr string) string {
	switch strings.TrimSpace(attr) {
	case "cute":
		return "粉"
	case "cool":
		return "蓝"
	case "pure":
		return "绿"
	case "happy":
		return "橙"
	case "mysterious":
		return "紫"
	default:
		return ""
	}
}

func summarySkillTypeLabel(skillType string) string {
	switch strings.TrimSpace(skillType) {
	case "life_recovery":
		return "奶卡"
	case "score_up":
		return "分卡"
	case "judgment_up":
		return "判卡"
	default:
		return ""
	}
}

func summaryDetailedSkillLabel(skillIDs []int) string {
	if len(skillIDs) == 0 {
		return ""
	}
	if slices.Equal(skillIDs, []int{4}) {
		return "大分"
	}
	if slices.Equal(skillIDs, []int{11}) {
		return "P分"
	}
	if slices.Equal(skillIDs, []int{12}) {
		return "血分"
	}
	if slices.Equal(skillIDs, []int{13}) {
		return "判分"
	}
	if slices.Equal(skillIDs, []int{15, 16, 17, 18, 19}) {
		return "团分"
	}
	labels := make([]string, 0, len(skillIDs))
	for _, skillID := range skillIDs {
		if skillID <= 0 {
			continue
		}
		labels = append(labels, strconv.Itoa(skillID))
	}
	if len(labels) == 0 {
		return ""
	}
	return "技能" + strings.Join(labels, ",")
}

func summaryUnitFilterLabel(info *PjskCardQueryInfo) string {
	if info == nil {
		return ""
	}
	if info.MainUnit != "" {
		if info.MainUnit == "piapro" {
			if info.SupportUnit == "none" {
				return "原V"
			}
			if label := summaryAttachedVSLabel(info.SupportUnit); label != "" {
				return label
			}
		} else if info.SupportUnit == "none" {
			if label := summaryUnitLabel(info.MainUnit); label != "" {
				return "纯" + label
			}
		}
	}
	return summaryUnitLabel(info.Unit)
}

func summaryAttachedVSLabel(unit string) string {
	switch strings.TrimSpace(unit) {
	case "light_sound":
		return "LNV"
	case "idol":
		return "MMJV"
	case "street":
		return "VBSV"
	case "theme_park":
		return "WSV"
	case "school_refusal":
		return "25HV"
	default:
		return ""
	}
}

func summaryUnitLabel(unit string) string {
	switch strings.TrimSpace(unit) {
	case "light_sound":
		return "L/N"
	case "idol":
		return "MMJ"
	case "street":
		return "VBS"
	case "theme_park":
		return "WS"
	case "school_refusal":
		return "25H"
	case "piapro":
		return "VS"
	default:
		return ""
	}
}

func summaryRarityLabel(rarity string) string {
	switch strings.TrimSpace(rarity) {
	case "rarity_4":
		return "四星"
	case "rarity_3":
		return "三星"
	case "rarity_2":
		return "二星"
	case "rarity_1":
		return "一星"
	case "rarity_birthday":
		return "生日"
	default:
		return ""
	}
}

func summarySupplyLabel(supply string) string {
	switch strings.TrimSpace(supply) {
	case SupplyFes:
		return "fes"
	case SupplyCFes:
		return "cfes"
	case SupplyBFes:
		return "bfes"
	case SupplyWL:
		return "wl限定"
	case SupplyCollab:
		return "联动限定"
	case SupplyLimited:
		return "限定"
	case SupplyNormal:
		return "非限"
	case SupplyBirthday:
		return "生日"
	default:
		return ""
	}
}

func filterNonEmptyStrings(items []string) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		result = append(result, item)
	}
	return result
}
