package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"haruki-cloud/api/bot/onebot11"
	"haruki-cloud/internal/pjsk/parser"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/internal/pjsk/render/sk"
	accountdata "haruki-cloud/internal/pjsk/userdata"
	"haruki-cloud/utils/drawing"
)

func executeSK(rc *RequestContext) (message onebot11.Message, err error) {
	var data []byte
	switch rc.Cmd.Mode {
	case "sk-line":
		if trackerReq, ok := trackerRankQueryFromParams(rc.Cmd); ok {
			if err := prepareTrackerRankQuery(rc.Ctx, rc.App, &trackerReq, rc.Cmd.RequesterPlatform, rc.Cmd.RequesterUserID); err != nil {
				return nil, err
			}
			payload, err := rc.App.SK.BuildLineRequestFromTracker(trackerReq)
			if err != nil {
				return nil, err
			}
			data, err = rc.App.SK.RenderLine(*payload)
			break
		}
		req := sk.LineRequest{}
		mergeParams(rc.Cmd.Params, &req)
		data, err = rc.App.SK.RenderLine(req)
	case "sk-query":
		if trackerReq, ok := trackerRankQueryFromParams(rc.Cmd); ok {
			if err := prepareTrackerRankQuery(rc.Ctx, rc.App, &trackerReq, rc.Cmd.RequesterPlatform, rc.Cmd.RequesterUserID); err != nil {
				return nil, err
			}
			payload, err := rc.App.SK.BuildQueryRequestFromTracker(trackerReq)
			if err != nil {
				return nil, err
			}
			data, err = rc.App.SK.RenderQuery(*payload)
			break
		}
		req := drawing.SKRequest{}
		mergeParams(rc.Cmd.Params, &req)
		data, err = rc.App.SK.RenderQuery(req)
	case "sk-check-room":
		if trackerReq, ok := trackerRankQueryFromParams(rc.Cmd); ok {
			if err := prepareTrackerRankQuery(rc.Ctx, rc.App, &trackerReq, rc.Cmd.RequesterPlatform, rc.Cmd.RequesterUserID); err != nil {
				return nil, err
			}
			payload, err := rc.App.SK.BuildCheckRoomRequestFromTracker(trackerReq)
			if err != nil {
				return nil, err
			}
			data, err = rc.App.SK.RenderCheckRoom(*payload)
			break
		}
		req := drawing.CFRequest{}
		mergeParams(rc.Cmd.Params, &req)
		data, err = rc.App.SK.RenderCheckRoom(req)
	case "sk-speed":
		if trackerReq, ok := trackerRankQueryFromParams(rc.Cmd); ok {
			if err := prepareTrackerRankQuery(rc.Ctx, rc.App, &trackerReq, rc.Cmd.RequesterPlatform, rc.Cmd.RequesterUserID); err != nil {
				return nil, err
			}
			payload, err := rc.App.SK.BuildSpeedRequestFromTracker(trackerReq)
			if err != nil {
				return nil, err
			}
			data, err = rc.App.SK.RenderSpeed(*payload)
			break
		}
		req := drawing.SpeedRequest{}
		mergeParams(rc.Cmd.Params, &req)
		data, err = rc.App.SK.RenderSpeed(req)
	case "sk-player-trace":
		trackerReq, ok := trackerRankQueryFromParams(rc.Cmd)
		if !ok {
			// No params: build a basic query for the requester's own user
			trackerReq = sk.TrackerRankQuery{Region: rc.Cmd.Region}
			if trackerReq.Region == "" {
				trackerReq.Region = "jp"
			}
		}
		if err := resolveTrackerCharacterSelection(rc.Ctx, rc.App, &trackerReq); err != nil {
			return nil, err
		}
		hasExplicitTarget := strings.TrimSpace(trackerReq.TargetUserID) != ""
		// Resolve user ID if not provided.
		if trackerReq.UserID == nil {
			targetErr := resolveTrackerTargetUser(rc.Ctx, rc.App, &trackerReq, rc.Cmd.RequesterPlatform, rc.Cmd.RequesterUserID)
			if targetErr != nil && hasExplicitTarget {
				return nil, targetErr
			}
			// Fallback to requester's own UID only when command does not explicitly target user/rank.
			if trackerReq.UserID == nil && len(trackerReq.Ranks) == 0 && !hasExplicitTarget {
				if uid := resolveRequesterGameUID(rc.Ctx, rc.Cmd, rc.App); uid > 0 {
					trackerReq.UserID = &uid
				}
			}
		}
		if trackerReq.UserID != nil || len(trackerReq.Ranks) > 0 {
			payload, buildErr := rc.App.SK.BuildPlayerTraceFromTracker(trackerReq)
			if buildErr != nil {
				return nil, buildErr
			}
			data, err = rc.App.SK.RenderPlayerTrace(*payload)
			break
		}
		req := drawing.PlayerTraceRequest{}
		mergeParams(rc.Cmd.Params, &req)
		data, err = rc.App.SK.RenderPlayerTrace(req)
	case "sk-rank-trace":
		if trackerReq, ok := trackerRankQueryFromParams(rc.Cmd); ok {
			if err := prepareTrackerRankQuery(rc.Ctx, rc.App, &trackerReq, rc.Cmd.RequesterPlatform, rc.Cmd.RequesterUserID); err != nil {
				return nil, err
			}
			payload, err := rc.App.SK.BuildRankTraceRequestFromTracker(trackerReq)
			if err != nil {
				return nil, err
			}
			data, err = rc.App.SK.RenderRankTrace(*payload)
			break
		}
		req := drawing.RankTraceRequest{}
		mergeParams(rc.Cmd.Params, &req)
		data, err = rc.App.SK.RenderRankTrace(req)
	case "sk-winrate":
		req := drawing.WinRateRequest{}
		mergeParams(rc.Cmd.Params, &req)
		data, err = rc.App.SK.RenderWinRate(req)
	default:
		return nil, fmt.Errorf("bridge: unsupported sk mode %q", rc.Cmd.Mode)
	}
	if err != nil {
		return nil, err
	}
	return imageMessage(data, rc.App, BotModulePJSK)
}

