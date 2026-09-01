package handler

import (
	"fmt"
	"strings"

	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/region"
	rendercard "haruki-cloud/internal/pjsk/render/card"
	"haruki-cloud/internal/pjsk/render/event"
)

const querySingleEventHelp = `【查单个活动格式】
1. 活动ID：123
2. 倒数第几次活动：-1 -2
3. ban主昵称+序号：mnr1`

const queryMultiEventHelp = `【查多个活动格式】
1. 活动类型：5v5 普活 wl wl1 wl2 wl3
2. 颜色和团：紫 25h 仅25h
3. 年份：25年 去年
4. 活动角色：mnr hrk 可以加多个
5. 活动ban主：mnr箱`

const eventSearchHelp = querySingleEventHelp + "\n\n" + queryMultiEventHelp

func (sekaiHandlers) EventHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path: "event/list",
		Commands: []string{
			"/pjsk events", "/pjsk_events", "/events", "/活动列表", "/活动一览", "/event-list",
		},
		Helper: eventSearchHelp,
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			return resolveEventDetailOrList(ctx, true)
		},
	}, executeEvent)
}

func (sekaiHandlers) EventDetailHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path: "event",
		Commands: []string{
			"/pjsk event", "/pjsk_event", "/活动", "/查活动", "/event",
		},
		Helper: eventSearchHelp,
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			return resolveEventDetailOrList(ctx, false)
		},
	}, executeEvent)
}

func resolveEventDetailOrList(ctx HarrukiSekaiHandlerContext, preferList bool) (*CommandRequest, error) {
	args := strings.TrimSpace(ctx.GetArgs())
	if args == "" {
		return emptyEventQueryRequest(ctx, preferList), nil
	}
	if preferList {
		if params, ok := resolveAmbiguousEventListFilter(args); ok {
			return makeCommandRequestWithParams(ctx, parser.ModuleEvent, eventListCommand, params), nil
		}
	}

	info, err := parser.NewEventParser(rendercard.DefaultCharacterNicknames()).Parse(args)
	if err != nil {
		return nil, eventSearchUsageError(ctx.originalTriggerCmd)
	}
	if info.Type == parser.QueryTypeEventFilter {
		return makeCommandRequestWithParams(ctx, parser.ModuleEvent, eventListCommand, eventFilterParams(info.Filter)), nil
	}
	params, ok := eventDetailParams(info)
	if !ok {
		return nil, eventSearchUsageError(ctx.originalTriggerCmd)
	}
	return makeCommandRequestWithParams(ctx, parser.ModuleEvent, eventDetailCommand, params), nil
}

func emptyEventQueryRequest(ctx HarrukiSekaiHandlerContext, preferList bool) *CommandRequest {
	if preferList {
		return makeCommandRequestWithParams(ctx, parser.ModuleEvent, eventListCommand, map[string]any{"include_past": true, "include_future": true})
	}
	return makeCommandRequestWithParams(ctx, parser.ModuleEvent, eventDetailCommand, map[string]any{"use_current": true})
}

func eventFilterParams(filter parser.EventFilter) map[string]any {
	params := map[string]any{"include_past": true, "include_future": true}
	setNonZeroEventFilterParams(params, filter)
	return params
}

func setNonZeroEventFilterParams(params map[string]any, filter parser.EventFilter) {
	setStringParam(params, "event_type", filter.EventType)
	setIntParam(params, "world_bloom_turn", filter.WorldBloomTurn)
	setStringParam(params, "unit", filter.Unit)
	setBoolParam(params, "only_unit", filter.OnlyUnit)
	setBoolParam(params, "blend", filter.Blend)
	setStringParam(params, "attr", filter.Attr)
	setIntParam(params, "year", filter.Year)
	setIntParam(params, "character_id", filter.CharacterID)
	if len(filter.CharacterIDs) > 0 {
		params["character_ids"] = filter.CharacterIDs
	}
	setIntParam(params, "banner_char_id", filter.BannerCharID)
}

func setStringParam(params map[string]any, key, value string) {
	if value != "" {
		params[key] = value
	}
}

func setIntParam(params map[string]any, key string, value int) {
	if value != 0 {
		params[key] = value
	}
}

func setBoolParam(params map[string]any, key string, value bool) {
	if value {
		params[key] = true
	}
}

func eventDetailParams(info *parser.EventQueryInfo) (map[string]any, bool) {
	params := make(map[string]any)
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
		return nil, false
	}
	return params, true
}

func eventSearchUsageError(trigger string) error {
	return onebot11.NewReplayError("活动查询参数格式不正确。查看完整用法请发送：%s -help", trigger)
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

func (sekaiHandlers) EventRecordHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path: "event/record",
		Commands: []string{
			"/pjsk event record", "/pjsk_event_record",
			"/活动记录", "/冲榜记录",
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			params, err := newSelfQueryParamsMap(ctx)
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleEvent, "event-record", params), nil
		},
	}, executeEvent)
}

func executeEvent(rc *RequestContext) (message onebot11.Message, err error) {
	defer func() {
		region := ""
		if rc != nil && rc.Cmd != nil {
			region = rc.Cmd.Region
		}
		err = normalizeEventUserFacingErrorForRegion(err, region)
	}()

	region := renderregion.Value(rc.Cmd.Region)
	switch rc.Cmd.Mode {
	case "event-planner-help":
		return onebot11.Message{onebot11.Text(eventPlannerHelp)}, nil
	case "event-planner":
		return executeEventPlanner(rc)
	}

	if rc.App.Events == nil {
		return nil, fmt.Errorf("event service unavailable: sekai client not configured")
	}
	eventCtrl := rc.App.Events.WithContext(rc.Ctx)
	var data []byte
	switch rc.Cmd.Mode {
	case eventDetailCommand:
		q := event.DetailQuery{Region: region}
		mergeParams(rc.Cmd.Params, &q)
		q.AllowUnreleased = allowReadOnlyLeaks(region.String())
		data, err = eventCtrl.RenderEventDetail(q)
	case eventListCommand:
		q := event.ListQuery{Region: region}
		mergeParams(rc.Cmd.Params, &q)
		data, err = eventCtrl.RenderEventList(q)
	case "event-record":
		finishBuild := measurePayloadBuild(rc.Ctx)
		req, buildErr := buildEventRecordFromSnapshot(rc, region)
		finishBuild()
		if buildErr != nil {
			return nil, buildErr
		}
		data, err = eventCtrl.RenderEventRecord(*req)
	default:
		return nil, unsupportedModeError("event", rc.Cmd.Mode)
	}
	if err != nil {
		return nil, err
	}
	return rc.ImageMessage(data)
}
