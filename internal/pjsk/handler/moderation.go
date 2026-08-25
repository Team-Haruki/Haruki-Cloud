package handler

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/parser"
	"haruki-cloud/internal/pjsk/render/common"

	json "github.com/bytedance/sonic"
)

const (
	modeGlobalKill = "global-kill"
	modeGlobalBack = "global-back"
)

type globalKillParams struct {
	Platform       string `json:"platform"`
	PlatformUserID string `json:"platform_user_id"`
	QQID           string `json:"qq_id"`
	Reason         string `json:"reason"`
	Days           *int   `json:"days,omitempty"`
}

type globalBackParams struct {
	Platform       string `json:"platform"`
	PlatformUserID string `json:"platform_user_id"`
	QQID           string `json:"qq_id"`
}

func (sekaiHandlers) GlobalKillHandle() HarukiSekaiCommandHandler {
	const usage = "使用方式:\n/kill QQ号 原因 [天数]"
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path:        "admin/kill",
		Commands:    []string{"/kill"},
		Helper:      usage,
		ParseUIDArg: common.BoolPtr(false),
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			qqID, reason, days, err := parseGlobalKillArgs(ctx.GetArgs(), usage)
			if err != nil {
				return nil, err
			}
			if ctx.GetPlatform() == "qq" && strings.TrimSpace(ctx.GetUserId()) == qqID {
				return nil, onebot11.NewReplayError("不能使用 /kill 封禁自己")
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleAdmin, modeGlobalKill, globalKillParams{
				Platform:       ctx.GetPlatform(),
				PlatformUserID: ctx.GetUserId(),
				QQID:           qqID,
				Reason:         reason,
				Days:           days,
			}), nil
		},
	}, executeGlobalModeration)
}

func (sekaiHandlers) GlobalBackHandle() HarukiSekaiCommandHandler {
	const usage = "使用方式:\n/back QQ号"
	return bindRequestExecutor(HarukiSekaiCommandHandler{
		Path:        "admin/back",
		Commands:    []string{"/back"},
		Helper:      usage,
		ParseUIDArg: common.BoolPtr(false),
		handleFunc: func(ctx HarrukiSekaiHandlerContext) (*CommandRequest, error) {
			qqID, err := parseQQIDArg(ctx.GetArgs(), usage)
			if err != nil {
				return nil, err
			}
			return makeCommandRequestWithParams(ctx, parser.ModuleAdmin, modeGlobalBack, globalBackParams{
				Platform:       ctx.GetPlatform(),
				PlatformUserID: ctx.GetUserId(),
				QQID:           qqID,
			}), nil
		},
	}, executeGlobalModeration)
}

func parseGlobalKillArgs(args, usage string) (string, string, *int, error) {
	fields := strings.Fields(strings.TrimSpace(args))
	if len(fields) < 2 || len(fields) > 3 {
		return "", "", nil, onebot11.NewReplayError("%s", usage)
	}
	qqID, err := validateQQID(fields[0])
	if err != nil {
		return "", "", nil, err
	}

	var days *int
	if len(fields) == 3 {
		parsed, err := strconv.Atoi(fields[2])
		if err != nil || parsed <= 0 {
			return "", "", nil, onebot11.NewReplayError("封禁天数必须为正整数")
		}
		days = &parsed
	}
	reason := strings.TrimSpace(fields[1])
	if reason == "" {
		return "", "", nil, onebot11.NewReplayError("请输入封禁原因")
	}
	if len([]rune(reason)) > 255 {
		return "", "", nil, onebot11.NewReplayError("封禁原因不能超过255个字符")
	}
	return qqID, reason, days, nil
}

func parseQQIDArg(args, usage string) (string, error) {
	fields := strings.Fields(strings.TrimSpace(args))
	if len(fields) != 1 {
		return "", onebot11.NewReplayError("%s", usage)
	}
	return validateQQID(fields[0])
}

func validateQQID(value string) (string, error) {
	value = strings.TrimSpace(value)
	qqID, err := strconv.ParseInt(value, 10, 64)
	if err != nil || qqID <= 0 {
		return "", onebot11.NewReplayError("QQ号必须为正整数")
	}
	return strconv.FormatInt(qqID, 10), nil
}

func executeGlobalModeration(rc *RequestContext) (onebot11.Message, error) {
	if rc == nil || rc.App == nil || rc.App.BanChecker == nil {
		return nil, fmt.Errorf("全局封禁服务未就绪，请稍后再试")
	}
	switch rc.Cmd.Mode {
	case modeGlobalKill:
		var params globalKillParams
		if err := json.Unmarshal(rc.Cmd.Params, &params); err != nil {
			return nil, fmt.Errorf("解析封禁参数失败: %w", err)
		}
		if !rc.App.BanChecker.IsAdmin(params.Platform, params.PlatformUserID) {
			return nil, onebot11.NewReplayError("你不是全局管理员")
		}
		var expiresAt *time.Time
		if params.Days != nil {
			value := time.Now().AddDate(0, 0, *params.Days)
			expiresAt = &value
		}
		status, err := rc.App.BanChecker.Kill(rc.Ctx, params.QQID, params.Reason, expiresAt)
		if err != nil {
			return nil, err
		}
		message := fmt.Sprintf("已全局封禁 QQ %s\n原因：%s\n期限：永久", params.QQID, status.Reason)
		if status.ExpiresAt != nil {
			message = fmt.Sprintf("已全局封禁 QQ %s\n原因：%s\n解封时间：%s", params.QQID, status.Reason, status.ExpiresAt.Format("2006-01-02 15:04:05 MST"))
		}
		return onebot11.Message{onebot11.Text(message)}, nil
	case modeGlobalBack:
		var params globalBackParams
		if err := json.Unmarshal(rc.Cmd.Params, &params); err != nil {
			return nil, fmt.Errorf("解析解封参数失败: %w", err)
		}
		if !rc.App.BanChecker.IsAdmin(params.Platform, params.PlatformUserID) {
			return nil, onebot11.NewReplayError("你不是全局管理员")
		}
		if err := rc.App.BanChecker.Back(rc.Ctx, params.QQID); err != nil {
			return nil, err
		}
		return onebot11.Message{onebot11.Text(fmt.Sprintf("已解除 QQ %s 的全局封禁", params.QQID))}, nil
	default:
		return nil, fmt.Errorf("不支持的全局封禁操作: %s", rc.Cmd.Mode)
	}
}
