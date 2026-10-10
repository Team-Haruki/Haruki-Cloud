package drawing

import (
	"strings"

	"haruki-cloud/internal/i18n"
)

// drawingLabelsKey is the request field holding Drawing's own labels: a map
// from a Drawing label key to display text. Drawing draws the text and falls
// back to its own words for a key that is missing, so the field is optional.
// "{name}" tokens in a value are slots Drawing fills (a count, a name, a
// duration), which is why the messages below are rendered with slot tokens as
// their data.
const drawingLabelsKey = "labels"

// drawingLabel is one Drawing label key and the catalog message for it.
// prefix is literal syntax Drawing expects in front of the text (the "/" that
// joins a gacha spin count to its draw type), kept out of the catalog.
type drawingLabel struct {
	key    string
	msg    i18n.Message
	prefix string
}

func slot(name string) string { return "{" + name + "}" }

// profileDrawingLabels go on every profile and on the info panel root: the
// chips of the profile card.
var profileDrawingLabels = []drawingLabel{
	{key: "profile.rank_level", msg: i18n.M("profile.image.rank_level", i18n.Data{"Level": slot("level")})},
	{key: "profile.mysekai_level", msg: i18n.M("profile.image.mysekai_level", i18n.Data{"Level": slot("level")})},
}

var skCountdownDrawingLabels = []drawingLabel{
	{key: "sk.time_to_end", msg: i18n.M("sk.image.time_to_end", i18n.Data{"Duration": slot("duration")})},
	{key: "sk.event_ended", msg: i18n.M("sk.image.event_ended")},
}

var skPredictionDrawingLabels = []drawingLabel{
	{key: "sk.prediction_notice", msg: i18n.M("sk.forecast.notice")},
}

var skPlayerTraceDrawingLabels = []drawingLabel{
	{key: "sk.single_chapter", msg: i18n.M("sk.image.single_chapter")},
	{key: "sk.trace.player_score", msg: i18n.M("sk.image.trace.player_score", i18n.Data{"Name": slot("name")})},
	{key: "sk.trace.player_rank", msg: i18n.M("sk.image.trace.player_rank", i18n.Data{"Name": slot("name")})},
	{key: "sk.trace.compare_line", msg: i18n.M("sk.image.trace.compare_line", i18n.Data{"Rank": slot("rank")})},
	{key: "sk.trace.reference_line", msg: i18n.M("sk.image.trace.reference_line")},
	{key: "sk.trace.compare_current", msg: i18n.M("sk.image.trace.compare_current", i18n.Data{"Rank": slot("rank")})},
	{key: "sk.trace.reference_current", msg: i18n.M("sk.image.trace.reference_current")},
	{key: "sk.trace.player_title", msg: i18n.M("sk.image.trace.player_title", i18n.Data{"Names": slot("names")})},
}

var skRankTraceDrawingLabels = []drawingLabel{
	{key: "sk.single_chapter", msg: i18n.M("sk.image.single_chapter")},
	{key: "sk.trace.score_line", msg: i18n.M("sk.image.trace.score_line")},
	{key: "sk.trace.speed", msg: i18n.M("sk.image.trace.speed")},
	{key: "sk.trace.predicted_final", msg: i18n.M("sk.image.trace.predicted_final", i18n.Data{"Score": slot("score")})},
	{key: "sk.trace.rank_title", msg: i18n.M("sk.image.trace.rank_title", i18n.Data{"Rank": slot("rank")})},
}

