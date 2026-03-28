package userdata

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	ProfileModeRender       = "profile"
	ProfileModeBind         = "profile-bind"
	ProfileModeBindList     = "profile-bind-list"
	ProfileModeUnbind       = "profile-unbind"
	ProfileModeDefaultSet   = "profile-default-set"
	ProfileModeDefaultClear = "profile-default-clear"
)

type ProfileBindingCommandParams struct {
	Platform       string `json:"platform"`
	PlatformUserID string `json:"platform_user_id"`
	Selector       string `json:"selector,omitempty"`
	Scope          string `json:"scope,omitempty"`
}

func DecodeProfileBindingParams(raw json.RawMessage) (ProfileBindingCommandParams, error) {
	var params ProfileBindingCommandParams
	if len(raw) == 0 {
		return params, fmt.Errorf("bridge: missing profile binding params")
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return params, fmt.Errorf("bridge: unmarshal profile binding params: %w", err)
	}
	params.Platform = strings.TrimSpace(params.Platform)
	params.PlatformUserID = strings.TrimSpace(params.PlatformUserID)
	params.Selector = strings.TrimSpace(params.Selector)
	params.Scope = strings.TrimSpace(params.Scope)
	if params.Platform == "" || params.PlatformUserID == "" {
		return params, fmt.Errorf("bridge: missing binding identity context")
	}
	return params, nil
}

func ExecuteProfileBindingCommand(ctx context.Context, service *BindingService, mode string, params ProfileBindingCommandParams) ([]byte, error) {
	if service == nil || !service.IsReady() {
		return nil, ErrBindingServiceUnavailable
	}

	switch mode {
	case ProfileModeBind:
		result, err := service.Bind(ctx, params.Platform, params.PlatformUserID, params.Selector)
		if err != nil {
			return nil, err
		}
		return []byte(formatBindResultText(result)), nil
	case ProfileModeBindList:
		items, err := service.List(ctx, params.Platform, params.PlatformUserID)
		if err != nil {
			return nil, err
		}
		return []byte(formatBindingListText(items)), nil
	case ProfileModeUnbind:
		result, err := service.Unbind(ctx, params.Platform, params.PlatformUserID, params.Selector)
		if err != nil {
			return nil, err
		}
		return []byte(formatUnbindResultText(result)), nil
	case ProfileModeDefaultSet:
		result, err := service.SetDefault(ctx, params.Platform, params.PlatformUserID, params.Selector, params.Scope)
		if err != nil {
			return nil, err
		}
		return []byte(formatDefaultBindingResultText("已设置", result)), nil
	case ProfileModeDefaultClear:
		result, err := service.ClearDefault(ctx, params.Platform, params.PlatformUserID, params.Selector, params.Scope)
		if err != nil {
			return nil, err
		}
		return []byte(formatDefaultBindingResultText("已取消", result)), nil
	default:
		return nil, fmt.Errorf("bridge: unsupported profile binding mode %q", mode)
	}
}

func formatBindingListText(items []BindingListItem) string {
	if len(items) == 0 {
		return "你还没有绑定任何PJSK账号"
	}

	lines := []string{"已绑定账号列表（按账号ID升序）:"}
	for _, item := range items {
		line := fmt.Sprintf("u%d [%s] %s", item.Index, strings.ToUpper(item.Server), formatBindingUID(item))
		marks := make([]string, 0, 2)
		if item.IsGlobalDefault {
			marks = append(marks, "全局默认")
		}
		if item.IsServerDefault {
			marks = append(marks, strings.ToUpper(item.Server)+"服默认")
		}
		if len(marks) > 0 {
			line += " (" + strings.Join(marks, " / ") + ")"
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func formatBindResultText(result *BindResult) string {
	if result == nil {
		return "绑定成功"
	}

	lines := []string{fmt.Sprintf("%s服绑定成功: %s (%s)", strings.ToUpper(result.Server), result.UserName, maskUID(result.UserID))}
	if result.AlreadyBound {
		lines = append(lines, "该账号此前已绑定，本次跳过重复写入")
	}
	if result.SetGlobalDefault {
		lines = append(lines, "已自动设为你的全局默认绑定")
	}
	if result.SetServerDefault {
		lines = append(lines, fmt.Sprintf("已自动设为你的%s服默认绑定", strings.ToUpper(result.Server)))
	}
	if result.MultipleServerMatch {
		lines = append(lines, "该ID在多个服务器都存在，当前默认绑定第一个命中的服务器")
	}
	return strings.Join(lines, "\n")
}

func formatUnbindResultText(result *UnbindResult) string {
	if result == nil {
		return "解绑成功"
	}

	lines := []string{fmt.Sprintf("已解绑 [%s] %s", strings.ToUpper(result.Removed.Server), formatBindingUID(result.Removed))}
	if result.ReassignedGlobal != nil {
		lines = append(lines, fmt.Sprintf("已将全局默认绑定切换为 [%s] %s",
			strings.ToUpper(result.ReassignedGlobal.Server), formatBindingUID(*result.ReassignedGlobal)))
	}
	if result.ReassignedServer != nil {
		lines = append(lines, fmt.Sprintf("已将%s服默认绑定切换为 %s",
			strings.ToUpper(result.ReassignedServer.Server), formatBindingUID(*result.ReassignedServer)))
	}
	return strings.Join(lines, "\n")
}

func formatDefaultBindingResultText(prefix string, result *DefaultBindingResult) string {
	if result == nil {
		return prefix + "默认绑定"
	}
	scopeLabel := "全局默认绑定"
	if result.Scope == DefaultScopeServer {
		scopeLabel = strings.ToUpper(result.Server) + "服默认绑定"
	}
	return fmt.Sprintf("%s%s为 [%s] %s", prefix, strings.TrimSpace(scopeLabel), strings.ToUpper(result.Binding.Server), formatBindingUID(result.Binding))
}

func formatBindingUID(item BindingListItem) string {
	if item.Visible {
		return item.UserID
	}
	return maskUID(item.UserID)
}

func maskUID(uid string) string {
	if len(uid) <= 6 {
		return uid
	}
	return uid[:3] + strings.Repeat("*", len(uid)-6) + uid[len(uid)-3:]
}
