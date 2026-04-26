package filteralias

type aliasGroup struct {
	key     string
	aliases []string
}

var attributeGroups = []aliasGroup{
	{key: "cute", aliases: []string{"cute", "可爱", "粉花", "粉", "pink"}},
	{key: "cool", aliases: []string{"cool", "帅气", "蓝星", "蓝", "blue"}},
	{key: "pure", aliases: []string{"pure", "纯真", "纯洁", "绿草", "草", "绿", "green"}},
	{key: "happy", aliases: []string{"happy", "快乐", "橙心", "橙", "黄", "orange"}},
	{key: "mysterious", aliases: []string{"mysterious", "神秘", "紫月", "紫", "purple"}},
}

var unitGroups = []aliasGroup{
	{key: "light_sound", aliases: []string{"l/n", "ln", "leoneed", "light_sound_club", "light_sound", "lightsound", "leo/need"}},
	{key: "idol", aliases: []string{"mmj", "moremorejump", "more_more_jump", "idol"}},
	{key: "street", aliases: []string{"vbs", "vividbadsquad", "vivid_bad_squad", "street"}},
	{key: "theme_park", aliases: []string{"ws", "wxs", "wonderlands", "wonderlandsxshowtime", "wonderlands_x_showtime", "theme_park", "themepark"}},
	{key: "school_refusal", aliases: []string{"25", "25h", "25时", "25ji", "25_ji_night_cord_de", "niigo", "nightcord", "school_refusal", "schoolrefusal"}},
	{key: "piapro", aliases: []string{"vs", "v", "piapro", "virtualsinger", "vocaloid"}},
}

var supplyGroups = []aliasGroup{
	{key: "festival", aliases: []string{"fes"}},
	{key: "colorful_festival_limited", aliases: []string{"cfes限定", "cfes"}},
	{key: "bloom_festival_limited", aliases: []string{"bfes限定", "bfes"}},
	{key: "unit_event_limited", aliases: []string{"worldlink限定", "wl限定"}},
	{key: "collaboration_limited", aliases: []string{"联动限定", "联动", "collab"}},
	{key: "limited", aliases: []string{"期间限定", "限定", "limit"}},
	{key: "normal", aliases: []string{"非限定", "常驻", "非限"}},
	{key: "birthday", aliases: []string{"生日"}},
}

func AttributeGroups() map[string][]string {
	return cloneGroupMap(attributeGroups, nil, nil)
}

func AttributeMap() map[string]string {
	return flattenGroups(attributeGroups, nil, nil)
}

func AttributeAliasSet() map[string]struct{} {
	return aliasSet(attributeGroups, nil, nil)
}

func UnitMap() map[string]string {
	return flattenGroups(unitGroups, nil, nil)
}

func UnitMapWithout(keys ...string) map[string]string {
	return flattenGroups(unitGroups, nil, newKeySet(keys...))
}

func UnitAliasSet() map[string]struct{} {
	return aliasSet(unitGroups, nil, nil)
}

func UnitAliasSetFor(keys ...string) map[string]struct{} {
	return aliasSet(unitGroups, newKeySet(keys...), nil)
}

func SupplyMap(keys ...string) map[string]string {
	return flattenGroups(supplyGroups, newKeySet(keys...), nil)
}

func cloneGroupMap(groups []aliasGroup, include map[string]struct{}, exclude map[string]struct{}) map[string][]string {
	result := make(map[string][]string)
	for _, group := range groups {
		if !shouldIncludeGroup(group.key, include, exclude) {
			continue
		}
		result[group.key] = append([]string(nil), group.aliases...)
	}
	return result
}

func flattenGroups(groups []aliasGroup, include map[string]struct{}, exclude map[string]struct{}) map[string]string {
	result := make(map[string]string)
	for _, group := range groups {
		if !shouldIncludeGroup(group.key, include, exclude) {
			continue
		}
		for _, alias := range group.aliases {
			result[alias] = group.key
		}
	}
	return result
}

func aliasSet(groups []aliasGroup, include map[string]struct{}, exclude map[string]struct{}) map[string]struct{} {
	result := make(map[string]struct{})
	for _, group := range groups {
		if !shouldIncludeGroup(group.key, include, exclude) {
			continue
		}
		for _, alias := range group.aliases {
			result[alias] = struct{}{}
		}
	}
	return result
}

func shouldIncludeGroup(key string, include map[string]struct{}, exclude map[string]struct{}) bool {
	if include != nil {
		if _, ok := include[key]; !ok {
			return false
		}
	}
	if exclude != nil {
		if _, ok := exclude[key]; ok {
			return false
		}
	}
	return true
}

func newKeySet(keys ...string) map[string]struct{} {
	if len(keys) == 0 {
		return nil
	}
	result := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		if key == "" {
			continue
		}
		result[key] = struct{}{}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
