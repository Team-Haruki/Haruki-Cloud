package handler

import (
	"strings"

	"haruki-cloud/api/bot/onebot11"
	"haruki-cloud/internal/pjsk/render/education"
	"haruki-cloud/internal/pjsk/render/userdata"
	accountdata "haruki-cloud/internal/pjsk/userdata"
	"haruki-cloud/utils/drawing"
)

func executeEducation(rc *RequestContext) (message onebot11.Message, err error) {
	if rc.App == nil || rc.App.Edu == nil {
		return nil, unsupportedModeError("education", rc.Cmd.Mode)
	}
	eduCtrl := rc.App.Edu.WithContext(rc.Ctx)
	var data []byte
	region := rc.Region
	regionStr := rc.RegionStr
	publicDetailedProfile := rc.GetDetailedProfile()

	// Resolve the user's suite-visible binding and its request-scoped snapshot.
	platform := rc.Platform
	platformUserID := rc.PlatformUserID
	var suitePJSKUserID string
	var suitePlatform, suitePlatformUserID string
	var suiteBinding *accountdata.ResolvedBinding
	var suiteSnapshot userdata.Snapshot

	_, binding, _ := resolveBindingWithFallback(
		rc.Ctx, rc.App.Bindings, platform, platformUserID, regionStr, rc.Cmd.RegionExplicit,
		bindingResolutionOptions{RequireSuite: true},
	)
	if binding != nil {
		suitePJSKUserID = binding.PJSKUserID
		suitePlatform = platform
		suitePlatformUserID = platformUserID
		suiteBinding = binding
		suiteSnapshot = resolveTargetSnapshot(rc.Ctx, rc.App, regionStr, suitePlatform, suitePlatformUserID, suitePJSKUserID, false)
	}

	switch rc.Cmd.Mode {
	case "education-challenge":
		q := education.ChallengeLiveQuery{Region: region}
		mergeParams(rc.Cmd.Params, &q)
		q.Profile = publicDetailedProfile
		if suiteSnapshot != nil {
			q.Snapshot = suiteSnapshot
		}
		data, err = eduCtrl.RenderChallengeLiveDetails(q)

	case "education-bonds":
		query := education.BondsQuery{Region: region}
		mergeParams(rc.Cmd.Params, &query)
		if query.Cid <= 0 && strings.TrimSpace(query.CharacterQuery) != "" {
			query.Cid, err = resolveEducationBondsCharacterID(rc.Ctx, rc.App, region, query.CharacterQuery)
			if err != nil {
				return nil, err
			}
		}

		req := drawing.BondsRequest{}
		mergeParams(rc.Cmd.Params, &req)
		if len(req.Bonds) == 0 && suiteSnapshot != nil {
			query.Profile = publicDetailedProfile
			query.Snapshot = suiteSnapshot
			bondsReq, buildErr := eduCtrl.BuildBondsRequestFromSnapshot(query)
			if buildErr == nil {
				req = *bondsReq
			}
		}
		data, err = eduCtrl.RenderBonds(req)

	case "education-leader":
		req := drawing.LeaderCountRequest{}
		mergeParams(rc.Cmd.Params, &req)
		if len(req.LeaderCounts) == 0 && suiteSnapshot != nil {
			leaderReq, buildErr := eduCtrl.BuildLeaderCountRequestFromSnapshot(education.LeaderCountQuery{
				Region:   region,
				Profile:  publicDetailedProfile,
				Snapshot: suiteSnapshot,
			})
			if buildErr == nil {
				req = *leaderReq
			}
		}
		data, err = eduCtrl.RenderLeaderCount(req)

	case "education-power":
		req := drawing.PowerBonusDetailRequest{}
		mergeParams(rc.Cmd.Params, &req)
		if len(req.CharaBonuses) == 0 && len(req.UnitBonuses) == 0 && len(req.AttrBonuses) == 0 {
			if snapshot := resolveTargetSnapshot(rc.Ctx, rc.App, regionStr, suitePlatform, suitePlatformUserID, suitePJSKUserID, hasUsableMySekaiData(suiteBinding)); snapshot != nil {
				builtReq, buildErr := eduCtrl.BuildPowerBonusDetailRequestFromSnapshot(education.PowerBonusQuery{
					Region:   region,
					Profile:  publicDetailedProfile,
					Snapshot: snapshot,
				})
				if buildErr != nil {
					return nil, buildErr
				}
				req = *builtReq
			}
		}
		data, err = eduCtrl.RenderPowerBonusDetail(req)

	case "education-area":
		query := education.AreaItemQuery{Region: region}
		mergeParams(rc.Cmd.Params, &query)
		if query.Cid <= 0 && strings.TrimSpace(query.CharacterQuery) != "" {
			query.Cid, err = resolveEducationAreaCharacterID(rc.Ctx, rc.App, region, query.CharacterQuery)
			if err != nil {
				return nil, err
			}
		}
		query.Profile = publicDetailedProfile
		if suiteSnapshot != nil {
			query.Snapshot = suiteSnapshot
			builtReq, buildErr := eduCtrl.BuildAreaItemUpgradeMaterialsRequestFromSnapshot(query)
			if buildErr != nil {
				return nil, buildErr
			}
			data, err = eduCtrl.RenderAreaItemUpgradeMaterials(*builtReq)
			break
		}
		data, err = eduCtrl.RenderAreaItemUpgradeMaterials(drawing.AreaItemUpgradeMaterialsRequest{})

	default:
		return nil, unsupportedModeError("education", rc.Cmd.Mode)
	}
	if err != nil {
		return nil, err
	}
	return rc.ImageMessage(data)
}
