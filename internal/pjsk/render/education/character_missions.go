package education

import (
	"sort"
	"strings"

	"haruki-cloud/internal/i18n"
	"haruki-cloud/internal/pjsk/drawing"
)

// characterMissionTitles are the display names of the character mission
// types, shown as row and column titles in the character mission images.
var characterMissionTitles = map[string]i18n.Message{
	"play_live":                                  i18n.M("education.mission.play_live"),
	"play_live_ex":                               i18n.M("education.mission.play_live_ex"),
	"waiting_room":                               i18n.M("education.mission.waiting_room"),
	"waiting_room_ex":                            i18n.M("education.mission.waiting_room_ex"),
	"collect_costume_3d":                         i18n.M("education.mission.collect_costume_3d"),
	"collect_stamp":                              i18n.M("education.mission.collect_stamp"),
	"read_area_talk":                             i18n.M("education.mission.read_area_talk"),
	"read_card_episode_first":                    i18n.M("education.mission.read_card_episode_first"),
	"read_card_episode_second":                   i18n.M("education.mission.read_card_episode_second"),
	"collect_another_vocal":                      i18n.M("education.mission.collect_another_vocal"),
	"area_item_level_up_character":               i18n.M("education.mission.area_item_level_up_character"),
	"area_item_level_up_unit":                    i18n.M("education.mission.area_item_level_up_unit"),
	"area_item_level_up_reality_world":           i18n.M("education.mission.area_item_level_up_reality_world"),
	"area_item_level_up_all_character":           i18n.M("education.mission.area_item_level_up_all_character"),
	"collect_member":                             i18n.M("education.mission.collect_member"),
	"skill_level_up_rare":                        i18n.M("education.mission.skill_level_up_rare"),
	"skill_level_up_standard":                    i18n.M("education.mission.skill_level_up_standard"),
	"master_rank_up_rare":                        i18n.M("education.mission.master_rank_up_rare"),
	"master_rank_up_standard":                    i18n.M("education.mission.master_rank_up_standard"),
	"collect_character_archive_voice":            i18n.M("education.mission.collect_character_archive_voice"),
	"collect_mysekai_fixture":                    i18n.M("education.mission.collect_mysekai_fixture"),
	"collect_mysekai_canvas":                     i18n.M("education.mission.collect_mysekai_canvas"),
	"read_mysekai_fixture_unique_character_talk": i18n.M("education.mission.read_mysekai_fixture_unique_character_talk"),
}

var CharacterMissionExTypes = map[string]struct{}{
	"play_live_ex":    {},
	"waiting_room_ex": {},
}

var CharacterMissionExBaseTypes = map[string]struct{}{
	"play_live":    {},
	"waiting_room": {},
}

//copylint:ignore-block 解析关键字
var CharacterMissionAllKeywords = []string{"all", "全部", "全量", "总表", "表格"}

//copylint:ignore-block 解析关键字
var characterMissionTypeAliases = map[string][]string{
	"play_live":                                  {"队长次数", "角色次数", "队长游玩次数", "角色游玩次数", "队长", "队长次数ex", "队长次数(ex)", "角色次数ex"},
	"waiting_room":                               {"休息室次数", "休息室", "控制室", "休息室次数ex", "休息室次数(ex)"},
	"collect_costume_3d":                         {"服装", "衣装", "服装数量", "衣装数量"},
	"collect_stamp":                              {"表情", "贴纸", "贴纸数量", "表情数量"},
	"read_area_talk":                             {"区域对话"},
	"read_card_episode_first":                    {"卡面剧情前篇", "前篇", "前编"},
	"read_card_episode_second":                   {"卡面剧情后篇", "后篇", "后编"},
	"collect_another_vocal":                      {"another vocal", "anvo"},
	"area_item_level_up_character":               {"单人家具升级次数", "单人家具", "单人道具"},
	"area_item_level_up_unit":                    {"团家具升级次数", "团家具"},
	"area_item_level_up_reality_world":           {"属性道具(树&花)升级次数", "花树", "树花", "属性家具", "属性道具", "植物"},
	"area_item_level_up_all_character":           {"想いの大樹升级次数", "想いの大樹", "想いの大树", "大樹", "大树", "思念之树", "全员家具", "全角色家具"},
	"collect_member":                             {"卡面", "图鉴", "成员"},
	"skill_level_up_rare":                        {"技能等级升级次数(★4&生日卡)", "4星技能", "四星技能", "四星slv", "4星slv"},
	"skill_level_up_standard":                    {"技能等级升级次数(★1~★3)", "低星技能", "低星slv"},
	"master_rank_up_rare":                        {"专精等级升级次数(★4&生日卡)", "4星专精", "四星专精", "四星突破", "4星突破", "4星mr", "四星mr"},
	"master_rank_up_standard":                    {"专精等级升级次数(★1~★3)", "低星专精", "低星突破", "低星mr"},
	"collect_character_archive_voice":            {"台词", "语音"},
	"collect_mysekai_fixture":                    {"mysekai家具数量", "ms家具", "烤森家具"},
	"collect_mysekai_canvas":                     {"mysekai画布数量", "ms画布", "烤森画布"},
	"read_mysekai_fixture_unique_character_talk": {"mysekai对话", "ms对话", "烤森对话"},
}

