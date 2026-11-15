package deck

import (
	"strconv"
	"strings"

	"haruki-cloud/internal/i18n"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/utils/usererror"
)

func (c *Controller) validateFixedCardCharacters(region renderregion.Value, recType string, option map[string]any) error {
	liveType := optionString(option, "live_type")
	if recType == "challenge" || liveType == "challenge" || liveType == "challenge_auto" {
		return nil
	}
	cardIDs, _ := option["fixed_cards"].([]int)
	if len(cardIDs) < 2 {
		return nil
	}
	_, source, err := c.resolveCardSource(region)
	if err != nil {
		return err
	}
	var characterIDs []int
	byCharacter := make(map[int][]int)
	seen := make(map[int]bool)
	for _, id := range cardIDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		card, err := c.cardByIDWithFallback(source, region, id)
		if err != nil || card == nil || card.CharacterID <= 0 {
			continue
		}
		if len(byCharacter[card.CharacterID]) == 0 {
			characterIDs = append(characterIDs, card.CharacterID)
		}
		byCharacter[card.CharacterID] = append(byCharacter[card.CharacterID], id)
	}
	var conflicts []i18n.Message
	for _, id := range characterIDs {
		if len(byCharacter[id]) > 1 {
			name := any(c.resolveCharacterName(region, id))
			if name == "" {
				name = i18n.M("deck.fixed.character_label", i18n.Data{"ID": id})
			}
			ids := make([]string, 0, len(byCharacter[id]))
			for _, cardID := range byCharacter[id] {
				ids = append(ids, strconv.Itoa(cardID))
			}
			conflicts = append(conflicts, i18n.M("deck.fixed.character_conflict", i18n.Data{
				"Name": name, "CardIDs": strings.Join(ids, ", "),
			}))
		}
	}
	if len(conflicts) > 0 {
		return usererror.Invalid(i18n.M("deck.fixed.character_conflicts", i18n.Data{"Conflicts": conflicts}))
	}
	return nil
}
