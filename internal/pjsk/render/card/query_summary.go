package card

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"haruki-cloud/internal/i18n"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/common"
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
	regionLabel, source := c.summaryRegionSource(region)
	if regionLabel != "" {
		parts = append(parts, regionLabel)
	}
	parts = append(parts, cardSummaryModeLabel(mode))
	parts = append(parts, c.describeQueryParts(mode, rawQuery, strictFilterOnly, cardIDs, source)...)
	if mode == "box" {
		parts = appendBoxSummaryOptions(parts, showID, showBox, useAfterTraining)
	}
	return strings.Join(filterNonEmptyStrings(parts), " / ")
}

func (c *Controller) summaryRegionSource(region string) (string, DataSource) {
	if c != nil {
		resolved, source, _, err := c.resolveBuilder(region)
		if err == nil {
			return i18n.RegionLabel(resolved.String()).String(), source
		}
	}
	code := strings.TrimSpace(renderregion.Normalize(region).String())
	if code != "" && !strings.EqualFold(code, "unknown") {
		return i18n.RegionLabel(code).String(), nil
	}
	if strings.TrimSpace(region) == "" {
		return "", nil
	}
	return i18n.RegionLabel(region).String(), nil
}

func appendBoxSummaryOptions(parts []string, showID, showBox bool, useAfterTraining *bool) []string {
	if showID {
		parts = append(parts, i18n.T("render_card.summary.show_id"))
	}
	if showBox {
		parts = append(parts, i18n.T("render_card.summary.show_box"))
	}
	if useAfterTraining != nil && !*useAfterTraining {
		parts = append(parts, i18n.T("render_card.summary.before_training"))
	}
	return parts
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
			return []string{i18n.T("render_card.summary.all_released")}
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
		return []string{i18n.T("render_card.summary.card_id", i18n.Data{"ID": cleaned[0]})}
	}
	if len(cleaned) <= 5 {
		labels := make([]string, 0, len(cleaned))
		for _, cardID := range cleaned {
			labels = append(labels, strconv.Itoa(cardID))
		}
		return []string{i18n.T("render_card.summary.card_ids", i18n.Data{"IDs": strings.Join(labels, ", ")})}
	}
	return []string{i18n.T("render_card.summary.card_count", i18n.Data{"Count": len(cleaned)})}
}

func describeCardQueryInfo(info *PjskCardQueryInfo, source DataSource, nicknames map[string]int) []string {
	if info == nil {
		return nil
	}

	switch info.Type {
	case QueryTypeID:
		return describeCardIDQuery(info)
	case QueryTypeLatest:
		return describeCardLatestQuery(info)
	case QueryTypeSeq:
		return describeCardSequenceQuery(info, source, nicknames)
	case QueryTypeFilter:
		return describeCardFilterQuery(info, source, nicknames)
	}
	return describeOriginalCardQuery(info)
}

func describeCardIDQuery(info *PjskCardQueryInfo) []string {
	if info.Value > 0 {
		return []string{i18n.T("render_card.summary.card_id", i18n.Data{"ID": info.Value})}
	}
	return describeOriginalCardQuery(info)
}

func describeCardLatestQuery(info *PjskCardQueryInfo) []string {
	if info.Sequence < 0 {
		return []string{i18n.T("render_card.summary.latest_global", i18n.Data{"N": -info.Sequence})}
	}
	return describeOriginalCardQuery(info)
}

func describeCardSequenceQuery(info *PjskCardQueryInfo, source DataSource, nicknames map[string]int) []string {
	if info.CharacterID <= 0 || info.Sequence == 0 {
		return describeOriginalCardQuery(info)
	}
	name := summaryCharacterLabel(source, nicknames, info.CharacterID)
	if info.Sequence < 0 {
		return []string{i18n.T("render_card.summary.latest_character", i18n.Data{"Character": name, "N": -info.Sequence})}
	}
	return []string{i18n.T("render_card.summary.character_seq", i18n.Data{"Character": name, "N": info.Sequence})}
}

func describeCardFilterQuery(info *PjskCardQueryInfo, source DataSource, nicknames map[string]int) []string {
	parts := make([]string, 0, 10)
	if info.EventID > 0 {
		parts = append(parts, fmt.Sprintf("event%d", info.EventID))
	}
	if info.BanCharID > 0 && info.BanSeq > 0 {
		parts = append(parts, i18n.T("render_card.summary.ban_event", i18n.Data{"Character": summaryCharacterLabel(source, nicknames, info.BanCharID), "N": info.BanSeq}))
	}
	if info.CharacterID > 0 {
		parts = append(parts, summaryCharacterLabel(source, nicknames, info.CharacterID))
	}
	parts = appendNonEmptyCardSummaryLabels(parts,
		summaryAttributeLabel(info.Attr),
		summaryDetailedSkillLabel(info.SkillIDs),
		summarySkillTypeLabel(info.SkillType),
		summaryUnitFilterLabel(info),
		summaryRarityLabel(info.Rarity),
		summarySupplyLabel(info.SupplyType),
	)
	if info.Year > 0 {
		parts = append(parts, i18n.T("render_card.summary.year", i18n.Data{"Year": info.Year}))
	}
	return parts
}