// The aliases above are matched after normalizeCharacterMissionQuery, which
// turns full-width parentheses into half-width ones. Every mission type key
// itself (e.g. "play_live") is accepted as well.
func init() {
	for missionType := range characterMissionTitles {
		characterMissionTypeAliases[missionType] = append(characterMissionTypeAliases[missionType], strings.ToLower(missionType))
	}
}

// CharacterMissionShortName is the display name of a character mission
// type; an unknown type is shown as is.
func CharacterMissionShortName(missionType string) string {
	if title, ok := characterMissionTitles[missionType]; ok {
		return title.String()
	}
	return missionType
}

func normalizeCharacterMissionQuery(text string) string {
	return strings.ToLower(strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(text, "（", "("), "）", ")")))
}

func ExtractCharacterMissionAllFlag(args string) (bool, string) {
	normalized := normalizeCharacterMissionQuery(args)
	for _, keyword := range CharacterMissionAllKeywords {
		if strings.Contains(normalized, keyword) {
			return true, strings.TrimSpace(strings.Replace(args, keyword, "", 1))
		}
	}
	return false, strings.TrimSpace(args)
}

func ExtractCharacterMissionType(args string) (string, string) {
	normalized := normalizeCharacterMissionQuery(args)
	pairs := make([]struct {
		missionType string
		alias       string
	}, 0)
	for missionType, aliases := range characterMissionTypeAliases {
		for _, alias := range aliases {
			pairs = append(pairs, struct {
				missionType string
				alias       string
			}{missionType: missionType, alias: alias})
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		return len(pairs[i].alias) > len(pairs[j].alias)
	})
	for _, pair := range pairs {
		if strings.Contains(normalized, pair.alias) {
			return pair.missionType, ""
		}
	}
	return "", strings.TrimSpace(args)
}

func characterMissionUpperPtr(value *int) *int {
	if value == nil {
		return nil
	}
	v := *value
	return &v
}

func characterMissionNextPtr(value int) *int {
	if value <= 0 {
		return nil
	}
	v := value
	return &v
}

func characterMissionRoundPtr(value int) *int {
	if value <= 0 {
		return nil
	}
	v := value
	return &v
}

func characterMissionRoundTextPtr(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	v := value
	return &v
}

func characterMissionOverviewRowClone(source drawing.CharacterMissionOverviewRow) drawing.CharacterMissionOverviewRow {
	return drawing.CharacterMissionOverviewRow{
		MissionID:            source.MissionID,
		MissionType:          source.MissionType,
		Title:                source.Title,
		IsAchievement:        source.IsAchievement,
		IsEx:                 source.IsEx,
		Current:              source.Current,
		Upper:                characterMissionUpperPtr(source.Upper),
		Ratio:                source.Ratio,
		NextNeed:             characterMissionUpperPtr(source.NextNeed),
		NextExp:              characterMissionUpperPtr(source.NextExp),
		CurrentRound:         characterMissionUpperPtr(source.CurrentRound),
		CurrentRoundProgress: characterMissionUpperPtr(source.CurrentRoundProgress),
		CurrentRoundNeed:     characterMissionUpperPtr(source.CurrentRoundNeed),
		ExDisplayRoundText:   characterMissionRoundTextPtr(derefString(source.ExDisplayRoundText)),
	}
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