var deckDrawingLabels = []drawingLabel{
	{key: "deck.noun.deck", msg: i18n.M("deck.image.noun.deck")},
	{key: "deck.noun.planner", msg: i18n.M("deck.image.noun.planner")},
	{key: "deck.title.mysekai_event", msg: i18n.M("deck.image.title.mysekai_event", i18n.Data{"EventID": slot("event_id"), "Noun": slot("noun")})},
	{key: "deck.title.mysekai_simulated", msg: i18n.M("deck.image.title.mysekai_simulated", i18n.Data{"Noun": slot("noun")})},
	{key: "deck.title.challenge", msg: i18n.M("deck.image.title.challenge", i18n.Data{"Noun": slot("noun")})},
	{key: "deck.title.bonus", msg: i18n.M("deck.image.title.bonus", i18n.Data{"EventID": slot("event_id"), "Noun": slot("noun")})},
	{key: "deck.title.wl_bonus", msg: i18n.M("deck.image.title.wl_bonus", i18n.Data{"EventID": slot("event_id"), "Noun": slot("noun")})},
	{key: "deck.title.event", msg: i18n.M("deck.image.title.event", i18n.Data{"EventID": slot("event_id"), "Noun": slot("noun")})},
	{key: "deck.title.wl_event", msg: i18n.M("deck.image.title.wl_event", i18n.Data{"EventID": slot("event_id"), "Noun": slot("noun")})},
	{key: "deck.title.wl_simulated", msg: i18n.M("deck.image.title.wl_simulated", i18n.Data{"Noun": slot("noun")})},
	{key: "deck.title.wl_final", msg: i18n.M("deck.image.title.wl_final", i18n.Data{"Noun": slot("noun")})},
	{key: "deck.title.unit_attr", msg: i18n.M("deck.image.title.unit_attr", i18n.Data{"Noun": slot("noun")})},
	{key: "deck.title.no_event", msg: i18n.M("deck.image.title.no_event", i18n.Data{"Noun": slot("noun")})},
	{key: "deck.algorithm.dfs", msg: i18n.M("deck.image.algorithm.dfs")},
	{key: "deck.algorithm.sa", msg: i18n.M("deck.image.algorithm.sa")},
	{key: "deck.algorithm.ga", msg: i18n.M("deck.image.algorithm.ga")},
	{key: "deck.algorithm.dfs_ga", msg: i18n.M("deck.image.algorithm.dfs_ga")},
	{key: "deck.algorithm.rl", msg: i18n.M("deck.image.algorithm.rl")},
	{key: "deck.algorithm.all", msg: i18n.M("deck.image.algorithm.all")},
	{key: "deck.skill_order.average", msg: i18n.M("deck.image.skill_order.average")},
	{key: "deck.skill_order.max", msg: i18n.M("deck.image.skill_order.max")},
	{key: "deck.skill_order.min", msg: i18n.M("deck.image.skill_order.min")},
	{key: "deck.skill_order.specific", msg: i18n.M("deck.image.skill_order.specific")},
	{key: "deck.skill_reference.average", msg: i18n.M("deck.image.skill_reference.average")},
	{key: "deck.skill_reference.max", msg: i18n.M("deck.image.skill_reference.max")},
	{key: "deck.skill_reference.min", msg: i18n.M("deck.image.skill_reference.min")},
}

var plannerDrawingLabels = []drawingLabel{
	{key: "deck.planner.title", msg: i18n.M("event.planner.title")},
	{key: "deck.planner.source", msg: i18n.M("deck.image.planner.source", i18n.Data{"Source": slot("source")})},
	{key: "deck.planner.target", msg: i18n.M("deck.image.planner.target", i18n.Data{"Point": slot("point")})},
	{key: "deck.planner.current", msg: i18n.M("deck.image.planner.current", i18n.Data{"Point": slot("point")})},
	{key: "deck.planner.remaining", msg: i18n.M("deck.image.planner.remaining", i18n.Data{"Point": slot("point")})},
}