func trackerRankQueryFromParams(r *parser.ResolvedCommand) (sk.TrackerRankQuery, bool) {
	if r == nil || len(r.Params) == 0 {
		return sk.TrackerRankQuery{}, false
	}
	var req sk.TrackerRankQuery
	if err := json.Unmarshal(r.Params, &req); err != nil {
		return sk.TrackerRankQuery{}, false
	}
	resolvedRegion := strings.TrimSpace(r.Region)
	if !req.RegionExplicit && resolvedRegion != "" {
		req.Region = resolvedRegion
	} else if req.Region == "" {
		req.Region = resolvedRegion
	}
	if len(req.Ranks) == 0 &&
		req.EventID == 0 &&
		req.WlCharacterID == nil &&
		req.UserID == nil &&
		strings.TrimSpace(req.TargetUserID) == "" {
		return sk.TrackerRankQuery{}, false
	}
	return req, true
}

func prepareTrackerRankQuery(ctx context.Context, app *renderapp.App, req *sk.TrackerRankQuery, requesterPlatform, requesterUserID string) error {
	if err := resolveTrackerCharacterSelection(ctx, app, req); err != nil {
		return err
	}
	return resolveTrackerTargetUser(ctx, app, req, requesterPlatform, requesterUserID)
}

func resolveTrackerCharacterSelection(ctx context.Context, app *renderapp.App, req *sk.TrackerRankQuery) error {
	if req == nil || req.WlCharacterID != nil {
		return nil
	}
	query := strings.TrimSpace(req.WlCharacterQuery)
	if query == "" {
		return nil
	}
	region := renderregion.WithDefault(renderregion.Normalize(req.Region))
	charID, err := resolveGameCharacterIDByQuery(ctx, app, region, query, "sk")
	if err != nil {
		return err
	}
	req.WlCharacterID = drawing.IntPtr(charID)
	req.WlCharacterQuery = ""
	return nil
}

