package parser

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// EventQueryType 定义活动查询类型
type EventQueryType int

const (
	QueryTypeEventUnknown EventQueryType = iota
	QueryTypeEventID                     // 指定 ID: event123
	QueryTypeEventSeq                    // 索引: -1, 10, next, prev
	QueryTypeEventBan                    // Ban主: mnr1
	QueryTypeEventFilter                 // 筛选: 25h, wl
)

// EventFilter 活动筛选条件
type EventFilter struct {
	Unit         string // 25h, vbs, etc.
	EventType    string // marathon, cheerful_carnival, world_bloom
	Year         int    // 2024
	CharacterID  int    // 筛选单个角色的活动
	CharacterIDs []int  // 筛选多个角色的活动
	BannerCharID int    // 箱活ban主
	Blend        bool   // 混活
	Attr         string // cute, cool, etc.
}

// EventQueryInfo 解析后的活动查询信息
type EventQueryInfo struct {
	Type       EventQueryType
	EventID    int
	Index      int         // 正数或负数索引
	Keyword    string      // "next", "prev", "current"
	BanCharID  int         // Ban主角色ID
	BanSeq     int         // Ban主第几次箱活
	Filter     EventFilter // 筛选条件
	Original   string
	IsDetailed bool // 是否需要详细信息
}

// EventParser 活动查询解析器
type EventParser struct {
	nicknames        map[string]int
	orderedNicknames []string
}

// CharacterIDByNickname resolves a character nickname to character id.
func (p *EventParser) CharacterIDByNickname(token string) (int, bool) {
	if p == nil {
		return 0, false
	}
	cid, ok := p.nicknames[normalizeEventToken(token)]
	return cid, ok
}

// NewEventParser 创建解析器
func NewEventParser(nicknames map[string]int) *EventParser {
	normalized := make(map[string]int, len(nicknames))
	ordered := make([]string, 0, len(nicknames))
	for nickname, cid := range nicknames {
		key := normalizeEventToken(nickname)
		if key == "" {
			continue
		}
		if _, exists := normalized[key]; !exists {
			ordered = append(ordered, key)
		}
		normalized[key] = cid
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		return len([]rune(ordered[i])) > len([]rune(ordered[j]))
	})
	return &EventParser{
		nicknames:        normalized,
		orderedNicknames: ordered,
	}
}

// Parse 解析查询字符串
func (p *EventParser) Parse(args string) (*EventQueryInfo, error) {
	args = strings.TrimSpace(args)
	if args == "" {
		return nil, fmt.Errorf("活动查询参数不能为空")
	}

	if info := p.tryParseEventID(args); info != nil {
		return info, nil
	}

	if info := p.tryParseBanEvent(args); info != nil {
		return info, nil
	}

	if info := p.tryParseEventSeq(args); info != nil {
		return info, nil
	}

	if info := p.tryParseFilter(args); info != nil {
		return info, nil
	}

	return nil, fmt.Errorf("无法解析的活动指令: %s", args)
}

func (p *EventParser) tryParseEventID(args string) *EventQueryInfo {
	lower := normalizeEventToken(args)
	if strings.HasPrefix(lower, "event") {
		numPart := strings.TrimPrefix(lower, "event")
		id, err := strconv.Atoi(numPart)
		if err == nil {
			return &EventQueryInfo{
				Type:       QueryTypeEventID,
				EventID:    id,
				IsDetailed: true,
				Original:   args,
			}
		}
	}
	if isNumeric(args) {
		id, _ := strconv.Atoi(args)
		return &EventQueryInfo{
			Type:       QueryTypeEventID,
			EventID:    id,
			IsDetailed: true,
			Original:   args,
		}
	}
	return nil
}

func (p *EventParser) tryParseBanEvent(args string) *EventQueryInfo {
	normalized := normalizeEventToken(args)
	for _, nickname := range p.orderedNicknames {
		if strings.HasPrefix(normalized, nickname) {
			suffix := strings.TrimPrefix(normalized, nickname)
			if isNumeric(suffix) {
				seq, _ := strconv.Atoi(suffix)
				return &EventQueryInfo{
					Type:       QueryTypeEventBan,
					BanCharID:  p.nicknames[nickname],
					BanSeq:     seq,
					IsDetailed: true,
					Original:   args,
				}
			}
		}
	}
	return nil
}

func (p *EventParser) tryParseEventSeq(args string) *EventQueryInfo {
	lower := normalizeEventToken(args)
	if lower == "next" || lower == "下期" || lower == "下" {
		return &EventQueryInfo{Type: QueryTypeEventSeq, Keyword: "next", IsDetailed: true, Original: args}
	}
	if lower == "prev" || lower == "perv" || lower == "上期" || lower == "上" {
		return &EventQueryInfo{Type: QueryTypeEventSeq, Keyword: "prev", IsDetailed: true, Original: args}
	}
	if lower == "current" || lower == "curr" || lower == "当期" || lower == "今" {
		return &EventQueryInfo{Type: QueryTypeEventSeq, Keyword: "current", IsDetailed: true, Original: args}
	}

	if len(args) > 1 && (strings.HasPrefix(args, "-") || strings.HasPrefix(args, "+")) && isNumeric(args[1:]) {
		idx, _ := strconv.Atoi(strings.TrimSpace(args))
		return &EventQueryInfo{
			Type:       QueryTypeEventSeq,
			Index:      idx,
			IsDetailed: true,
			Original:   args,
		}
	}
	return nil
}