var gachaDrawingLabels = []drawingLabel{
	{key: "gacha.type.beginner", msg: i18n.M("gacha.image.type.beginner")},
	{key: "gacha.type.normal", msg: i18n.M("gacha.image.type.normal")},
	{key: "gacha.type.ceil", msg: i18n.M("gacha.image.type.ceil")},
	{key: "gacha.type.gift", msg: i18n.M("gacha.image.type.gift")},
	{key: "gacha.behavior.normal", msg: i18n.M("gacha.image.behavior.normal")},
	{key: "gacha.behavior.over_rarity_3_once", msg: i18n.M("gacha.image.behavior.over_rarity_3_once")},
	{key: "gacha.behavior.over_rarity_4_once", msg: i18n.M("gacha.image.behavior.over_rarity_4_once")},
	{key: "gacha.behavior.once_a_day", msg: i18n.M("gacha.image.behavior.once_a_day")},
	{key: "gacha.behavior.once_a_week", msg: i18n.M("gacha.image.behavior.once_a_week")},
	{key: "gacha.behavior.unknown", msg: i18n.M("gacha.image.behavior.unknown")},
	{key: "gacha.spin.single", msg: i18n.M("gacha.image.spin.single"), prefix: "/"},
	{key: "gacha.spin.ten", msg: i18n.M("gacha.image.spin.ten"), prefix: "/"},
	{key: "gacha.colorful_pass", msg: i18n.M("gacha.image.colorful_pass", i18n.Data{"Behavior": slot("behavior")})},
	{key: "gacha.execute_limit", msg: i18n.M("gacha.image.execute_limit", i18n.Data{"Count": slot("count")})},
	{key: "gacha.rarity.rarity_1", msg: i18n.M("gacha.image.rarity.rarity_1")},
	{key: "gacha.rarity.rarity_2", msg: i18n.M("gacha.image.rarity.rarity_2")},
	{key: "gacha.rarity.rarity_3", msg: i18n.M("gacha.image.rarity.rarity_3")},
	{key: "gacha.rarity.rarity_4", msg: i18n.M("gacha.image.rarity.rarity_4")},
	{key: "gacha.rarity.rarity_birthday", msg: i18n.M("gacha.image.rarity.rarity_birthday")},
	{key: "gacha.rarity.pickup", msg: i18n.M("gacha.image.rarity.pickup")},
}

var inventoryDrawingLabels = []drawingLabel{
	{key: "inventory.resource_type.coin", msg: i18n.M("inventory.image.resource_type.coin")},
	{key: "inventory.resource_type.jewel", msg: i18n.M("inventory.image.resource_type.jewel")},
	{key: "inventory.resource_type.virtual_coin", msg: i18n.M("inventory.image.resource_type.virtual_coin")},
	{key: "inventory.resource_type.boost_item", msg: i18n.M("inventory.image.resource_type.boost_item")},
	{key: "inventory.resource_type.event_item", msg: i18n.M("inventory.image.resource_type.event_item")},
	{key: "inventory.resource_type.gacha_ticket", msg: i18n.M("inventory.image.resource_type.gacha_ticket")},
	{key: "inventory.resource_type.gacha_ceil_item", msg: i18n.M("inventory.image.resource_type.gacha_ceil_item")},
	{key: "inventory.resource_type.practice_ticket", msg: i18n.M("inventory.image.resource_type.practice_ticket")},
	{key: "inventory.resource_type.skill_practice_ticket", msg: i18n.M("inventory.image.resource_type.skill_practice_ticket")},
	{key: "inventory.resource_type.mysekai_material", msg: i18n.M("inventory.image.resource_type.mysekai_material")},
	{key: "inventory.resource_type.honor_background", msg: i18n.M("inventory.image.resource_type.honor_background")},
	{key: "inventory.resource_type.honor_word", msg: i18n.M("inventory.image.resource_type.honor_word")},
	{key: "inventory.resource_type.virtual_item", msg: i18n.M("inventory.image.resource_type.virtual_item")},
}

var costumeDrawingLabels = []drawingLabel{
	{key: "costume.part.body", msg: i18n.M("costume.part.outfit")},
	{key: "costume.part.head", msg: i18n.M("costume.part.accessory")},
	{key: "costume.part.hair", msg: i18n.M("costume.part.hair")},
}

var cardListDrawingLabels = []drawingLabel{
	{key: "card.unreleased", msg: i18n.M("render_card.unreleased")},
}

