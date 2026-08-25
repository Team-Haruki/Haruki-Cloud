package handler

import (
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/parser"
)

const ratingUnavailableMessage = "官方并没有提供详细定数，故不提供该服务"

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
	return onebot11.Message{onebot11.Text(ratingUnavailableMessage)}, nil
}
