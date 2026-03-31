package provider

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"

	sekaiDB "haruki-cloud/database/sekai"
	"haruki-cloud/database/sekai/skill"
	"haruki-cloud/internal/pjsk/render/common"
	"haruki-cloud/internal/pjsk/render/masterdata"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

var skillPlaceholder = regexp.MustCompile(`\{\{(.*?)\}\}`)

type dbSkillProvider struct {
	client     *sekaiDB.Client
	region     renderregion.Value
	characters *dbCharacterProvider

	mu    sync.RWMutex
	cache map[int]*masterdata.Skill
}

func (p *dbSkillProvider) init() {
	if p.cache == nil {
		p.cache = make(map[int]*masterdata.Skill)
	}
}

func (p *dbSkillProvider) GetByID(id int) (*masterdata.Skill, error) {
	if id == 0 {
		return nil, nil
	}
	p.init()

	p.mu.RLock()
	if cached, ok := p.cache[id]; ok {
		p.mu.RUnlock()
		return common.CloneSkill(cached), nil
	}
	p.mu.RUnlock()

	entity, err := p.client.Skill.Query().
		Where(skill.ServerRegionEQ(p.region.String()), skill.GameIDEQ(int64(id))).
		Only(context.Background())
	if err != nil {
		return nil, fmt.Errorf("query skill %d: %w", id, err)
	}
	model, err := common.ConvertSkillEntity(entity)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	p.cache[id] = model
	p.mu.Unlock()
	return common.CloneSkill(model), nil
}

func (p *dbSkillProvider) FormatDescription(skillInfo *masterdata.Skill, cardCharacterID int) string {
	if skillInfo == nil {
		return ""
	}

	return skillPlaceholder.ReplaceAllStringFunc(skillInfo.Description, func(match string) string {
		content := match[2 : len(match)-2]
		parts := strings.Split(content, ";")
		if len(parts) != 2 {
			return match
		}

		ids := make([]int, 0, 2)
		for _, rawID := range strings.Split(parts[0], ",") {
			value, err := strconv.Atoi(strings.TrimSpace(rawID))
			if err == nil {
				ids = append(ids, value)
			}
		}
		if len(ids) == 0 {
			return match
		}

		if parts[1] == "c" {
			if p.characters != nil {
				ch, err := p.characters.GetByID(cardCharacterID)
				if err == nil && ch != nil {
					return ch.FirstName + ch.GivenName
				}
			}
			return "???"
		}

		effects := make([]*masterdata.SkillEffect, 0, len(ids))
		for _, effectID := range ids {
			for idx := range skillInfo.SkillEffects {
				if skillInfo.SkillEffects[idx].ID == effectID {
					effects = append(effects, &skillInfo.SkillEffects[idx])
					break
				}
			}
		}
		if len(effects) != len(ids) {
			return "?"
		}

		if len(effects) == 1 {
			return formatSingleEffect(effects[0], parts[1])
		}
		if len(effects) == 2 {
			return formatDualEffects(effects[0], effects[1], parts[1])
		}
		return match
	})
}

func formatSingleEffect(effect *masterdata.SkillEffect, mode string) string {
	switch mode {
	case "d":
		if len(effect.SkillEffectDetails) > 0 {
			return fmt.Sprintf("%.1f", effect.SkillEffectDetails[0].ActivateEffectDuration)
		}
		return "0.0"
	case "v":
		return formatEffectValues(getEffectValues(effect))
	case "e":
		return fmt.Sprintf("%d", effect.SkillEnhance.ActivateEffectValue)
	case "m":
		values := getEffectValues(effect)
		for idx := range values {
			values[idx] += effect.SkillEnhance.ActivateEffectValue * 5
		}
		return formatEffectValues(values)
	}
	return "?"
}

func formatDualEffects(e1, e2 *masterdata.SkillEffect, mode string) string {
	values1 := getEffectValues(e1)
	values2 := getEffectValues(e2)
	switch mode {
	case "v":
		var sums []int
		for idx := 0; idx < len(values1) && idx < len(values2); idx++ {
			sums = append(sums, values1[idx]+values2[idx])
		}
		return formatEffectValues(sums)
	case "u", "o":
		enhanced1 := getEnhancedValues(e1, values1)
		enhanced2 := getEnhancedValues(e2, values2)
		var sums []int
		for idx := 0; idx < len(enhanced1) && idx < len(enhanced2); idx++ {
			sums = append(sums, enhanced1[idx]+enhanced2[idx])
		}
		return formatEffectValues(sums)
	case "r", "s":
		return "..."
	}
	return "?"
}

func getEffectValues(effect *masterdata.SkillEffect) []int {
	if effect == nil || len(effect.SkillEffectDetails) == 0 {
		return []int{0}
	}
	values := make([]int, 0, len(effect.SkillEffectDetails))
	for _, detail := range effect.SkillEffectDetails {
		values = append(values, detail.ActivateEffectValue)
	}
	return values
}

func getEnhancedValues(effect *masterdata.SkillEffect, base []int) []int {
	if effect == nil {
		return nil
	}
	out := make([]int, 0, len(effect.SkillEffectDetails))
	for idx, detail := range effect.SkillEffectDetails {
		if detail.ActivateEffectValue2 != nil {
			out = append(out, *detail.ActivateEffectValue2)
		} else if idx < len(base) {
			out = append(out, base[idx])
		}
	}
	return out
}

func formatEffectValues(values []int) string {
	if len(values) == 0 {
		return ""
	}
	allSame := true
	first := values[0]
	for _, v := range values[1:] {
		if v != first {
			allSame = false
			break
		}
	}
	if allSame {
		return fmt.Sprintf("%d", first)
	}
	seen := make(map[int]struct{})
	unique := make([]string, 0, len(values))
	for _, v := range values {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		unique = append(unique, fmt.Sprintf("%d", v))
	}
	return strings.Join(unique, "/")
}
