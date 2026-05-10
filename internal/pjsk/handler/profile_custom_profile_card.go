package handler

import (
	"fmt"
	"strconv"
	"strings"

	json "github.com/bytedance/sonic"

	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/region"
	rendersnapshot "haruki-cloud/internal/pjsk/render/snapshot"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
)

const profileModeCustomProfileCardThumbnail = "profile-custom-profile-card-thumbnail"

type profileCustomProfileCardThumbnailParams struct {
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
				"/自定义档案", "/pjsk custom profile", "/custom-profile",
			},
		},
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			params, err := buildProfileCustomProfileCardThumbnailParams(ctx)
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleProfile, profileModeCustomProfileCardThumbnail, params), nil
		},
	}, executeProfile)
}

func buildProfileCustomProfileCardThumbnailParams(ctx HarrukiSekaiHandlerContext) (profileCustomProfileCardThumbnailParams, error) {
	query, err := resolveSelfOnlyQueryParams(ctx)
	if err != nil {
		return profileCustomProfileCardThumbnailParams{}, err
	}

	params := profileCustomProfileCardThumbnailParams{UserQueryParams: query, Seq: 1}
	args := strings.Fields(strings.TrimSpace(ctx.GetArgs()))
	if len(args) == 0 {
		return params, nil
	}
	if len(args) != 2 {
		return params, onebot11.NewReplayError("使用方式:\n%s\n%s 组ID 档案ID\n%s u2\n%s 组ID 档案ID u2",
			ctx.originalTriggerCmd,
			ctx.originalTriggerCmd,
			ctx.originalTriggerCmd,
			ctx.originalTriggerCmd,
		)
	}

	customProfileID, ok := parsePositiveIntArg(args[0])
	if !ok {
		return params, onebot11.NewReplayError("组ID必须是正整数")
	}
	customProfileCardID, ok := parsePositiveIntArg(args[1])
	if !ok {
		return params, onebot11.NewReplayError("档案ID必须是正整数")
	}

	params.Seq = 0
	params.CustomProfileID = customProfileID
	params.CustomProfileCardID = customProfileCardID
	return params, nil
}

func executeProfileCustomProfileCardThumbnail(rc *RequestContext) (onebot11.Message, error) {
	if rc == nil || rc.App == nil || rc.App.SekaiAPI == nil {
		return nil, fmt.Errorf("sekai api service unavailable")
	}

	binding, suiteSnapshot, suiteErr := rc.requireVisibleSuiteSnapshot()
	if suiteErr != nil {
		return nil, suiteErr
	}
	if suiteSnapshot == nil {
		return nil, newSuiteDataNotFoundReplayErrorForBinding(binding)
	}

	var params profileCustomProfileCardThumbnailParams
	mergeParams(rc.Cmd.Params, &params)

	region := renderregion.Normalize(rc.RegionStr)
	if binding != nil {
		if bindingRegion := renderregion.Normalize(binding.Server); !bindingRegion.IsZero() {
			region = bindingRegion
		}
	}
	if region.IsZero() {
		region = renderregion.JP
	}
	if region != renderregion.JP && region != renderregion.EN {
		return nil, onebot11.NewReplayError("当前%s服暂不支持自定义档案缩略图", strings.ToUpper(region.String()))
	}

	cards, err := resolveSuiteCustomProfileCards(suiteSnapshot)
	if err != nil {
		return nil, err
	}
	if len(cards) == 0 {
		if resp := rc.GetPublicProfileResponse(); resp != nil {
			cards = append(cards, resp.UserCustomProfileCards...)
		}
	}
	thumbnailPath, err := resolveCustomProfileCardThumbnailPath(cards, params)
	if err != nil {
		return nil, err
	}

	data, err := rc.App.SekaiAPI.GetCustomProfileCardThumbnail(region.String(), thumbnailPath)
	if err != nil {
		return nil, fmt.Errorf("获取自定义档案缩略图失败：%w", err)
	}
	return imageMessage(rc.Ctx, data, rc.App, BotModulePJSK)
}

func resolveSuiteCustomProfileCards(suiteSnapshot rendersnapshot.Snapshot) ([]sekaiapi.UserCustomProfileCard, error) {
	if suiteSnapshot == nil {
		return nil, newSuiteDataNotFoundReplayError()
	}

	raw, err := suiteSnapshot.RawValue("userCustomProfileCards")
	if err != nil {
		return nil, onebot11.NewReplayError("当前suite数据中没有找到自定义档案信息")
	}

	var cards []sekaiapi.UserCustomProfileCard
	if err := json.Unmarshal(raw, &cards); err != nil {
		return nil, fmt.Errorf("解析自定义档案数据失败：%w", err)
	}
	return cards, nil
}

func resolveCustomProfileCardThumbnailPath(cards []sekaiapi.UserCustomProfileCard, params profileCustomProfileCardThumbnailParams) (string, error) {
	if len(cards) == 0 {
		return "", onebot11.NewReplayError("当前suite与公开profile中都没有自定义档案")
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
			return "", onebot11.NewReplayError("未找到第%d组第%d张自定义档案", params.CustomProfileID, params.CustomProfileCardID)
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
			return "", onebot11.NewReplayError("未找到序号为%d的自定义档案", targetSeq)
		}
	}

	thumbnailPath := strings.TrimSpace(target.ThumbnailPath)
	if thumbnailPath == "" {
		return "", onebot11.NewReplayError("目标自定义档案没有可用的缩略图")
	}
	if !looksLikeTwoSegmentImagePath(thumbnailPath) {
		return "", onebot11.NewReplayError("目标自定义档案的缩略图路径格式无效")
	}
	return thumbnailPath, nil
}

func looksLikeTwoSegmentImagePath(value string) bool {
	parts := strings.Split(strings.TrimSpace(value), "/")
	return len(parts) == 2 && strings.TrimSpace(parts[0]) != "" && strings.TrimSpace(parts[1]) != ""
}

func parsePositiveIntArg(value string) (int, bool) {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return 0, false
	}
	return parsed, true
}