func (p *EventParser) tryParseFilter(args string) *EventQueryInfo {
	args = strings.TrimSpace(args)
	if args == "" {
		return nil
	}

	filter := EventFilter{}
	matched := false

	ext := NewExtractor(nil)
	yearRes := ext.ExtractYear(args)
	if yearRes.Found {
		filter.Year = yearRes.Value
		args = yearRes.Remaining
		matched = true
	}

	parts := strings.Fields(strings.ToLower(args))
	if len(parts) == 0 {
		if matched {
			return &EventQueryInfo{
				Type:     QueryTypeEventFilter,
				Filter:   filter,
				Original: strings.TrimSpace(args),
			}
		}
		return nil
	}

	units := map[string]string{
		"l/n": "light_sound", "ln": "light_sound", "leoneed": "light_sound",
		"mmj": "idol", "moremorejump": "idol",
		"vbs": "street", "vividbadsquad": "street",
		"ws": "theme_park", "wxs": "theme_park", "wonderlands": "theme_park",
		"25h": "school_refusal", "niigo": "school_refusal", "25": "school_refusal",
		"vs": "piapro", "virtualsinger": "piapro",
	}

	types := map[string]string{
		"marathon": "marathon", "普活": "marathon", "马拉松": "marathon",
		"cheerful": "cheerful_carnival", "5v5": "cheerful_carnival", "carnival": "cheerful_carnival",
		"wl": "world_bloom", "worldlink": "world_bloom", "world": "world_bloom",
	}

	attrAliases := map[string]string{
		"cute": "cute", "可爱": "cute", "粉": "cute",
		"cool": "cool", "帅气": "cool", "蓝": "cool",
		"pure": "pure", "纯真": "pure", "草": "pure", "绿": "pure",
		"happy": "happy", "快乐": "happy", "橙": "happy",
		"mysterious": "mysterious", "神秘": "mysterious", "紫": "mysterious",
	}

	charSet := make(map[int]struct{})
	for _, part := range parts {
		token := normalizeEventToken(part)
		if token == "" {
			continue
		}

		if u, ok := units[token]; ok {
			if filter.Blend {
				return nil
			}
			filter.Unit = u
			matched = true
			continue
		}

		if token == "混活" || token == "混" || token == "blend" || token == "mixed" {
			if filter.Unit != "" {
				return nil
			}
			filter.Blend = true
			matched = true
			continue
		}

		if t, ok := types[token]; ok {
			filter.EventType = t
			matched = true
			continue
		}

		if attr, ok := attrAliases[token]; ok {
			filter.Attr = attr
			matched = true
			continue
		}

		if strings.Contains(token, "箱") || strings.Contains(token, "ban") {
			bannerToken := strings.ReplaceAll(token, "箱", "")
			bannerToken = strings.ReplaceAll(bannerToken, "ban", "")
			if cid, ok := p.CharacterIDByNickname(bannerToken); ok {
				filter.BannerCharID = cid
				matched = true
				continue
			}
		}

		if cid, ok := p.CharacterIDByNickname(token); ok {
			charSet[cid] = struct{}{}
			matched = true
			continue
		}

		if token == "去年" {
			filter.Year = time.Now().Year() - 1
			matched = true
			continue
		}
		if token == "今年" {
			filter.Year = time.Now().Year()
			matched = true
			continue
		}

		if strings.HasSuffix(token, "年") {
			yStr := strings.TrimSuffix(token, "年")
			if isNumeric(yStr) {
				y, _ := strconv.Atoi(yStr)
				if y < 100 {
					y += 2000
				}
				filter.Year = y
				matched = true
				continue
			}
		}
		if isNumeric(token) {
			y, _ := strconv.Atoi(token)
			if y > 2019 && y < 2030 {
				filter.Year = y
				matched = true
				continue
			}
		}

		if token != "" {
			return nil
		}
	}

	if len(charSet) > 0 {
		filter.CharacterIDs = make([]int, 0, len(charSet))
		for cid := range charSet {
			filter.CharacterIDs = append(filter.CharacterIDs, cid)
		}
		sort.Ints(filter.CharacterIDs)
		if len(filter.CharacterIDs) == 1 {
			filter.CharacterID = filter.CharacterIDs[0]
		}
	}

	if matched {
		return &EventQueryInfo{
			Type:     QueryTypeEventFilter,
			Filter:   filter,
			Original: args,
		}
	}

	return nil
}

func normalizeEventToken(text string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(text)), ""))
}
