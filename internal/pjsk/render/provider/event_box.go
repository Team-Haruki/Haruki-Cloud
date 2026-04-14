package provider

import (
	"context"
	"strings"

	"haruki-cloud/internal/pjsk/render/masterdata"
)

func eventUsesSingleBonusUnit(
	ctx context.Context,
	bonuses []*masterdata.EventDeckBonus,
	resolveUnit func(context.Context, int) (string, bool),
) bool {
	units := make(map[string]struct{})
	for _, bonus := range bonuses {
		if bonus == nil || bonus.GameCharacterUnitID == 0 {
			continue
		}
		unit, ok := resolveUnit(ctx, bonus.GameCharacterUnitID)
		if !ok {
			continue
		}
		units[unit] = struct{}{}
		if len(units) > 1 {
			return false
		}
	}
	return len(units) == 1
}

func normalizeEventBonusUnit(unit string) (string, bool) {
	unit = strings.ToLower(strings.TrimSpace(unit))
	if unit == "" {
		return "", false
	}
	return unit, true
}
