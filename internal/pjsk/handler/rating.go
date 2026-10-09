package handler

import (
	"haruki-cloud/internal/i18n"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/parser"
)

func (sekaiHandlers) B30Handle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path: "music/b30",
		Commands: []string{
			"/b30", "/pjskb30",
			"/b39", "/pjskb39",
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			return makeCommandRequest(ctx, parser.ModuleMusic, "rating-unavailable"), nil
		},
	}, executeRatingUnavailable)
}

func executeRatingUnavailable(*RequestContext) (onebot11.Message, error) {
	return onebot11.Message{onebot11.Text(i18n.T("music.b30.unavailable"))}, nil
}
