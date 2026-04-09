package provider

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	sekaiDB "haruki-cloud/database/sekai"
	"haruki-cloud/database/sekai/gamecharacter"
	"haruki-cloud/internal/pjsk/render/assets"
)

func (p *dbHonorProvider) deriveBirthdayAssetsForGroup(ctx context.Context, groupID int, groupName string) (honorBirthdayAssets, bool) {
	p.birthdayMu.RLock()
	if cached, ok := p.birthdayByGroup[groupID]; ok {
		p.birthdayMu.RUnlock()
		return cached, true
	}
	p.birthdayMu.RUnlock()

	p.birthdayMu.Lock()
	defer p.birthdayMu.Unlock()
	if cached, ok := p.birthdayByGroup[groupID]; ok {
		return cached, true
	}

	if !p.birthdayLoaded {
		if ctx == nil {
			ctx = context.Background()
		}
		rows, err := p.client.Gamecharacter.Query().
			Where(gamecharacter.ServerRegionEQ(p.region.String())).
			All(ctx)
		if err == nil {
			p.birthdayChars = rows
		}
		p.birthdayLoaded = true
	}

	for _, row := range p.birthdayChars {
		gameID := int(row.GameID)
		if gameID <= 0 {
			continue
		}
		if !honorBirthdayGroupMatchesCharacter(groupName, row) {
			continue
		}
		suffix := fmt.Sprintf("01_%02d", gameID)
		derived := honorBirthdayAssets{
			background: "honor_bg_birthday_" + suffix,
			frame:      "honor_frame_birthday_" + suffix,
		}
		p.birthdayByGroup[groupID] = derived
		slog.Info(
			"honor birthday match trace",
			"group_id", groupID,
			"group_name", groupName,
			"character_id", row.GameID,
			"first_name", row.FirstName,
			"given_name", row.GivenName,
			"first_name_english", row.FirstNameEnglish,
			"given_name_english", row.GivenNameEnglish,
			"background_assetbundle_name", derived.background,
			"frame_name", derived.frame,
		)
		return derived, true
	}
	return honorBirthdayAssets{}, false
}

func honorBirthdayGroupMatchesCharacter(groupName string, row *sekaiDB.Gamecharacter) bool {
	if row == nil {
		return false
	}
	name := strings.TrimSpace(groupName)
	if name == "" {
		return false
	}
	candidates := []string{
		strings.TrimSpace(row.FirstName),
		strings.TrimSpace(row.GivenName),
		strings.TrimSpace(row.FirstName + row.GivenName),
		strings.TrimSpace(row.FirstNameEnglish),
		strings.TrimSpace(row.GivenNameEnglish),
		strings.TrimSpace(row.FirstNameEnglish + row.GivenNameEnglish),
	}
	for _, candidate := range candidates {
		if candidate != "" && strings.Contains(name, candidate) {
			return true
		}
	}
	if nickname, ok := assets.CharacterIDToNickname[int(row.GameID)]; ok && nickname != "" {
		if strings.Contains(strings.ToLower(name), strings.ToLower(strings.TrimSpace(nickname))) {
			return true
		}
	}
	return false
}
