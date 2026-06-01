package handler

import (
	"fmt"
	json "github.com/bytedance/sonic"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/region"
	rendermusic "haruki-cloud/internal/pjsk/render/music"
	"haruki-cloud/internal/pjsk/requestbuilder"
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

func (sekaiHandlers) ScoreControlHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path: "score",
			Commands: []string{
				"/分数", "/查分数", "/pjsk score", "/score control",
				"/控分",
			},
		},
		Regions:    []renderregion.Value{renderregion.JP},
		PrefixArgs: []string{"wl"},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			params, err := buildScoreControlParams(ctx)
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleScore, "score-control", params), nil
		},
	}, executeScore)
}

func buildScoreControlParams(ctx HarrukiSekaiHandlerContext) (scoreControlParams, error) {
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

func (sekaiHandlers) CustomRoomScoreControlHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path: "score/custom-room",
			Commands: []string{
				"/pjsk custom room score", "/custom room score",
				"/自定义房间控分", "/自定义房控分", "/自定义控分",
				"/自定义房间分数", "/自定义分数",
			},
		},
		Regions: []renderregion.Value{renderregion.JP},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			targetPT, err := strconv.Atoi(args)
			if err != nil || targetPT <= 0 {
				return nil, onebot11.NewReplayError("使用方式: %s 目标PT", ctx.originalTriggerCmd)
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleScore, "score-custom-room", customRoomScoreParams{
				TargetPoint: targetPT,
			}), nil
		},
	}, executeScore)
}

func (sekaiHandlers) MusicMetaHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path: "score/music-meta",
			Commands: []string{
				"/pjsk music meta", "/music meta",
				"/歌曲meta", "/曲目meta",
			},
			Priority: 1,
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			clean := splitMusicMetaQueries(args)
			if len(clean) == 0 {
				return nil, fmt.Errorf("请至少提供一个歌曲ID或名称")
			}
			if len(clean) > 3 {
				return nil, fmt.Errorf("一次最多进行3首歌曲的比较")
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleScore, "score-music-meta", musicMetaQueriesParams{Queries: clean}), nil
		},
	}, executeScore)
}

func splitMusicMetaQueries(args string) []string {
	return rendermusic.SplitMusicQueries(args)
}

func (sekaiHandlers) MusicBoardHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		CommandHandlerBase: CommandHandlerBase{
			Path: "score/music-board",
			Commands: []string{
				"/pjsk music board", "/music board",
				"/歌曲排行", "/歌曲比较", "/歌曲对比", "/歌曲排名", "/曲目榜",
			},
			Priority: 1,
		},
		Regions: []renderregion.Value{renderregion.JP},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			params, err := buildMusicBoardParams(ctx.GetArgs())
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleScore, "score-music-board", params), nil
		},
	}, executeScore)
}

func executeScore(rc *RequestContext) (message onebot11.Message, err error) {
	var musicCtrl *rendermusic.Controller
	scoreCtrl := rc.App.Score
	if scoreCtrl != nil {
		scoreCtrl = scoreCtrl.WithContext(rc.Ctx)
	}
	if rc.App != nil && rc.App.Music != nil {
		musicCtrl = rc.App.Music.WithContext(rc.Ctx)
		if rc.App.Aliases != nil {
			musicCtrl.SetAliasResolver(rc.App.Aliases)
		}
	}
	var data []byte
	switch rc.Cmd.Mode {
	case "score-control":
		req := drawing.ScoreControlRequest{}
		mergeParams(rc.Cmd.Params, &req)
		if req.MusicID <= 0 || req.TargetPoint <= 0 || len(req.ValidScores) == 0 {
			reqPtr, resolveErr := requestbuilder.BuildScoreControlRequest(rc.Ctx, toRequestBuilderCommandInput(rc.Cmd), rc.App)
			if resolveErr != nil {
				return nil, resolveErr
			}
			req = *reqPtr
		}
		data, err = scoreCtrl.RenderScoreControl(req)
	case "score-custom-room":
		req := drawing.CustomRoomScoreRequest{}
		mergeParams(rc.Cmd.Params, &req)
		if req.TargetPoint <= 0 || len(req.CandidatePairs) == 0 {
			reqPtr, resolveErr := requestbuilder.BuildCustomRoomScoreRequest(toRequestBuilderCommandInput(rc.Cmd), rc.App)
			if resolveErr != nil {
				return nil, resolveErr
			}
			req = *reqPtr
		}
		data, err = scoreCtrl.RenderCustomRoomScore(req)
	case "score-music-meta":
		var params struct {
			Queries []string `json:"queries"`
		}
		if rc.Cmd.Params != nil {
			if err := json.Unmarshal(rc.Cmd.Params, &params); err != nil {
				return nil, fmt.Errorf("bridge: unmarshal music-meta params: %w", err)
			}
		}
		if len(params.Queries) == 0 {
			params.Queries = splitScoreMusicMetaQueries(rc.Cmd.Query)
		}
		req, resolveErr := musicCtrl.ResolveMusicMetaRequests(rc.Cmd.Region, params.Queries)
		if resolveErr != nil {
			return nil, resolveErr
		}
		data, err = scoreCtrl.RenderMusicMeta(req)
	case "score-music-board":
		req := drawing.MusicBoardRequest{}
		mergeParams(rc.Cmd.Params, &req)
		if len(req.Items) == 0 {
			if rc.App == nil || rc.App.Music == nil {
				return nil, fmt.Errorf("music board service unavailable: music controller is not configured")
			}
			boardQuery := rendermusic.BoardQuery{}
			mergeParams(rc.Cmd.Params, &boardQuery)
			if len(rc.Cmd.Params) == 0 && len(boardQuery.SpecQueries) == 0 {
				boardQuery.SpecQueries = splitScoreMusicMetaQueries(rc.Cmd.Query)
			}
			reqPtr, resolveErr := musicCtrl.ResolveMusicBoardRequest(rc.Cmd.Region, boardQuery)
			if resolveErr != nil {
				return nil, resolveErr
			}
			req = *reqPtr
		}
		data, err = scoreCtrl.RenderMusicBoard(req)
	default:
		return nil, unsupportedModeError("score", rc.Cmd.Mode)
	}
	if err != nil {
		return nil, err
	}
	return imageMessage(rc.Ctx, data, rc.App, BotModulePJSK)
}

func splitScoreMusicMetaQueries(args string) []string {
	segments := strings.Split(strings.ReplaceAll(strings.TrimSpace(args), "/", "|"), "|")
	clean := make([]string, 0, len(segments))
	for _, seg := range segments {
		seg = strings.TrimSpace(seg)
		if seg != "" {
			clean = append(clean, seg)
		}
	}
	return clean
}