// resolveRequesterGameUID resolves the game user ID from the requester's binding.
func resolveRequesterGameUID(ctx context.Context, r *parser.ResolvedCommand, app *renderapp.App) int64 {
	platform := strings.TrimSpace(r.RequesterPlatform)
	platformUserID := strings.TrimSpace(r.RequesterUserID)
	if platform == "" || platformUserID == "" || app.Bindings == nil {
		return 0
	}
	regionStr := strings.TrimSpace(r.Region)
	if regionStr == "" {
		regionStr = "jp"
	}
	var binding *accountdata.ResolvedBinding
	var err error
	if !r.RegionExplicit {
		_, binding, err = app.Bindings.ResolveUserBinding(ctx, platform, platformUserID, accountdata.GlobalDefaultBindingScope)
		if err != nil || binding == nil {
			_, binding, err = app.Bindings.ResolveUserBinding(ctx, platform, platformUserID, regionStr)
		}
	} else {
		_, binding, err = app.Bindings.ResolveUserBinding(ctx, platform, platformUserID, regionStr)
	}
	if err != nil || binding == nil {
		return 0
	}
	uid, parseErr := strconv.ParseInt(strings.TrimSpace(binding.PJSKUserID), 10, 64)
	if parseErr != nil || uid <= 0 {
		return 0
	}
	return uid
}

func resolveTrackerTargetUser(ctx context.Context, app *renderapp.App, req *sk.TrackerRankQuery, requesterPlatform, requesterUserID string) error {
	if req == nil || req.UserID != nil {
		return nil
	}

	targetPlatform := strings.TrimSpace(req.TargetPlatform)
	targetUserID := strings.TrimSpace(req.TargetUserID)
	targetSelector := strings.TrimSpace(req.TargetSelector)
	if targetPlatform == "" || targetUserID == "" {
		return nil
	}

	if app == nil || app.Bindings == nil || !app.Bindings.IsReady() {
		return fmt.Errorf("绑定服务未就绪，无法解析目标用户")
	}

	var (
		binding *accountdata.ResolvedBinding
		err     error
	)

	// Selector mode: pick a specific bound account directly (u1/u2...).
	if targetSelector != "" {
		_, binding, err = app.Bindings.ResolveUserBindingBySelector(ctx, targetPlatform, targetUserID, targetSelector)
		if err != nil {
			return fmt.Errorf("无法解析账号选择器 %s: %w", targetSelector, err)
		}
	} else if req.RegionExplicit {
		region := normalizeTrackerRegion(req.Region)
		_, binding, err = app.Bindings.ResolveUserBinding(ctx, targetPlatform, targetUserID, region)
		if err != nil {
			return fmt.Errorf("@用户 %s 在 %s 服没有绑定账号", targetUserID, strings.ToUpper(region))
		}
	} else {
		// No explicit region prefix: use global default binding first, then JP.
		_, binding, err = app.Bindings.ResolveUserBinding(ctx, targetPlatform, targetUserID, accountdata.GlobalDefaultBindingScope)
		if err != nil || binding == nil {
			_, binding, err = app.Bindings.ResolveUserBinding(ctx, targetPlatform, targetUserID, string(renderregion.JP))
			if err != nil {
				return fmt.Errorf("@用户 %s 没有可用绑定", targetUserID)
			}
		}
	}

	if binding == nil {
		return fmt.Errorf("@用户 %s 没有可用绑定", targetUserID)
	}

	isSelfTarget := strings.EqualFold(strings.TrimSpace(targetPlatform), strings.TrimSpace(requesterPlatform)) &&
		strings.TrimSpace(targetUserID) != "" &&
		strings.TrimSpace(targetUserID) == strings.TrimSpace(requesterUserID)

	// @target should respect visible=false and deny query, except self query.
	if targetSelector == "" && !binding.Visible && !isSelfTarget {
		return fmt.Errorf("@用户 %s 已隐藏个人信息，无法查询", targetUserID)
	}

	uid, parseErr := strconv.ParseInt(strings.TrimSpace(binding.PJSKUserID), 10, 64)
	if parseErr != nil || uid <= 0 {
		return fmt.Errorf("@用户 %s 的绑定UID无效: %s", targetUserID, binding.PJSKUserID)
	}
	req.UserID = &uid
	if !req.RegionExplicit {
		req.Region = normalizeTrackerRegion(binding.Server)
	}
	return nil
}

func normalizeTrackerRegion(region string) string {
	normalized := renderregion.Normalize(strings.TrimSpace(region))
	if normalized.IsZero() {
		return string(renderregion.JP)
	}
	return normalized.String()
}
