package sekai

import (
	"haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/parser"
)

func (sekaiHandlers) LiveHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path:     "vlive",
			Commands: []string{"/pjsk live", "/虚拟live", "/pjsk vlive", "/vlive"},
		},
		handleFunc: func(ctx SekaiHandlerContext) (interface{}, error) {
			return makeResolvedCmd(ctx, parser.ModuleVLive, "vlive-list"), nil
		},
	}
}
