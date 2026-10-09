package handler

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"haruki-cloud/internal/i18n"
	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/region"
	rendermusic "haruki-cloud/internal/pjsk/render/music"
	renderscore "haruki-cloud/internal/pjsk/render/score"
	"haruki-cloud/internal/pjsk/requestbuilder"
	"haruki-cloud/utils/usererror"
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
		Path: "score",
		Commands: []string{
			"/分数", "/查分数", "/pjsk score", "/score control",
			"/控分",
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
		return scoreControlParams{}, usererror.Misuse(i18n.M("score.control.usage"))
	}

	targetPT, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || targetPT <= 0 {
		return scoreControlParams{}, usererror.Misuse(i18n.M("score.control.usage"))
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
		Path: "score/custom-room",
		Commands: []string{
			"/pjsk custom room score", "/custom room score",
			"/自定义房间控分", "/自定义房控分", "/自定义控分",
			"/自定义房间分数", "/自定义分数",
		},
		Regions: []renderregion.Value{renderregion.JP},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			targetPT, err := strconv.Atoi(args)
			if err != nil || targetPT <= 0 {
				return nil, usererror.Misuse(i18n.M("score.custom_room.usage"))
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleScore, "score-custom-room", customRoomScoreParams{
				TargetPoint: targetPT,
			}), nil
		},
	}, executeScore)
}

func (sekaiHandlers) MusicMetaHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path: "score/music-meta",
		Commands: []string{
			"/pjsk music meta", "/music meta",
			"/歌曲meta", "/曲目meta",
		},
		Priority: 1,
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			args := strings.TrimSpace(ctx.GetArgs())
			clean := splitMusicMetaQueries(args)
			if len(clean) == 0 {
				return nil, usererror.Misuse(i18n.M("score.music_meta.required"))
			}
			if len(clean) > 3 {
				return nil, usererror.Invalid(i18n.M("score.music_meta.too_many", i18n.Data{"Max": 3}))
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
		Path: "score/music-board",
		Commands: []string{
			"/pjsk music board", "/music board",
			"/歌曲排行", "/歌曲比较", "/歌曲对比", "/歌曲排名", "/曲目榜",
		},
		Priority: 1,
		Regions:  []renderregion.Value{renderregion.JP},
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
	scoreCtrl, musicCtrl := scoreControllers(rc)
	data, err := executeScoreMode(rc, scoreCtrl, musicCtrl)
	if err != nil {
		return nil, err
	}
	return rc.RenderedImageMessage(data)
}

func scoreControllers(rc *RequestContext) (*renderscore.Controller, *rendermusic.Controller) {
	scoreCtrl := rc.App.Score
	if scoreCtrl != nil {
		scoreCtrl = scoreCtrl.WithContext(rc.Ctx)
	}
	var musicCtrl *rendermusic.Controller
	if rc.App != nil && rc.App.Music != nil {
		musicCtrl = rc.App.Music.WithContext(rc.Ctx)
		if rc.App.Aliases != nil {
			musicCtrl.SetAliasResolver(rc.App.Aliases)
		}
	}
	return scoreCtrl, musicCtrl
}

func executeScoreMode(rc *RequestContext, scoreCtrl *renderscore.Controller, musicCtrl *rendermusic.Controller) (drawing.ImageResult, error) {
	switch rc.Cmd.Mode {
	case "score-control":
		return executeScoreControl(rc, scoreCtrl)
	case "score-custom-room":
		return executeScoreCustomRoom(rc, scoreCtrl)
	case "score-music-meta":
		return executeScoreMusicMeta(rc, scoreCtrl, musicCtrl)
	case "score-music-board":
		return executeScoreMusicBoard(rc, scoreCtrl, musicCtrl)
	default:
		return drawing.ImageResult{}, unsupportedModeError("score", rc.Cmd.Mode)
	}
}

