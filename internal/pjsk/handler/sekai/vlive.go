package sekai

import (
	"haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/parser"
)

func (sekaiHandlers) LiveHandle() HarukiSekaiCommandHandler {
	return HarukiSekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path:     "vlive",
			Commands: []string{"/pjsk live", "/虚拟live", "/pjsk vlive", "/vlive"},
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*parser.ResolvedCommand, error) {
			return makeResolvedCmd(ctx, parser.ModuleVLive, "vlive-list"), nil
		},
	}
}
