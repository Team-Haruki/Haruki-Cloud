package handler

import (
	"strings"

	"haruki-cloud/internal/pjsk/render/sk"
	"haruki-cloud/internal/pjsk/drawing"
)

func executeSKMode(rc *RequestContext, skCtrl *sk.Controller) ([]byte, error) {
	switch rc.Cmd.Mode {
	case "sk-line":
		return executeSKLine(rc, skCtrl)
	case "sk-query":
		return executeSKQuery(rc, skCtrl)
	case "sk-check-room":
		return executeSKCheckRoom(rc, skCtrl)
	case "sk-speed", "sk-daily-speed":
		return executeSKSpeed(rc, skCtrl)
	case "sk-player-trace":
		return executeSKPlayerTrace(rc, skCtrl)
	case "sk-rank-trace":
		return executeSKRankTrace(rc, skCtrl)
	case "sk-predict":
		return executeSKPredict(rc, skCtrl)
	case "sk-winrate":
		return executeSKWinRate(rc, skCtrl)
	default:
		return nil, unsupportedModeError("sk", rc.Cmd.Mode)
	}
}

func executeSKLine(rc *RequestContext, skCtrl *sk.Controller) ([]byte, error) {
	if trackerReq, ok := trackerRankQueryFromParams(rc.Cmd); ok {
		if err := prepareTrackerRankQuery(rc.Ctx, rc.App, &trackerReq, rc.Cmd.RequesterPlatform, rc.Cmd.RequesterUserID); err != nil {
			return nil, err
		}
		payload, err := skCtrl.BuildLineRequestFromTracker(trackerReq)
		if err != nil {
			return nil, err
		}
		return skCtrl.RenderLine(*payload)
	}
	req := sk.LineRequest{}
	mergeParams(rc.Cmd.Params, &req)
	return skCtrl.RenderLine(req)
}

func executeSKQuery(rc *RequestContext, skCtrl *sk.Controller) ([]byte, error) {
	if trackerReq, ok := trackerRankQueryFromParams(rc.Cmd); ok {
		if err := prepareTrackerRankQuery(rc.Ctx, rc.App, &trackerReq, rc.Cmd.RequesterPlatform, rc.Cmd.RequesterUserID); err != nil {
			return nil, err
		}
		payload, err := skCtrl.BuildQueryRequestFromTracker(trackerReq)
		if err != nil {
			return nil, err
		}
		return skCtrl.RenderQuery(*payload)
	}
	req := drawing.SKRequest{}
	mergeParams(rc.Cmd.Params, &req)
	return skCtrl.RenderQuery(req)
}

func executeSKCheckRoom(rc *RequestContext, skCtrl *sk.Controller) ([]byte, error) {
	if trackerReq, ok := trackerRankQueryFromParams(rc.Cmd); ok {
		if err := prepareTrackerRankQuery(rc.Ctx, rc.App, &trackerReq, rc.Cmd.RequesterPlatform, rc.Cmd.RequesterUserID); err != nil {
			return nil, err
		}
		payload, err := skCtrl.BuildCheckRoomRequestFromTracker(trackerReq)
		if err != nil {
			return nil, err
		}
		return skCtrl.RenderCheckRoom(*payload)
	}
	req := drawing.CFRequest{}
	mergeParams(rc.Cmd.Params, &req)
	return skCtrl.RenderCheckRoom(req)
}

func executeSKSpeed(rc *RequestContext, skCtrl *sk.Controller) ([]byte, error) {
	if trackerReq, ok := trackerRankQueryFromParams(rc.Cmd); ok {
		if err := prepareTrackerRankQuery(rc.Ctx, rc.App, &trackerReq, rc.Cmd.RequesterPlatform, rc.Cmd.RequesterUserID); err != nil {
			return nil, err
		}
		payload, err := skCtrl.BuildSpeedRequestFromTracker(trackerReq)
		if err != nil {
			return nil, err
		}
		return skCtrl.RenderSpeed(*payload)
	}
	req := drawing.SpeedRequest{}
	mergeParams(rc.Cmd.Params, &req)
	return skCtrl.RenderSpeed(req)
}

func executeSKPlayerTrace(rc *RequestContext, skCtrl *sk.Controller) ([]byte, error) {
	trackerReq, ok := trackerRankQueryFromParams(rc.Cmd)
	if !ok {
		// No params: build a basic query for the requester's own user.
		trackerReq = sk.TrackerRankQuery{Region: rc.Cmd.Region}
		if trackerReq.Region == "" {
			trackerReq.Region = DefaultRegionStr
		}
	}
	if err := resolveTrackerCharacterSelection(rc.Ctx, rc.App, &trackerReq); err != nil {
		return nil, err
	}
	hasExplicitTarget := strings.TrimSpace(trackerReq.TargetUserID) != ""
	if trackerReq.UserID == nil {
		targetErr := resolveTrackerTargetUser(rc.Ctx, rc.App, &trackerReq, rc.Cmd.RequesterPlatform, rc.Cmd.RequesterUserID)
		if targetErr != nil && hasExplicitTarget {
			return nil, targetErr
		}
		// Fallback to requester's own UID only when command does not explicitly target user/rank.
		if trackerReq.UserID == nil && len(trackerReq.Ranks) == 0 && !hasExplicitTarget {
			if uid := resolveRequesterGameUID(rc); uid > 0 {
				trackerReq.UserID = &uid
			}
		}
	}
	if trackerReq.UserID != nil || len(trackerReq.Ranks) > 0 {
		payload, err := skCtrl.BuildPlayerTraceFromTracker(trackerReq)
		if err != nil {
			return nil, err
		}
		return skCtrl.RenderPlayerTrace(*payload)
	}
	req := drawing.PlayerTraceRequest{}
	mergeParams(rc.Cmd.Params, &req)
	return skCtrl.RenderPlayerTrace(req)
}

func executeSKRankTrace(rc *RequestContext, skCtrl *sk.Controller) ([]byte, error) {
	if trackerReq, ok := trackerRankQueryFromParams(rc.Cmd); ok {
		if err := prepareTrackerRankQuery(rc.Ctx, rc.App, &trackerReq, rc.Cmd.RequesterPlatform, rc.Cmd.RequesterUserID); err != nil {
			return nil, err
		}
		payload, err := skCtrl.BuildRankTraceRequestFromTracker(trackerReq)
		if err != nil {
			return nil, err
		}
		return skCtrl.RenderRankTrace(*payload)
	}
	req := drawing.RankTraceRequest{}
	mergeParams(rc.Cmd.Params, &req)
	return skCtrl.RenderRankTrace(req)
}

func executeSKPredict(rc *RequestContext, skCtrl *sk.Controller) ([]byte, error) {
	if trackerReq, ok := trackerRankQueryFromParams(rc.Cmd); ok {
		if err := prepareTrackerRankQuery(rc.Ctx, rc.App, &trackerReq, rc.Cmd.RequesterPlatform, rc.Cmd.RequesterUserID); err != nil {
			return nil, err
		}
		payload, err := skCtrl.BuildPredictLineRequestFromTracker(trackerReq)
		if err != nil {
			return nil, err
		}
		return skCtrl.RenderLine(*payload)
	}
	req := sk.LineRequest{}
	mergeParams(rc.Cmd.Params, &req)
	return skCtrl.RenderLine(req)
}

func executeSKWinRate(rc *RequestContext, skCtrl *sk.Controller) ([]byte, error) {
	req := drawing.WinRateRequest{}
	mergeParams(rc.Cmd.Params, &req)
	return skCtrl.RenderWinRate(req)
}
