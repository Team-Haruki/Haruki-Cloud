package sekai

import (
	"errors"
	"haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/parser"
	"strings"
)

const MUSIC_SEARCH_HELP = `请输入要查询的曲目，支持以下查询方式:
1. 直接使用曲目名称或别名
2. 显式曲目ID: music123
3. 纯歌曲入口兼容歌曲ID: 123 / id123
4. 曲目负数索引: 例如 -1 表示最新的曲目
5. 活动id: event123
6. 箱活: ick1`

func (sekaiHandlers) ChartHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "music/chart",
			Commands: []string{
				"/pjsk chart",
				"/谱面查询", "/铺面查询", "/谱面预览", "/铺面预览", "/谱面", "/铺面", "/查谱面", "/查铺面", "/查谱",
				"/技能预览",
			},
			Helper: MUSIC_SEARCH_HELP,
		},
		handleFunc: func(ctx SekaiHandlerContext) (any, error) {
			if strings.TrimSpace(ctx.GetArgs()) == "" {
				return nil, errors.New(MUSIC_SEARCH_HELP)
			}
			if ctx.GetTriggerCmd() == "/技能预览" {
				return makeResolvedCmdWithParams(ctx, parser.ModuleMusic, "music-chart", map[string]bool{
					"skill": true,
				}), nil
			}
			return makeResolvedCmd(ctx, parser.ModuleMusic, "music-chart"), nil
		},
	}
}
