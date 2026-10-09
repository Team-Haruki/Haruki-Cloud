package handler

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"haruki-cloud/internal/core/upstreamerr"
	"haruki-cloud/internal/i18n"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/parser"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
	"haruki-cloud/utils/usererror"
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
		Path:        "profile/custom-profile-card",
		Commands: []string{
			"/自定义个人信息", "/cp", "/自定义资料卡",
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
		return params, usererror.Misuse(i18n.M("profile.custom_card.index_required"))
	}

	seq, ok := parsePositiveIntArg(args[0])
	if !ok {
		return params, usererror.Invalid(i18n.M("profile.custom_card.index_positive"))
	}

	params.Seq = seq
	return params, nil
}

func executeProfileCustomProfileCard(rc *RequestContext) (onebot11.Message, error) {
	if rc == nil || rc.App == nil || rc.Cmd == nil {
		return nil, usererror.Misconfigured(errors.New("profile service unavailable"))
	}
	if rc.App.SekaiAPI == nil {
		return nil, usererror.Misconfigured(errors.New("sekai api service unavailable"))
	}
	if rc.App.Drawing == nil {
		return nil, drawing.ErrNotConfigured
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
		return nil, playerProfileFetchError(err)
	}
	if rc.App.Censor != nil {
		if !rc.App.Censor.CensorName(rc.Ctx, target.HarukiUserID, target.PJSKUserID, resp.User.Name, region) {
			resp.User.Name = ""
		}
		if !rc.App.Censor.CensorShortBio(rc.Ctx, target.HarukiUserID, target.PJSKUserID, resp.UserProfile.Word, region) {
			resp.UserProfile.Word = ""
		}
	}

	finishBuild := measureCommandOperation(rc.Ctx, "custom_profile.build")
	defer finishBuild()
	card, err := resolveCustomProfileCard(resp.UserCustomProfileCards, params)
	if err != nil {
		return nil, err
	}
	resources, err := buildCustomProfileResources(rc.Ctx, rc.App, region, *card, resp)
	if err != nil {
		return nil, usererror.Wrap(usererror.CodeUnavailable, i18n.M("profile.custom_card.failed"), fmt.Errorf("resolve custom profile resources: %w", err))
	}
	req := drawing.NewCustomProfileCardRenderRequest(region, *card, resp, resources)
	finishBuild()
	data, err := rc.App.Drawing.WithContext(rc.Ctx).GenerateCustomProfileCardImage(req)
	if err != nil {
		return nil, customProfileRenderError(err)
	}
	return rc.RenderedImageMessage(data)
}

// customProfileRenderError keeps a classified renderer failure and gives any
// other one the custom profile reply.
func customProfileRenderError(err error) error {
	if typed := upstreamerr.UserError(err); typed != nil {
		return typed
	}
	return usererror.Wrap(usererror.CodeUnavailable, i18n.M("profile.custom_card.failed"), err)
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
		return nil, usererror.New(usererror.CodeNotFound, i18n.M("profile.custom_card.none"))
	}
	if params.CustomProfileID > 0 && params.CustomProfileCardID > 0 {
		if target := findCustomProfileCard(cards, params.CustomProfileID, params.CustomProfileCardID); target != nil {
			return target, nil
		}
		return nil, usererror.New(usererror.CodeNotFound, i18n.M("profile.custom_card.not_found_pair", i18n.Data{"UserGroup": i18n.UserNumber(params.CustomProfileID), "UserCard": i18n.UserNumber(params.CustomProfileCardID)}))
	}
	page := max(1, params.Seq)
	if page > len(cards) {
		return nil, usererror.New(usererror.CodeNotFound, i18n.M("profile.custom_card.not_found_page", i18n.Data{"UserPage": i18n.UserNumber(page), "Total": len(cards)}))
	}
	ordered := slices.Clone(cards)
	slices.SortStableFunc(ordered, func(a, b sekaiapi.UserCustomProfileCard) int { return a.Seq - b.Seq })
	return &ordered[page-1], nil
}

func findCustomProfileCard(cards []sekaiapi.UserCustomProfileCard, profileID, cardID int) *sekaiapi.UserCustomProfileCard {
	for i := range cards {
		if cards[i].CustomProfileID == profileID && cards[i].CustomProfileCardID == cardID {
			return &cards[i]
		}
	}
	return nil
}

func parsePositiveIntArg(value string) (int, bool) {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return 0, false
	}
	return parsed, true
}
