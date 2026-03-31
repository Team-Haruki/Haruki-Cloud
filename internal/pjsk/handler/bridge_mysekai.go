package handler

import (
	"fmt"
	"strconv"
	"strings"

	"haruki-cloud/api/bot/onebot11"
	harukiConfig "haruki-cloud/config"
	"haruki-cloud/internal/pjsk/render/mysekai"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/internal/pjsk/render/userdata"
	"haruki-cloud/utils/drawing"
	sekaiutils "haruki-cloud/utils/sekai"
)

func executeMysekai(rc *RequestContext) (message onebot11.Message, err error) {
	// MySekai is disabled for CN region unless the requester's
	// platform+group is on the whitelist.
	if strings.EqualFold(rc.Cmd.Region, "cn") {
		allowed := false
		for _, entry := range harukiConfig.Cfg.PJSK.AllowCNMySekai {
			if strings.EqualFold(entry.Platform, rc.Cmd.RequesterPlatform) &&
				entry.GroupID == rc.Cmd.RequesterGroupID {
				allowed = true
				break
			}
		}
		if !allowed {
			return onebot11.Message{onebot11.Text("MySekai 功能在此区服暂未开放")}, nil
		}
	}

	// Resolve the target binding from params (supports u[i] selector and
	// region-specific vs global default bindings).
	var p userQueryParams
	mergeParams(rc.Cmd.Params, &p)
	if p.Mode == "" {
		p.Mode = "self"
		p.Platform = rc.Platform
		p.PlatformUserID = rc.PlatformUserID
	}

	regionStr := rc.RegionStr

	// When binding service is available, resolve target through it.
	// Otherwise fall back to old behavior (use local snapshot as-is).
	var target *resolvedGameTarget
	var uid int64
	var platform, platformUserID string

	if rc.App.Bindings != nil && p.Platform != "" && p.PlatformUserID != "" {
		t, targetErr := resolveGameTarget(rc.Ctx, p, regionStr, rc.Cmd.RegionExplicit, rc.App)
		if targetErr != nil {
			return nil, targetErr
		}
		target = &t
		uid, _ = strconv.ParseInt(target.PJSKUserID, 10, 64)
		platform, platformUserID = platformCredentials(p)
	}

	// Build public profile card for the resolved target.
	var publicProfileCard *drawing.ProfileCardRequest
	if target != nil {
		publicProfileCard = buildPublicProfileCardForTarget(*target, regionStr, platform, platformUserID, rc.App)
	}

	// Inject live Toolbox data. Prefer the full snapshot (suite + mysekai
	// merged); fall back to mysekai-only data which is sufficient for all
	// mysekai render modes (profile card comes from the public API override).
	msCtrl := rc.App.MySekai
	if target != nil {
		tc := sekaiutils.GetToolboxClient()

		if target.Binding != nil && hasUsableSuiteData(target.Binding) {
			suiteJSON, suiteErr := tc.GetSuiteData(regionStr, uid, platform, platformUserID)
			if suiteErr == nil && len(suiteJSON) > 0 {
				var mysekaiJSON []byte
				if hasUsableMySekaiData(target.Binding) {
					mysekaiJSON, _ = tc.GetMySekaiData(regionStr, uid, platform, platformUserID)
				}
				region := renderregion.Normalize(regionStr)
				if snapshot, snapErr := userdata.NewFromBytes(rc.App.Sekai, rc.App.Assets, region, suiteJSON, mysekaiJSON, nil); snapErr == nil {
					msCtrl = msCtrl.WithSnapshot(snapshot)
				}
			}
		}
		if msCtrl == rc.App.MySekai && target.Binding != nil && hasUsableMySekaiData(target.Binding) {
			if data, dataErr := tc.GetMySekaiData(regionStr, uid, platform, platformUserID); dataErr == nil && len(data) > 0 {
				msCtrl = msCtrl.WithMySekaiData(data)
			}
		}
	}

	var data []byte
	switch rc.Cmd.Mode {
	case "mysekai-resource":
		q := mysekai.ResourceQuery{Region: rc.Cmd.Region}
		mergeParams(rc.Cmd.Params, &q)
		q.Profile = publicProfileCard
		data, err = msCtrl.RenderResource(q)
	case "mysekai-map":
		q := mysekai.MapQuery{Region: rc.Cmd.Region}
		mergeParams(rc.Cmd.Params, &q)
		data, err = msCtrl.RenderMap(q)
	case "mysekai-fixture-list":
		q := mysekai.FixtureListQuery{Region: rc.Cmd.Region}
		mergeParams(rc.Cmd.Params, &q)
		q.Profile = publicProfileCard
		data, err = msCtrl.RenderFixtureList(q)
	case "mysekai-fixture-detail":
		q := mysekai.FixtureDetailQuery{Region: rc.Cmd.Region, Query: rc.Cmd.Query}
		mergeParams(rc.Cmd.Params, &q)
		data, err = msCtrl.RenderFixtureDetail(q)
	case "mysekai-door-upgrade":
		q := mysekai.DoorUpgradeQuery{Region: rc.Cmd.Region, Query: rc.Cmd.Query}
		mergeParams(rc.Cmd.Params, &q)
		q.Profile = publicProfileCard
		data, err = msCtrl.RenderDoorUpgrade(q)
	case "mysekai-music-record":
		q := mysekai.MusicRecordQuery{Region: rc.Cmd.Region}
		mergeParams(rc.Cmd.Params, &q)
		q.Profile = publicProfileCard
		data, err = msCtrl.RenderMusicRecord(q)
	case "mysekai-photo":
		q := mysekai.PhotoQuery{Region: rc.Cmd.Region}
		mergeParams(rc.Cmd.Params, &q)
		result, resolveErr := msCtrl.ResolvePhoto(q)
		if resolveErr != nil {
			return nil, resolveErr
		}
		data, err = sekaiutils.GetSekaiAPIClient().GetMySekaiImage(result.Region, result.ImagePath)
		if err != nil {
			return nil, fmt.Errorf("获取 MySekai 照片失败：%w", err)
		}
		image, imageErr := imageMessage(data, rc.App, BotModulePJSK)
		if imageErr != nil {
			return nil, imageErr
		}
		photoTime := "未知"
		if !result.ObtainedAt.IsZero() {
			photoTime = result.ObtainedAt.Format("2006-01-02 15:04")
		}
		return append(image, onebot11.Text(fmt.Sprintf("拍摄时间: %s", photoTime))), nil
	case "mysekai-talk-list":
		q := mysekai.TalkListQuery{Region: rc.Cmd.Region, Query: rc.Cmd.Query}
		mergeParams(rc.Cmd.Params, &q)
		q.Profile = publicProfileCard
		data, err = msCtrl.RenderTalkList(q)
	default:
		return nil, fmt.Errorf("bridge: unsupported mysekai mode %q", rc.Cmd.Mode)
	}
	if err != nil {
		return nil, err
	}
	return imageMessage(data, rc.App, BotModulePJSK)
}
