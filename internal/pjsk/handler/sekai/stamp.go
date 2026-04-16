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
		handleFunc: func(ctx SekaiHandlerContext) (*parser.ResolvedCommand, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			if page, remaining, ok := parseStampPageWithRemaining(args); ok {
				ctx.SetArgs(remaining)
				return makeResolvedCmdWithParams(ctx, parser.ModuleStamp, "stamp-list", map[string]any{
					"page": page,
				}), nil
			}
			if parseStampAll(args) {
				ctx.SetArgs("")
				return makeResolvedCmdWithParams(ctx, parser.ModuleStamp, "stamp-list", map[string]any{
					"all": true,
				}), nil
			}
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

func parseStampPageWithRemaining(args string) (int, string, bool) {
	fields := strings.Fields(strings.TrimSpace(args))
	if len(fields) < 2 {
		return 0, "", false
	}

	for i := 0; i < len(fields)-1; i++ {
		switch strings.ToLower(fields[i]) {
		case "page", "p", "页":
		default:
			continue
		}

		page, err := strconv.Atoi(fields[i+1])
		if err != nil || page <= 0 {
			return 0, "", false
		}

		remainingFields := make([]string, 0, len(fields)-2)
		remainingFields = append(remainingFields, fields[:i]...)
		remainingFields = append(remainingFields, fields[i+2:]...)
		return page, strings.Join(remainingFields, " "), true
	}

	return 0, "", false
}

func parseStampAll(args string) bool {
	value := strings.TrimSpace(strings.ToLower(args))
	switch value {
	case "all", "全部", "所有":
		return true
	default:
		return false
	}
}