func executeScoreControl(rc *RequestContext, scoreCtrl *renderscore.Controller) (drawing.ImageResult, error) {
	finishBuild := measurePayloadBuild(rc.Ctx)
	defer finishBuild()
	req := drawing.ScoreControlRequest{}
	mergeParams(rc.Cmd.Params, &req)
	if req.MusicID <= 0 || req.TargetPoint <= 0 || len(req.ValidScores) == 0 {
		reqPtr, err := requestbuilder.BuildScoreControlRequest(rc.Ctx, toRequestBuilderCommandInput(rc.Cmd), rc.App)
		if err != nil {
			return drawing.ImageResult{}, err
		}
		req = *reqPtr
	}
	finishBuild()
	return scoreCtrl.RenderScoreControlImage(req)
}

func executeScoreCustomRoom(rc *RequestContext, scoreCtrl *renderscore.Controller) (drawing.ImageResult, error) {
	finishBuild := measurePayloadBuild(rc.Ctx)
	defer finishBuild()
	req := drawing.CustomRoomScoreRequest{}
	mergeParams(rc.Cmd.Params, &req)
	if req.TargetPoint <= 0 || len(req.CandidatePairs) == 0 {
		reqPtr, err := requestbuilder.BuildCustomRoomScoreRequest(toRequestBuilderCommandInput(rc.Cmd), rc.App)
		if err != nil {
			return drawing.ImageResult{}, err
		}
		req = *reqPtr
	}
	finishBuild()
	return scoreCtrl.RenderCustomRoomScoreImage(req)
}

func executeScoreMusicMeta(rc *RequestContext, scoreCtrl *renderscore.Controller, musicCtrl *rendermusic.Controller) (drawing.ImageResult, error) {
	finishBuild := measurePayloadBuild(rc.Ctx)
	defer finishBuild()
	var params struct {
		Queries []string `json:"queries"`
	}
	if rc.Cmd.Params != nil {
		if err := json.Unmarshal(rc.Cmd.Params, &params); err != nil {
			return drawing.ImageResult{}, fmt.Errorf("bridge: unmarshal music-meta params: %w", err)
		}
	}
	if len(params.Queries) == 0 {
		params.Queries = splitScoreMusicMetaQueries(rc.Cmd.Query)
	}
	req, err := musicCtrl.ResolveMusicMetaRequests(rc.Cmd.Region, params.Queries)
	if err != nil {
		return drawing.ImageResult{}, err
	}
	finishBuild()
	return scoreCtrl.RenderMusicMetaImage(req)
}

func executeScoreMusicBoard(rc *RequestContext, scoreCtrl *renderscore.Controller, musicCtrl *rendermusic.Controller) (drawing.ImageResult, error) {
	finishBuild := measurePayloadBuild(rc.Ctx)
	defer finishBuild()
	req := drawing.MusicBoardRequest{}
	mergeParams(rc.Cmd.Params, &req)
	if len(req.Items) == 0 {
		resolved, err := resolveScoreMusicBoardRequest(rc, musicCtrl)
		if err != nil {
			return drawing.ImageResult{}, err
		}
		req = *resolved
	}
	finishBuild()
	return scoreCtrl.RenderMusicBoardImage(req)
}

func resolveScoreMusicBoardRequest(rc *RequestContext, musicCtrl *rendermusic.Controller) (*drawing.MusicBoardRequest, error) {
	if rc.App == nil || rc.App.Music == nil {
		return nil, usererror.Misconfigured(errors.New("music board service unavailable: music controller is not configured"))
	}
	boardQuery := rendermusic.BoardQuery{}
	mergeParams(rc.Cmd.Params, &boardQuery)
	if len(rc.Cmd.Params) == 0 && len(boardQuery.SpecQueries) == 0 {
		boardQuery.SpecQueries = splitScoreMusicMetaQueries(rc.Cmd.Query)
	}
	return musicCtrl.ResolveMusicBoardRequest(rc.Cmd.Region, boardQuery)
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
