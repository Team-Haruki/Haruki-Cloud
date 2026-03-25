package sekai

import (
	"fmt"
	"haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"strings"
)

func (sekaiHandlers) EventHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "event/list",
			Commands: []string{
				"/pjsk events", "/pjsk_events", "/events", "/活动列表", "/活动一览", "/event-list",
			},
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			return makeResolvedCmd(ctx, parser.ModuleEvent, "event-list"), nil
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
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			return makeResolvedCmd(ctx, parser.ModuleEvent, "event-detail"), nil
		},
	}
}

func (sekaiHandlers) EventStoryHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk event story", "/pjsk_event_story",
				"/活动剧情", "/活动故事", "/活动总结",
			},
			Disabled: true,
		},
		Regions: []renderregion.Value{renderregion.JP},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			refresh := false
			save := true
			if strings.Contains(args, "refresh") {
				refresh = true
				args = strings.TrimSpace(strings.ReplaceAll(args, "refresh", ""))
			}
			model := ""
			if strings.Contains(args, "model:") {
				parts := strings.SplitN(args, "model:", 2)
				args = strings.TrimSpace(parts[0])
				model = strings.TrimSpace(parts[1])
				refresh = true
				save = false
			}

			return nil, fmt.Errorf(
				"TODO: 活动剧情未实现，query=%q, refresh=%t, save=%t, model=%q",
				args, refresh, save, model,
			)
		},
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
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			return makeResolvedCmd(ctx, parser.ModuleEvent, "event-record"), nil
		},
	}
}
