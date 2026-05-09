package education

import (
	"sort"
	"strings"

	"haruki-cloud/internal/pjsk/drawing"
)

var CharacterMissionShortNames = map[string]string{
	"play_live":                                  "队长次数",
	"play_live_ex":                               "队长次数(EX)",
	"waiting_room":                               "休息室次数",
	"waiting_room_ex":                            "休息室次数(EX)",
	"collect_costume_3d":                         "服装",
	"collect_stamp":                              "表情",
	"read_area_talk":                             "区域对话",
	"read_card_episode_first":                    "卡面剧情前篇",
	"read_card_episode_second":                   "卡面剧情后篇",
	"collect_another_vocal":                      "Another Vocal",
	"area_item_level_up_character":               "单人家具升级次数",
	"area_item_level_up_unit":                    "团家具升级次数",
	"area_item_level_up_reality_world":           "属性道具（树&花）升级次数",
	"collect_member":                             "卡面",
	"skill_level_up_rare":                        "技能等级升级次数（★4&生日卡）",
	"skill_level_up_standard":                    "技能等级升级次数（★1~★3）",
	"master_rank_up_rare":                        "专精等级升级次数（★4&生日卡）",
	"master_rank_up_standard":                    "专精等级升级次数（★1~★3）",
	"collect_character_archive_voice":            "台词",
	"collect_mysekai_fixture":                    "MySekai家具数量",
	"collect_mysekai_canvas":                     "MySekai画布数量",
	"read_mysekai_fixture_unique_character_talk": "MySekai对话",
}

var CharacterMissionExTypes = map[string]struct{}{
	"play_live_ex":    {},
	"waiting_room_ex": {},
}

var CharacterMissionExBaseTypes = map[string]struct{}{
	"play_live":    {},
	"waiting_room": {},
}

var CharacterMissionAllKeywords = []string{"all", "全部", "全量", "总表", "表格"}

var characterMissionTypeAliases = map[string][]string{
	"play_live":                                  {"队长次数", "角色次数", "队长游玩次数", "角色游玩次数", "队长", "队长次数ex", "队长次数(ex)", "队长次数（ex）", "角色次数ex"},
	"waiting_room":                               {"休息室次数", "休息室", "控制室", "休息室次数ex", "休息室次数(ex)", "休息室次数（ex）"},
	"collect_costume_3d":                         {"服装", "衣装", "服装数量", "衣装数量"},
	"collect_stamp":                              {"表情", "贴纸", "贴纸数量", "表情数量"},
	"read_area_talk":                             {"区域对话"},
	"read_card_episode_first":                    {"卡面剧情前篇", "前篇", "前编"},
	"read_card_episode_second":                   {"卡面剧情后篇", "后篇", "后编"},
	"collect_another_vocal":                      {"another vocal", "anvo"},
	"area_item_level_up_character":               {"单人家具升级次数", "单人家具", "单人道具"},
	"area_item_level_up_unit":                    {"团家具升级次数", "团家具"},
	"area_item_level_up_reality_world":           {"属性道具（树&花）升级次数", "树花", "属性家具", "属性道具", "植物"},
	"collect_member":                             {"卡面", "图鉴", "成员"},
	"skill_level_up_rare":                        {"技能等级升级次数（★4&生日卡）", "4星技能", "四星技能", "四星slv", "4星slv"},
	"skill_level_up_standard":                    {"技能等级升级次数（★1~★3）", "低星技能", "低星slv"},
	"master_rank_up_rare":                        {"专精等级升级次数（★4&生日卡）", "4星专精", "四星专精", "四星突破", "4星突破", "4星mr", "四星mr"},
	"master_rank_up_standard":                    {"专精等级升级次数（★1~★3）", "低星专精", "低星突破", "低星mr"},
	"collect_character_archive_voice":            {"台词", "语音"},
	"collect_mysekai_fixture":                    {"mysekai家具数量", "ms家具", "烤森家具"},
	"collect_mysekai_canvas":                     {"mysekai画布数量", "ms画布", "烤森画布"},
	"read_mysekai_fixture_unique_character_talk": {"mysekai对话", "ms对话", "烤森对话"},
}

func init() {
	for missionType, shortName := range CharacterMissionShortNames {
		normalized := normalizeCharacterMissionQuery(shortName)
		characterMissionTypeAliases[missionType] = append(characterMissionTypeAliases[missionType], normalized, strings.ToLower(missionType))
	}
}

func CharacterMissionShortName(missionType string) string {
	if value, ok := CharacterMissionShortNames[missionType]; ok {
		return value
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
