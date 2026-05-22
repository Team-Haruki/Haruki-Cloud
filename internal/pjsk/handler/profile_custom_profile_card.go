package handler

import (
	"fmt"
	"strconv"
	"strings"

	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/parser"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
)

const profileModeCustomProfileCard = "profile-custom-profile-card"

type profileCustomProfileCardParams struct {
	UserQueryParams
	CustomProfileID     int `json:"custom_profile_id,omitempty"`
	CustomProfileCardID int `json:"custom_profile_card_id,omitempty"`
	Seq                 int `json:"seq,omitempty"`
}

func (sekaiHandlers) ProfileCustomProfileCardHandle() HarukiSekaiCommandHandler {
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		ParseUIDArg: commandBoolPtr(true),
		CommandHandlerBase: CommandHandlerBase{
			Path: "profile/custom-profile-card",
			Commands: []string{
				"/自定义个人信息", "/自定义档案", "/pjsk custom profile", "/custom-profile",
			},
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			params, err := buildProfileCustomProfileCardParams(ctx)
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleProfile, profileModeCustomProfileCard, params), nil
		},
	}, executeProfile)
}

func buildProfileCustomProfileCardParams(ctx HarrukiSekaiHandlerContext) (profileCustomProfileCardParams, error) {
	query, err := resolveSelfOnlyQueryParams(ctx)
	if err != nil {
		return profileCustomProfileCardParams{}, err
	}

	params := profileCustomProfileCardParams{UserQueryParams: query, Seq: 1}
	args := strings.Fields(strings.TrimSpace(ctx.GetArgs()))
	if len(args) != 1 {
		return params, onebot11.NewReplayError("使用方式:\n%s 1\n%s 2\n%s u2 3\n数字为要渲染的自定义个人信息页序号，每次只渲染一张",
			ctx.originalTriggerCmd,
			ctx.originalTriggerCmd,
			ctx.originalTriggerCmd,
		)
	}

	seq, ok := parsePositiveIntArg(args[0])
	if !ok {
		return params, onebot11.NewReplayError("自定义个人信息页序号必须是正整数")
	}

	params.Seq = seq
	return params, nil
}

func executeProfileCustomProfileCard(rc *RequestContext) (onebot11.Message, error) {
	if rc == nil || rc.App == nil || rc.Cmd == nil {
		return nil, fmt.Errorf("profile service unavailable")
	}
	if rc.App.SekaiAPI == nil {
		return nil, fmt.Errorf("sekai api service unavailable")
	}
	if rc.App.Drawing == nil {
		return nil, fmt.Errorf("drawing service unavailable")
	}

	var params profileCustomProfileCardParams
	mergeParams(rc.Cmd.Params, &params)

	region := regionWithDefault(rc.Cmd.Region)
	target, err := resolveGameTarget(rc.Ctx, params.userQueryParams(), region, rc.Cmd.RegionExplicit, rc.App)
	if err != nil {
		return nil, err
	}
	region = resolvedTargetRegion(region, target)

	resp, err := fetchCachedSekaiUserProfile(rc.Ctx, rc.App, region, target.PJSKUserID)
	if err != nil {
		return nil, fmt.Errorf("获取玩家信息失败：%w", err)
	}
	if rc.App.Censor != nil {
		if !rc.App.Censor.CensorName(rc.Ctx, target.HarukiUserID, target.PJSKUserID, resp.User.Name, region) {
			resp.User.Name = ""
		}
		if !rc.App.Censor.CensorShortBio(rc.Ctx, target.HarukiUserID, target.PJSKUserID, resp.UserProfile.Word, region) {
			resp.UserProfile.Word = ""
		}
	}

	card, err := resolveCustomProfileCard(resp.UserCustomProfileCards, params)
	if err != nil {
		return nil, err
	}
	resources, err := buildCustomProfileResources(rc.Ctx, rc.App, region, *card, resp)
	if err != nil {
		return nil, fmt.Errorf("解析自定义档案资源失败：%w", err)
	}
	req := drawing.NewCustomProfileCardRenderRequest(region, *card, resp, resources)
	data, err := rc.App.Drawing.WithContext(rc.Ctx).GenerateCustomProfileCard(req)
	if err != nil {
		return nil, fmt.Errorf("渲染自定义档案失败：%w", err)
	}
	return imageMessage(rc.Ctx, data, rc.App, BotModulePJSK)
}

func (p profileCustomProfileCardParams) userQueryParams() userQueryParams {
	mode := strings.TrimSpace(p.Mode)
	if mode == "" {
		mode = "self"
	}
	return userQueryParams{
		Mode:           mode,
		Platform:       p.Platform,
		PlatformUserID: p.PlatformUserID,
		AtUserID:       p.AtUserID,
		PJSKUserID:     p.PJSKUserID,
		Selector:       p.Selector,
	}
}

func resolveCustomProfileCard(cards []sekaiapi.UserCustomProfileCard, params profileCustomProfileCardParams) (*sekaiapi.UserCustomProfileCard, error) {
	if len(cards) == 0 {
		return nil, onebot11.NewReplayError("当前公开profile中没有自定义档案")
	}

	var target *sekaiapi.UserCustomProfileCard
	if params.CustomProfileID > 0 && params.CustomProfileCardID > 0 {
		for i := range cards {
			card := &cards[i]
			if card.CustomProfileID == params.CustomProfileID && card.CustomProfileCardID == params.CustomProfileCardID {
				target = card
				break
			}
		}
		if target == nil {
			return nil, onebot11.NewReplayError("未找到第%d组第%d张自定义档案", params.CustomProfileID, params.CustomProfileCardID)
		}
	} else {
		targetSeq := params.Seq
		if targetSeq <= 0 {
			targetSeq = 1
		}
		for i := range cards {
			card := &cards[i]
			if card.Seq == targetSeq {
				target = card
				break
			}
		}
		if target == nil {
			return nil, onebot11.NewReplayError("未找到序号为%d的自定义档案", targetSeq)
		}
	}
	return target, nil
}

func parsePositiveIntArg(value string) (int, bool) {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return 0, false
	}
	return parsed, true
}
