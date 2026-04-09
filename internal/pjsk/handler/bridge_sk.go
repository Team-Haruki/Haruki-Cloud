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
	if rc == nil || rc.App == nil || rc.App.SK == nil {
		return nil, fmt.Errorf("sk service unavailable: tracker controller is not configured")
	}
	skCtrl := rc.App.SK.WithContext(rc.Ctx)
	data, err := executeSKMode(rc, skCtrl)
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
func resolveRequesterGameUID(rc *RequestContext) int64 {
	_, binding, _ := resolveBindingWithFallback(
		rc.Ctx, rc.App.Bindings, rc.Platform, rc.PlatformUserID, rc.RegionStr, rc.Cmd.RegionExplicit,
		bindingResolutionOptions{},
	)
	if binding == nil {
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
			_, binding, err = app.Bindings.ResolveUserBinding(ctx, targetPlatform, targetUserID, DefaultRegionStr)
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
	return regionWithDefault(region)
}