func appendNonEmptyCardSummaryLabels(parts []string, labels ...string) []string {
	for _, label := range labels {
		if label != "" {
			parts = append(parts, label)
		}
	}
	return parts
}

func describeOriginalCardQuery(info *PjskCardQueryInfo) []string {
	if original := strings.TrimSpace(info.Original); original != "" {
		return []string{original}
	}
	return nil
}

func cardSummaryModeLabel(mode string) string {
	switch mode {
	case "list":
		return i18n.T("render_card.summary.mode.list")
	case "box":
		return i18n.T("render_card.summary.mode.box")
	default:
		return i18n.T("render_card.summary.mode.detail")
	}
}

func summaryCharacterLabel(source DataSource, nicknames map[string]int, characterID int) string {
	if characterID <= 0 {
		return ""
	}
	if name := summaryCharacterSourceName(source, characterID); name != "" {
		return name
	}
	if nickname := bestSummaryCharacterNickname(nicknames, characterID); nickname != "" {
		return nickname
	}
	return common.CharacterFallbackName(characterID)
}

func summaryCharacterSourceName(source DataSource, characterID int) string {
	if source == nil {
		return ""
	}
	character, err := source.GetCharacterByID(characterID)
	if err != nil || character == nil {
		return ""
	}
	return strings.TrimSpace(character.FirstName + character.GivenName)
}

func bestSummaryCharacterNickname(nicknames map[string]int, characterID int) string {
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
	return best
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
		return i18n.T("render_card.summary.attr.cute")
	case "cool":
		return i18n.T("render_card.summary.attr.cool")
	case "pure":
		return i18n.T("render_card.summary.attr.pure")
	case "happy":
		return i18n.T("render_card.summary.attr.happy")
	case "mysterious":
		return i18n.T("render_card.summary.attr.mysterious")
	default:
		return ""
	}
}

func summarySkillTypeLabel(skillType string) string {
	switch strings.TrimSpace(skillType) {
	case "life_recovery":
		return i18n.T("render_card.summary.skill_type.life_recovery")
	case "score_up":
		return i18n.T("render_card.summary.skill_type.score_up")
	case "judgment_up":
		return i18n.T("render_card.summary.skill_type.judgment_up")
	default:
		return ""
	}
}

func summaryDetailedSkillLabel(skillIDs []int) string {
	if len(skillIDs) == 0 {
		return ""
	}
	if slices.Equal(skillIDs, []int{4}) {
		return i18n.T("render_card.summary.skill.big_score")
	}
	if slices.Equal(skillIDs, []int{11}) {
		return i18n.T("render_card.summary.skill.perfect_score")
	}
	if slices.Equal(skillIDs, []int{12}) {
		return i18n.T("render_card.summary.skill.life_score")
	}
	if slices.Equal(skillIDs, []int{13}) {
		return i18n.T("render_card.summary.skill.judgment_score")
	}
	if slices.Equal(skillIDs, []int{15, 16, 17, 18, 19}) {
		return i18n.T("render_card.summary.skill.unit_score")
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
	return i18n.T("render_card.summary.skill.ids", i18n.Data{"IDs": strings.Join(labels, ",")})
}

func summaryUnitFilterLabel(info *PjskCardQueryInfo) string {
	if info == nil {
		return ""
	}
	if info.MainUnit != "" {
		if info.MainUnit == "piapro" {
			if info.SupportUnit == "none" {
				return i18n.T("render_card.summary.unit.original_vs")
			}
			if label := summaryAttachedVSLabel(info.SupportUnit); label != "" {
				return label
			}
		} else if info.SupportUnit == "none" {
			if label := summaryUnitLabel(info.MainUnit); label != "" {
				return i18n.T("render_card.summary.unit.pure", i18n.Data{"Unit": label})
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
		return i18n.T("render_card.summary.rarity.rarity_4")
	case "rarity_3":
		return i18n.T("render_card.summary.rarity.rarity_3")
	case "rarity_2":
		return i18n.T("render_card.summary.rarity.rarity_2")
	case "rarity_1":
		return i18n.T("render_card.summary.rarity.rarity_1")
	case "rarity_birthday":
		return i18n.T("render_card.summary.rarity.birthday")
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
		return i18n.T("render_card.summary.supply.wl")
	case SupplyCollab:
		return i18n.T("render_card.summary.supply.collab")
	case SupplyLimited:
		return i18n.T("render_card.summary.supply.limited")
	case SupplyNormal:
		return i18n.T("render_card.summary.supply.normal")
	case SupplyBirthday:
		return i18n.T("render_card.summary.supply.birthday")
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
