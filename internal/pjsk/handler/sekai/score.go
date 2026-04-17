package sekai

import (
	"fmt"
	"haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/onebot11"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/region"
	rendermusic "haruki-cloud/internal/pjsk/render/music"
	"strconv"
	"strings"
)

type scoreControlParams struct {
	TargetPoint int    `json:"target_point"`
	Query       string `json:"query,omitempty"`
	WL          bool   `json:"wl,omitempty"`
}

type customRoomScoreParams struct {
	TargetPoint int `json:"target_point"`
}

type musicMetaQueriesParams struct {
	Queries []string `json:"queries"`
}

func (sekaiHandlers) ScoreControlHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "score",
			Commands: []string{
				"/分数", "/查分数", "/pjsk score", "/score control",
				"/控分",
			},
		},
		Regions:    []renderregion.Value{renderregion.JP},
		PrefixArgs: []string{"wl"},
		handleFunc: func(ctx SekaiHandlerContext) (*parser.ResolvedCommand, error) {
			params, err := buildScoreControlParams(ctx)
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleScore, "score-control", params), nil
		},
	}
}

func buildScoreControlParams(ctx SekaiHandlerContext) (scoreControlParams, error) {
	args := strings.TrimSpace(ctx.GetArgs())
	parts := strings.SplitN(args, " ", 2)
	if len(parts) == 0 {
		return scoreControlParams{}, onebot11.NewReplayError("使用方式:\n%s 活动pt 歌曲名(可选)", ctx.originalTriggerCmd)
	}

	targetPT, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || targetPT <= 0 {
		return scoreControlParams{}, onebot11.NewReplayError("使用方式:\n%s 活动pt 歌曲名(可选)", ctx.originalTriggerCmd)
	}

	params := scoreControlParams{
		TargetPoint: targetPT,
		WL:          ctx.PrefixArg() == "wl",
	}
	if len(parts) > 1 {
		params.Query = strings.TrimSpace(parts[1])
	}
	return params, nil
}

func (sekaiHandlers) CustomRoomScoreControlHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "score/custom-room",
			Commands: []string{
				"/pjsk custom room score", "/custom room score",
				"/自定义房间控分", "/自定义房控分", "/自定义控分",
				"/自定义房间分数", "/自定义分数",
			},
		},
		Regions: []renderregion.Value{renderregion.JP},
		handleFunc: func(ctx SekaiHandlerContext) (*parser.ResolvedCommand, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			targetPT, err := strconv.Atoi(args)
			if err != nil || targetPT <= 0 {
				return nil, onebot11.NewReplayError("使用方式: %s 目标PT", ctx.originalTriggerCmd)
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleScore, "score-custom-room", customRoomScoreParams{
				TargetPoint: targetPT,
			}), nil
		},
	}
}

func (sekaiHandlers) MusicMetaHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "score/music-meta",
			Commands: []string{
				"/pjsk music meta", "/music meta",
				"/歌曲meta", "/曲目meta",
			},
			Priority: 1,
		},
		handleFunc: func(ctx SekaiHandlerContext) (*parser.ResolvedCommand, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			clean := splitMusicMetaQueries(args)
			if len(clean) == 0 {
				return nil, fmt.Errorf("请至少提供一个歌曲ID或名称")
			}
			if len(clean) > 3 {
				return nil, fmt.Errorf("一次最多进行3首歌曲的比较")
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleScore, "score-music-meta", musicMetaQueriesParams{Queries: clean}), nil
		},
	}
}

func splitMusicMetaQueries(args string) []string {
	return rendermusic.SplitMusicQueries(args)
}

func (sekaiHandlers) MusicBoardHandle() SekaiCommandHandler {
	return SekaiCommandHandler{
		CommandHandlerBase: handler.CommandHandlerBase{
			Path: "score/music-board",
			Commands: []string{
				"/pjsk music board", "/music board",
				"/歌曲排行", "/歌曲比较", "/歌曲排名", "/曲目榜",
			},
			Priority: 1,
		},
		Regions: []renderregion.Value{renderregion.JP},
		handleFunc: func(ctx SekaiHandlerContext) (*parser.ResolvedCommand, error) {
			params, err := buildMusicBoardParams(ctx.GetArgs())
			if err != nil {
				return nil, err
			}
			return makeResolvedCmdWithParams(ctx, parser.ModuleScore, "score-music-board", params), nil
		},
	}
}