var cardBoxDrawingLabels = []drawingLabel{
	{key: "card.attr.cute", msg: i18n.M("render_card.attr.cute")},
	{key: "card.attr.cool", msg: i18n.M("render_card.attr.cool")},
	{key: "card.attr.pure", msg: i18n.M("render_card.attr.pure")},
	{key: "card.attr.happy", msg: i18n.M("render_card.attr.happy")},
	{key: "card.attr.mysterious", msg: i18n.M("render_card.attr.mysterious")},
	{key: "card.attr.unknown", msg: i18n.M("render_card.attr.unknown")},
}

var scoreDrawingLabels = []drawingLabel{
	{key: "score.target_pt", msg: i18n.M("score.image.target_pt")},
}

var vliveDrawingLabels = []drawingLabel{
	{key: "vlive.type.solo_virtual_live", msg: i18n.M("vlive.image.type.solo_virtual_live")},
	{key: "vlive.type.virtual_message", msg: i18n.M("vlive.image.type.virtual_message")},
	{key: "vlive.type.cheerful_carnival", msg: i18n.M("vlive.image.type.cheerful_carnival")},
	{key: "vlive.type.streaming", msg: i18n.M("vlive.image.type.streaming")},
	{key: "vlive.type.beginner", msg: i18n.M("vlive.image.type.beginner")},
}

// drawingEndpointLabels lists, per Drawing endpoint path, the label groups
// sent at the request root (and on every item of a list payload).
var drawingEndpointLabels = map[string][][]drawingLabel{
	"/api/pjsk/sk/line":           {skCountdownDrawingLabels, skPredictionDrawingLabels},
	"/api/pjsk/sk/query":          {skCountdownDrawingLabels},
	"/api/pjsk/sk/check-room":     {skCountdownDrawingLabels},
	"/api/pjsk/sk/csb":            {skCountdownDrawingLabels},
	"/api/pjsk/sk/speed":          {skCountdownDrawingLabels},
	"/api/pjsk/sk/winrate":        {skCountdownDrawingLabels},
	"/api/pjsk/sk/player-trace":   {skPlayerTraceDrawingLabels},
	"/api/pjsk/sk/rank-trace":     {skRankTraceDrawingLabels},
	"/api/pjsk/deck/recommend":    {deckDrawingLabels},
	"/api/pjsk/event/planner":     {deckDrawingLabels, plannerDrawingLabels},
	"/api/pjsk/gacha/detail":      {gachaDrawingLabels},
	"/api/pjsk/inventory/list":    {inventoryDrawingLabels},
	"/api/pjsk/costume/list":      {costumeDrawingLabels},
	"/api/pjsk/card/list":         {cardListDrawingLabels},
	"/api/pjsk/card/box":          {cardBoxDrawingLabels},
	"/api/pjsk/score/control":     {scoreDrawingLabels},
	"/api/pjsk/score/custom-room": {scoreDrawingLabels},
	"/api/pjsk/vlive/list":        {vliveDrawingLabels},
	"/api/pjsk/vlive/detail":      {vliveDrawingLabels},
	InfoPanelEndpoint:             {profileDrawingLabels},
}

// applyDrawingEndpointLabels adds the endpoint's label groups to body.
func applyDrawingEndpointLabels(endpointPath string, body map[string]any, locale i18n.Locale) {
	for _, group := range drawingEndpointLabels[endpointPath] {
		mergeDrawingLabels(body, group, locale)
	}
}

// mergeDrawingLabels adds group to body's labels; a key the builder already
// set keeps its value.
func mergeDrawingLabels(body map[string]any, group []drawingLabel, locale i18n.Locale) {
	if body == nil || len(group) == 0 {
		return
	}
	labels := mapAt(body, drawingLabelsKey)
	if labels == nil {
		labels = make(map[string]any, len(group))
	}
	for _, label := range group {
		if strings.TrimSpace(scalarString(labels[label.key])) != "" {
			continue
		}
		labels[label.key] = label.prefix + label.msg.In(locale)
	}
	body[drawingLabelsKey] = labels
}
