package mysekai

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
)

const userBirthdayPartiesKey = "userBirthdayParties"

// extractBirthdayParties returns the progress of every birthday party running
// at nowMillis (startAt <= now < closedAt). The level is the user's
// obtainedMysekaiMaterialCount, which the client compares with
// birthdayPartyDeliveryTotalRewards.requirement and shows as
// "<count>/<requirement>" (it keeps counting past the last requirement). The
// target is that party's highest requirement. Each party also carries when
// 露滴 stop dropping (birthdayStartAt) and when 浇水 ends (closedAt), matching
// the 露滴掉落 / 浇水开放 spans of the birthday command.
//
// Nothing is returned when the snapshot carries no userBirthdayParties (no
// data to show) or the region's master lacks either table.
func (c *Controller) extractBirthdayParties(region renderregion.Value, merged map[string]any, nowMillis int64) []drawing.MysekaiBirthdayPartyProgress {
	userParties, ok := userBirthdayParties(merged)
	if !ok {
		return nil
	}
	parties := c.masterdata.loadList("birthdayParties.json")
	if len(parties) == 0 {
		return nil
	}

	running := make([]map[string]any, 0, 2)
	for _, party := range parties {
		startAt := int64NumberFrom(party, "startAt", "start_at")
		closedAt := int64NumberFrom(party, "closedAt", "closed_at")
		if intNumber(party["id"], 0) <= 0 || startAt <= 0 || closedAt <= 0 {
			continue
		}
		if startAt <= nowMillis && nowMillis < closedAt {
			running = append(running, party)
		}
	}
	if len(running) == 0 {
		return nil
	}
	sort.SliceStable(running, func(i, j int) bool {
		si, sj := int64NumberFrom(running[i], "startAt", "start_at"), int64NumberFrom(running[j], "startAt", "start_at")
		if si != sj {
			return si < sj
		}
		return intNumber(running[i]["id"], 0) < intNumber(running[j]["id"], 0)
	})

	maxLevels := birthdayPartyMaxLevels(c.masterdata.loadList("birthdayPartyDeliveryTotalRewards.json"))
	result := make([]drawing.MysekaiBirthdayPartyProgress, 0, len(running))
	for _, party := range running {
		partyID := intNumber(party["id"], 0)
		maxLevel := maxLevels[partyID]
		if maxLevel <= 0 {
			continue
		}
		unitID := intNumberFrom(party, 0, "gameCharacterUnitId", "game_character_unit_id")
		if unitID <= 0 {
			continue
		}
		level := 0
		if record := userParties[partyID]; record != nil {
			level = max(intNumberFrom(record, 0, "obtainedMysekaiMaterialCount", "obtained_mysekai_material_count"), 0)
		}
		result = append(result, drawing.MysekaiBirthdayPartyProgress{
			BirthdayPartyID:   partyID,
			CharacterUnitID:   unitID,
			CharacterName:     c.birthdayPartyCharacterName(unitID),
			CharacterIconPath: c.staticPath(fmt.Sprintf("chara_icon/%s.png", charaIconName(unitID))),
			CharacterColor:    stringValueFrom(c.masterdata.loadMapByID("gameCharacterUnits.json")[unitID], "colorCode", "color_code"),
			Level:             level,
			MaxLevel:          maxLevel,
			DropEndAt:         int64NumberFrom(party, "birthdayStartAt", "birthday_start_at"),
			WateringEndAt:     int64NumberFrom(party, "closedAt", "closed_at"),
		})
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// userBirthdayParties indexes the snapshot's userBirthdayParties by party ID.
// ok is false when the snapshot does not carry the list at all, so a missing
// record can be told apart from a party the user has not delivered to yet.
func userBirthdayParties(merged map[string]any) (map[int]map[string]any, bool) {
	if merged == nil {
		return nil, false
	}
	raw, ok := merged[userBirthdayPartiesKey]
	if !ok {
		if updated, isMap := merged["updatedResources"].(map[string]any); isMap {
			raw, ok = updated[userBirthdayPartiesKey]
		}
	}
	items, isList := raw.([]any)
	if !ok || !isList {
		return nil, false
	}
	result := make(map[int]map[string]any, len(items))
	for _, item := range items {
		entry, isMap := item.(map[string]any)
		if !isMap {
			continue
		}
		if partyID := intNumberFrom(entry, 0, "birthdayPartyId", "birthday_party_id"); partyID > 0 {
			result[partyID] = entry
		}
	}
	return result, true
}

func birthdayPartyMaxLevels(rewards []map[string]any) map[int]int {
	result := make(map[int]int)
	for _, reward := range rewards {
		partyID := intNumberFrom(reward, 0, "birthdayPartyId", "birthday_party_id")
		requirement := intNumber(reward["requirement"], 0)
		if partyID > 0 && requirement > result[partyID] {
			result[partyID] = requirement
		}
	}
	return result
}

func (c *Controller) birthdayPartyCharacterName(unitID int) string {
	characterID := c.gameCharacterIDByUnitID(unitID)
	if characterID <= 0 {
		return ""
	}
	character := c.masterdata.loadMapByID("gameCharacters.json")[characterID]
	first := strings.TrimSpace(stringValueFrom(character, "firstName", "first_name"))
	given := strings.TrimSpace(stringValueFrom(character, "givenName", "given_name"))
	if first == "" || given == "" {
		return first + given
	}
	if isLatinName(first) && isLatinName(given) {
		return first + " " + given
	}
	return first + given
}

func isLatinName(value string) bool {
	for _, r := range value {
		if r > unicode.MaxLatin1 {
			return false
		}
	}
	return true
}

func int64NumberFrom(item map[string]any, keys ...string) int64 {
	for _, key := range keys {
		if value, ok := item[key]; ok {
			if n := int64Number(value, 0); n != 0 {
				return n
			}
		}
	}
	return 0
}
