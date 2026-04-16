package sekai

import (
	"fmt"
	"strings"

	"haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/parser"
	rendercard "haruki-cloud/internal/pjsk/render/card"
)

const querySingleEventHelp = `【查单个活动格式】
1. 活动ID：123
2. 倒数第几次活动：-1 -2
3. ban主昵称+序号：mnr1`

const queryMultiEventHelp = `【查多个活动格式】
1. 活动类型：5v5 普活 wl
2. 颜色和团：紫 25h
3. 年份：25年 去年
4. 活动角色：mnr hrk 可以加多个
5. 活动ban主：mnr箱`

const eventSearchHelp = querySingleEventHelp + "\n\n" + queryMultiEventHelp

func (sekaiHandlers) EventHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "event/list",
			Commands: []string{
				"/pjsk events", "/pjsk_events", "/events", "/活动列表", "/活动一览", "/event-list",
			},
			Helper: eventSearchHelp,
		},
		handleFunc: func(ctx SekaiHandlerContext) (*parser.ResolvedCommand, error) {
			return resolveEventDetailOrList(ctx, true)
		},
	}
}

func (sekaiHandlers) EventDetailHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "event",
			Commands: []string{
				"/pjsk event", "/pjsk_event", "/活动", "/查活动", "/event",
			},
			Helper: eventSearchHelp,
		},
		handleFunc: func(ctx SekaiHandlerContext) (*parser.ResolvedCommand, error) {
			return resolveEventDetailOrList(ctx, false)
		},
	}
}

func resolveEventDetailOrList(ctx SekaiHandlerContext, preferList bool) (*parser.ResolvedCommand, error) {
	args := strings.TrimSpace(ctx.GetArgs())
	if args == "" {
		if preferList {
			return makeResolvedCmdWithParams(ctx, parser.ModuleEvent, "event-list", map[string]any{
				"include_past":   true,
				"include_future": true,
			}), nil
		}
		return makeResolvedCmdWithParams(ctx, parser.ModuleEvent, "event-detail", map[string]any{
			"use_current": true,
		}), nil
	}

	if preferList {
		if params, ok := resolveAmbiguousEventListFilter(args); ok {
			return makeResolvedCmdWithParams(ctx, parser.ModuleEvent, "event-list", params), nil
		}
	}

	info, err := parser.NewEventParser(rendercard.DefaultCharacterNicknames()).Parse(args)
	if err != nil {
		return nil, fmt.Errorf("活动查询参数错误: %q\n%s\n%s", args, querySingleEventHelp, queryMultiEventHelp)
	}

	if info.Type == parser.QueryTypeEventFilter {
		params := map[string]any{
			"include_past":   true,
			"include_future": true,
		}
		if info.Filter.EventType != "" {
			params["event_type"] = info.Filter.EventType
		}
		if info.Filter.Unit != "" {
			params["unit"] = info.Filter.Unit
		}
		if info.Filter.Blend {
			params["blend"] = true
		}
		if info.Filter.Attr != "" {
			params["attr"] = info.Filter.Attr
		}
		if info.Filter.Year != 0 {
			params["year"] = info.Filter.Year
		}
		if info.Filter.CharacterID != 0 {
			params["character_id"] = info.Filter.CharacterID
		}
		if len(info.Filter.CharacterIDs) > 0 {
			params["character_ids"] = info.Filter.CharacterIDs
		}
		if info.Filter.BannerCharID != 0 {
			params["banner_char_id"] = info.Filter.BannerCharID
		}
		return makeResolvedCmdWithParams(ctx, parser.ModuleEvent, "event-list", params), nil
	}

	params := map[string]any{}
	switch info.Type {
	case parser.QueryTypeEventID:
		params["event_id"] = info.EventID
	case parser.QueryTypeEventBan:
		params["ban_char_id"] = info.BanCharID
		params["ban_seq"] = info.BanSeq
	case parser.QueryTypeEventSeq:
		if info.Keyword != "" {
			if info.Keyword == "current" {
				params["use_current"] = true
			} else {
				params["keyword"] = info.Keyword
			}
		} else {
			params["index"] = info.Index
		}
	default:
		return nil, fmt.Errorf("活动查询参数错误: %q\n%s\n%s", args, querySingleEventHelp, queryMultiEventHelp)
	}
	return makeResolvedCmdWithParams(ctx, parser.ModuleEvent, "event-detail", params), nil
}

func resolveAmbiguousEventListFilter(args string) (map[string]any, bool) {
	switch strings.ToLower(strings.TrimSpace(args)) {
	case "25":
		return map[string]any{
			"include_past":   true,
			"include_future": true,
			"unit":           "school_refusal",
		}, true
	default:
		return nil, false
	}
}

func (sekaiHandlers) EventRecordHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "event/record",
			Commands: []string{
				"/pjsk event record", "/pjsk_event_record",
				"/活动记录", "/冲榜记录",
			},
		},
		handleFunc: func(ctx SekaiHandlerContext) (*parser.ResolvedCommand, error) {
			params, err := newSelfQueryParamsMap(ctx)
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleEvent, "event-record", params), nil
		},
	}
}
