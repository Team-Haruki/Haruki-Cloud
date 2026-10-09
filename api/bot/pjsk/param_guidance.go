package pjsk

import (
	"strings"

	"haruki-cloud/internal/i18n"
)

// routeGuidance is the one-line reason shown, per route, when a command's
// arguments could not be understood at all (usererror.Unrecognized). It is
// looked up by route path and error code, never by message text.
var routeGuidance = map[string]i18n.Message{
	"arrest":                      i18n.M("guidance.arrest"),
	"card/box":                    i18n.M("guidance.card_box"),
	"card/detail":                 i18n.M("guidance.card_detail"),
	"card/image":                  i18n.M("guidance.card_image"),
	"card/list":                   i18n.M("guidance.card_list"),
	"costume/detail":              i18n.M("guidance.costume_detail"),
	"deck/event":                  i18n.M("guidance.deck_event"),
	"deck/challenge":              i18n.M("guidance.deck_challenge"),
	"deck/no-event":               i18n.M("guidance.deck_no_event"),
	"deck/bonus":                  i18n.M("guidance.deck_bonus"),
	"deck/mysekai":                i18n.M("guidance.deck_mysekai"),
	"deck/score-up":               i18n.M("guidance.deck_score_up"),
	"education/area":              i18n.M("guidance.education_area"),
	"education/bonds":             i18n.M("guidance.education_bonds"),
	"education/challenge":         i18n.M("guidance.education_challenge"),
	"education/character-mission": i18n.M("guidance.education_character_mission"),
	"education/leader":            i18n.M("guidance.education_leader"),
	"education/power":             i18n.M("guidance.education_power"),
	"event":                       i18n.M("guidance.event"),
	"event/list":                  i18n.M("guidance.event_list"),
	"event/planner":               i18n.M("guidance.event_planner"),
	"event/record":                i18n.M("guidance.event_record"),
	"gacha":                       i18n.M("guidance.gacha"),
	"inventory/list":              i18n.M("guidance.inventory_list"),
	"misc/birthday":               i18n.M("guidance.misc_birthday"),
	"music":                       i18n.M("guidance.music"),
	"music/b30":                   i18n.M("guidance.music_b30"),
	"music/bpm":                   i18n.M("guidance.music_bpm"),
	"music/bpm-search":            i18n.M("guidance.music_bpm_search"),
	"music/chart":                 i18n.M("guidance.music_chart"),
	"music/cover":                 i18n.M("guidance.music_cover"),
	"music/list":                  i18n.M("guidance.music_list"),
	"music/note-count":            i18n.M("guidance.music_note_count"),
	"music/progress":              i18n.M("guidance.music_progress"),
	"music/rewards":               i18n.M("guidance.music_rewards"),
	"mysekai/blueprint":           i18n.M("guidance.mysekai_blueprint"),
	"mysekai/blueprint-term":      i18n.M("guidance.mysekai_blueprint_term"),
	"mysekai/door-upgrade":        i18n.M("guidance.mysekai_door_upgrade"),
	"mysekai/fixture-detail":      i18n.M("guidance.mysekai_fixture_detail"),
	"mysekai/fixture-list":        i18n.M("guidance.mysekai_fixture_list"),
	"mysekai/housing-sk":          i18n.M("guidance.mysekai_housing_sk"),
	"mysekai/map":                 i18n.M("guidance.mysekai_map"),
	"mysekai/music-record":        i18n.M("guidance.mysekai_music_record"),
	"mysekai/overview":            i18n.M("guidance.mysekai_overview"),
	"mysekai/photo":               i18n.M("guidance.mysekai_photo"),
	"mysekai/resource":            i18n.M("guidance.mysekai_resource"),
	"mysekai/shop":                i18n.M("guidance.mysekai_shop"),
	"mysekai/talk-list":           i18n.M("guidance.mysekai_talk_list"),
	"profile":                     i18n.M("guidance.profile"),
	"profile/arrest-difficulty":   i18n.M("guidance.profile_arrest_difficulty"),
	"profile/bg/adjust":           i18n.M("guidance.profile_bg_adjust"),
	"profile/bind":                i18n.M("guidance.profile_bind"),
	"profile/bind/swap":           i18n.M("guidance.profile_bind_swap"),
	"profile/chart-style":         i18n.M("guidance.profile_chart_style"),
	"profile/custom-profile-card": i18n.M("guidance.profile_custom_profile_card"),
	"profile/default":             i18n.M("guidance.profile_default"),
	"profile/info-panel":          i18n.M("guidance.profile_info_panel"),
	"profile/timezone":            i18n.M("guidance.profile_timezone"),
	"profile/uid":                 i18n.M("guidance.profile_uid"),
	"profile/unbind":              i18n.M("guidance.profile_unbind"),
	"score":                       i18n.M("guidance.score"),
	"score/custom-room":           i18n.M("guidance.score_custom_room"),
	"score/music-board":           i18n.M("guidance.score_music_board"),
	"score/music-meta":            i18n.M("guidance.score_music_meta"),
	"sk/check-room":               i18n.M("guidance.sk_check_room"),
	"sk/csb":                      i18n.M("guidance.sk_csb"),
	"sk/daily-speed":              i18n.M("guidance.sk_daily_speed"),
	"sk/line":                     i18n.M("guidance.sk_line"),
	"sk/player-trace":             i18n.M("guidance.sk_player_trace"),
	"sk/predict":                  i18n.M("guidance.sk_predict"),
	"sk/query":                    i18n.M("guidance.sk_query"),
	"sk/rank-trace":               i18n.M("guidance.sk_rank_trace"),
	"sk/speed":                    i18n.M("guidance.sk_speed"),
	"sk/winrate":                  i18n.M("guidance.sk_winrate"),
	"stamp":                       i18n.M("guidance.stamp"),
	"vlive":                       i18n.M("guidance.vlive")}

// guidanceForRoute returns the route's guidance message.
func guidanceForRoute(commandPath string) (i18n.Message, bool) {
	guidance, ok := routeGuidance[strings.Trim(strings.TrimSpace(commandPath), "/")]
	return guidance, ok
}
