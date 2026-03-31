package handler

import (
	"encoding/json"
	"fmt"
	"strings"

	"haruki-cloud/api/bot/onebot11"
	"haruki-cloud/internal/pjsk/render/music"
	"haruki-cloud/internal/pjsk/requestbuilder"
	"haruki-cloud/utils/drawing"
)

func executeScore(rc *RequestContext) (message onebot11.Message, err error) {
	if rc.App != nil && rc.App.Music != nil && rc.App.Aliases != nil {
		rc.App.Music.SetAliasResolver(rc.App.Aliases)
	}
	var data []byte
	switch rc.Cmd.Mode {
	case "score-control":
		req := drawing.ScoreControlRequest{}
		mergeParams(rc.Cmd.Params, &req)
		if req.MusicID <= 0 || req.TargetPoint <= 0 || len(req.ValidScores) == 0 {
			reqPtr, resolveErr := requestbuilder.BuildScoreControlRequest(rc.Cmd, rc.App)
			if resolveErr != nil {
				return nil, resolveErr
			}
			req = *reqPtr
		}
		data, err = rc.App.Score.RenderScoreControl(req)
	case "score-custom-room":
		req := drawing.CustomRoomScoreRequest{}
		mergeParams(rc.Cmd.Params, &req)
		if req.TargetPoint <= 0 || len(req.CandidatePairs) == 0 {
			reqPtr, resolveErr := requestbuilder.BuildCustomRoomScoreRequest(rc.Cmd, rc.App)
			if resolveErr != nil {
				return nil, resolveErr
			}
			req = *reqPtr
		}
		data, err = rc.App.Score.RenderCustomRoomScore(req)
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
		req, resolveErr := rc.App.Music.ResolveMusicMetaRequests(rc.Cmd.Region, params.Queries)
		if resolveErr != nil {
			return nil, resolveErr
		}
		data, err = rc.App.Score.RenderMusicMeta(req)
	case "score-music-board":
		req := drawing.MusicBoardRequest{}
		mergeParams(rc.Cmd.Params, &req)
		if len(req.Items) == 0 {
			if rc.App == nil || rc.App.Music == nil {
				return nil, fmt.Errorf("music board service unavailable: music controller is not configured")
			}
			boardQuery := music.BoardQuery{}
			mergeParams(rc.Cmd.Params, &boardQuery)
			if len(boardQuery.SpecQueries) == 0 {
				boardQuery.SpecQueries = splitScoreMusicMetaQueries(rc.Cmd.Query)
			}
			reqPtr, resolveErr := rc.App.Music.ResolveMusicBoardRequest(rc.Cmd.Region, boardQuery)
			if resolveErr != nil {
				return nil, resolveErr
			}
			req = *reqPtr
		}
		data, err = rc.App.Score.RenderMusicBoard(req)
	default:
		return nil, fmt.Errorf("bridge: unsupported score mode %q", rc.Cmd.Mode)
	}
	if err != nil {
		return nil, err
	}
	return imageMessage(data, rc.App, BotModulePJSK)
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
