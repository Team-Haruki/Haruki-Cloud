package sekai

import (
	"haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/parser"
	"strconv"
	"strings"
)

func (sekaiHandlers) StampHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "stamp",
			Commands: []string{
				"/贴纸", "/查贴纸", "/pjsk贴纸", "/pjsk表情", "/pjsk stamp", "/pjsk bq", "/stamp",
			},
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			if params := parseStampIDs(args); len(params) > 0 {
				ctx.SetArgs("")
				return makeResolvedCmdWithParams(ctx, parser.ModuleStamp, "stamp-list", map[string]any{
					"ids": params,
				}), nil
			}
			return makeResolvedCmd(ctx, parser.ModuleStamp, "stamp-list"), nil
		},
	}
}

func parseStampIDs(args string) []int {
	fields := strings.Fields(strings.TrimSpace(args))
	if len(fields) == 0 {
		return nil
	}
	ids := make([]int, 0, len(fields))
	for _, field := range fields {
		id, err := strconv.Atoi(field)
		if err != nil || id <= 0 {
			return nil
		}
		ids = append(ids, id)
	}
	return ids
}
