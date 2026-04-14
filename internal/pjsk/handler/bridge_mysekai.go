package handler

import (
	"context"
	"fmt"
	"strings"

	"haruki-cloud/api/bot/onebot11"
	harukiConfig "haruki-cloud/config"
	"haruki-cloud/internal/pjsk/render/mysekai"
	sekaiutils "haruki-cloud/utils/sekai"

	"golang.org/x/sync/errgroup"
)

type concurrentMessageJob func(context.Context) (onebot11.Message, error)

func executeConcurrentMessages(ctx context.Context, jobs ...concurrentMessageJob) (onebot11.Message, error) {
	if len(jobs) == 0 {
		return nil, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	group, groupCtx := errgroup.WithContext(ctx)
	messages := make([]onebot11.Message, len(jobs))
	for i, job := range jobs {
		i, job := i, job
		group.Go(func() error {
			message, err := job(groupCtx)
			if err != nil {
				return err
			}
			messages[i] = message
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}

	combined := make(onebot11.Message, 0, len(messages))
	for _, message := range messages {
		combined = append(combined, message...)
	}
	return combined, nil
}

func executeMysekai(rc *RequestContext) (message onebot11.Message, err error) {
	if rc.App == nil || rc.App.MySekai == nil {
		return nil, fmt.Errorf("mysekai service unavailable: mysekai controller is not configured")
	}

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

	renderCtx, err := resolveMySekaiRenderContext(rc.Ctx, rc.App, p, regionStr, rc.Cmd.RegionExplicit)
	if err != nil {
		return nil, err
	}

	var data []byte
	switch rc.Cmd.Mode {
	case "mysekai-resource":
		q := mysekai.ResourceQuery{Region: renderCtx.Region}
		mergeParams(rc.Cmd.Params, &q)
		q.Profile = renderCtx.Profile
		data, err = renderCtx.Controller.RenderResource(q)
	case "mysekai-resource-map":
		resourceQuery := mysekai.ResourceQuery{Region: renderCtx.Region}
		mergeParams(rc.Cmd.Params, &resourceQuery)
		resourceQuery.Profile = renderCtx.Profile
		mapQuery := mysekai.MapQuery{Region: renderCtx.Region}
		mergeParams(rc.Cmd.Params, &mapQuery)
		return executeConcurrentMessages(
			rc.Ctx,
			func(ctx context.Context) (onebot11.Message, error) {
				resourceData, resourceErr := renderCtx.Controller.RenderResource(resourceQuery)
				if resourceErr != nil {
					return nil, resourceErr
				}
				return imageMessage(ctx, resourceData, rc.App, BotModulePJSK)
			},
			func(ctx context.Context) (onebot11.Message, error) {
				mapData, mapErr := renderCtx.Controller.RenderMap(mapQuery)
				if mapErr != nil {
					return nil, mapErr
				}
				return imageMessage(ctx, mapData, rc.App, BotModulePJSK)
			},
		)
	case "mysekai-map":
		q := mysekai.MapQuery{Region: renderCtx.Region}
		mergeParams(rc.Cmd.Params, &q)
		data, err = renderCtx.Controller.RenderMap(q)
		replayMessage, err := imageMessage(rc.Ctx, data, rc.App, BotModulePJSK)
		if err != nil {
			return nil, err
		} else {
			replayMessage = append(replayMessage, onebot11.At(rc.PlatformUserID))
			return replayMessage, nil
		}
	case "mysekai-fixture-list":
		q := mysekai.FixtureListQuery{Region: renderCtx.Region}
		mergeParams(rc.Cmd.Params, &q)
		q.Profile = renderCtx.Profile
		data, err = renderCtx.Controller.RenderFixtureList(q)
	case "mysekai-fixture-detail":
		q := mysekai.FixtureDetailQuery{Region: renderCtx.Region, Query: rc.Cmd.Query}
		mergeParams(rc.Cmd.Params, &q)
		data, err = renderCtx.Controller.RenderFixtureDetail(q)
	case "mysekai-door-upgrade":
		q := mysekai.DoorUpgradeQuery{Region: renderCtx.Region, Query: rc.Cmd.Query}
		mergeParams(rc.Cmd.Params, &q)
		q.Profile = renderCtx.Profile
		data, err = renderCtx.Controller.RenderDoorUpgrade(q)
	case "mysekai-music-record":
		q := mysekai.MusicRecordQuery{Region: renderCtx.Region}
		mergeParams(rc.Cmd.Params, &q)
		q.Profile = renderCtx.Profile
		data, err = renderCtx.Controller.RenderMusicRecord(q)
	case "mysekai-photo":
		q := mysekai.PhotoQuery{Region: renderCtx.Region}
		mergeParams(rc.Cmd.Params, &q)
		result, resolveErr := renderCtx.Controller.ResolvePhoto(q)
		if resolveErr != nil {
			return nil, resolveErr
		}
		data, err = sekaiutils.GetSekaiAPIClient().GetMySekaiImage(result.Region, result.ImagePath)
		if err != nil {
			return nil, fmt.Errorf("获取 MySekai 照片失败：%w", err)
		}
		image, imageErr := imageMessage(rc.Ctx, data, rc.App, BotModulePJSK)
		if imageErr != nil {
			return nil, imageErr
		}
		photoTime := "未知"
		if !result.ObtainedAt.IsZero() {
			photoTime = result.ObtainedAt.Format("2006-01-02 15:04")
		}
		return append(image, onebot11.Text(fmt.Sprintf("拍摄时间: %s", photoTime))), nil
	case "mysekai-talk-list":
		q := mysekai.TalkListQuery{Region: renderCtx.Region, Query: rc.Cmd.Query}
		mergeParams(rc.Cmd.Params, &q)
		q.Profile = renderCtx.Profile
		data, err = renderCtx.Controller.RenderTalkList(q)
	default:
		return nil, unsupportedModeError("mysekai", rc.Cmd.Mode)
	}
	if err != nil {
		return nil, err
	}
	return imageMessage(rc.Ctx, data, rc.App, BotModulePJSK)
}
