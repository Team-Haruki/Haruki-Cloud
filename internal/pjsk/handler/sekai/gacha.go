package sekai

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/parser"
)

func (sekaiHandlers) GachaHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "gacha",
			Commands: []string{
				"/pjsk gacha", "/卡池列表", "/卡池一览", "/卡池", "/查卡池",
			},
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			return makeResolvedCmd(ctx, parser.ModuleGacha, "gacha"), nil
		},
	}
}

func (sekaiHandlers) GachaRecordHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Commands: []string{
				"/pjsk gacha record", "/抽卡记录", "/抽卡历史",
			},
			Disabled: true,
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			specGIDs := make([]int, 0)
			if args != "" {
				for _, part := range strings.Fields(args) {
					gid, err := strconv.Atoi(part)
					if err != nil {
						return nil, fmt.Errorf("卡池ID参数错误: %s", part)
					}
					specGIDs = append(specGIDs, gid)
				}
			}
			return nil, errors.New("抽卡记录功能正在开发中，敬请期待")
		},
	}
}
