package provider

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	sekaiDB "haruki-cloud/database/sekai"
	"haruki-cloud/database/sekai/skill"
	"haruki-cloud/internal/observability/commandtrace"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/common"
	"haruki-cloud/internal/pjsk/render/masterdata"

	"golang.org/x/sync/singleflight"
)

var skillPlaceholder = regexp.MustCompile(`{{(.*?)}}`)

type dbSkillProvider struct {
	client     *sekaiDB.Client
	region     renderregion.Value
	characters *dbCharacterProvider
	once       sync.Once

	mu            sync.RWMutex
	cache         map[int]*masterdata.Skill
	cacheLoadedAt map[int]time.Time

	allLoaded   bool
	allLoadedAt time.Time
	allLoads    singleflight.Group
}

func (p *dbSkillProvider) init() {
	p.once.Do(func() {
		p.cache = make(map[int]*masterdata.Skill)
		p.cacheLoadedAt = make(map[int]time.Time)
	})
}

func (p *dbSkillProvider) GetByID(ctx context.Context, id int) (*masterdata.Skill, error) {
	if id == 0 {
		return nil, nil
	}
	p.init()

	p.mu.RLock()
	cached, ok := p.cache[id]
	cachedAt := p.cacheLoadedAt[id]
	allLoaded := dbBulkIndexFresh(p.allLoaded, p.allLoadedAt)
	if ok && dbBulkIndexFresh(true, cachedAt) {
		p.mu.RUnlock()
		return common.CloneSkill(cached), nil
	}
	p.mu.RUnlock()
	if allLoaded {
		return nil, fmt.Errorf("skill %d not found", id)
	}

	entity, err := p.client.Skill.Query().
		Where(skill.ServerRegionEQ(p.region.String()), skill.GameIDEQ(int64(id))).
		Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("query skill %d: %w", id, err)
	}
	model, err := common.ConvertSkillEntity(entity)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	p.cache[id] = model
	p.cacheLoadedAt[id] = time.Now()
	p.mu.Unlock()
	return common.CloneSkill(model), nil
}

func (p *dbSkillProvider) matchingTypeIDs(ctx context.Context, skillType string) (map[int]struct{}, error) {
	if p == nil || p.client == nil {
		return nil, fmt.Errorf("skill provider is not configured")
	}
	if err := p.ensureAllLoaded(ctx); err != nil {
		return nil, err
	}

	result := make(map[int]struct{})
	p.mu.RLock()
	for id, skillInfo := range p.cache {
		if skillInfo != nil && cardSkillTypesMatch(skillType, skillInfo.DescriptionSpriteName) {
			result[id] = struct{}{}
		}
	}
	p.mu.RUnlock()
	return result, nil
}

func (p *dbSkillProvider) ensureAllLoaded(ctx context.Context) error {
	p.init()
	p.mu.RLock()
	loaded := dbBulkIndexFresh(p.allLoaded, p.allLoadedAt)
	p.mu.RUnlock()
	if loaded {
		return nil
	}

	callerToken := new(dbBulkIndexFlightToken)
	result := p.allLoads.DoChan("all", func() (any, error) {
		completed := runDBBulkIndexFlight(callerToken, func(loadCtx context.Context) error {
			finishIndex := commandtrace.MeasureOperation(loadCtx, "skills.index")
			defer finishIndex()
			p.mu.RLock()
			alreadyLoaded := dbBulkIndexFresh(p.allLoaded, p.allLoadedAt)
			p.mu.RUnlock()
			if alreadyLoaded {
				return nil
			}
			entities, err := p.client.Skill.Query().
				Where(skill.ServerRegionEQ(p.region.String())).
				All(loadCtx)
			if err != nil {
				return fmt.Errorf("query skills for region %s: %w", p.region, err)
			}

			loadedSkills := make(map[int]*masterdata.Skill, len(entities))
			for _, entity := range entities {
				model, err := common.ConvertSkillEntity(entity)
				if err != nil {
					return fmt.Errorf("decode skill %d for region %s: %w", entity.GameID, p.region, err)
				}
				loadedSkills[model.ID] = model
			}

			p.mu.Lock()
			p.cache = loadedSkills
			loadedAt := time.Now()
			p.cacheLoadedAt = make(map[int]time.Time, len(loadedSkills))
			for id := range loadedSkills {
				p.cacheLoadedAt[id] = loadedAt
			}
			p.allLoaded = true
			p.allLoadedAt = loadedAt
			p.mu.Unlock()
			return nil
		})
		return completed, nil
	})

	return waitDBBulkIndexFlight(ctx, result, callerToken, "skills.index_wait", "skills.index_shared")
}

func (p *dbSkillProvider) FormatDescription(ctx context.Context, skillInfo *masterdata.Skill, cardCharacterID int) string {
	var characterLookup skillCharacterLookup
	if p.characters != nil {
		characterLookup = p.characters.GetByID
	}
	return formatSkillDescription(ctx, skillInfo, cardCharacterID, characterLookup)
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
