package accountdata

import (
	"context"
	"encoding/json"
	"fmt"
	sonic "github.com/bytedance/sonic"
	"strings"

	renderregion "haruki-cloud/internal/pjsk/region"
)

const (
	ProfileModeRender       = "profile"
	ProfileModeBind         = "profile-bind"
	ProfileModeBindList     = "profile-bind-list"
	ProfileModeBindSwap     = "profile-bind-swap"
	ProfileModeUnbind       = "profile-unbind"
	ProfileModeDefaultSet   = "profile-default-set"
	ProfileModeDefaultClear = "profile-default-clear"
	ProfileModeQueryUID     = "profile-query-uid"
)

type ProfileBindingCommandParams struct {
	Platform       string `json:"platform"`
	PlatformUserID string `json:"platform_user_id"`
	Selector       string `json:"selector,omitempty"`
	SelectorOther  string `json:"selector_other,omitempty"`
	Server         string `json:"server,omitempty"`
	Scope          string `json:"scope,omitempty"`
}

func DecodeProfileBindingParams(raw json.RawMessage) (ProfileBindingCommandParams, error) {
	var params ProfileBindingCommandParams
	if len(raw) == 0 {
		return params, fmt.Errorf("bridge: missing profile binding params")
	}
	if err := sonic.Unmarshal(raw, &params); err != nil {
		return params, fmt.Errorf("bridge: unmarshal profile binding params: %w", err)
	}
	params.Platform = strings.TrimSpace(params.Platform)
	params.PlatformUserID = strings.TrimSpace(params.PlatformUserID)
	params.Selector = strings.TrimSpace(params.Selector)
	params.SelectorOther = strings.TrimSpace(params.SelectorOther)
	params.Server = strings.TrimSpace(strings.ToLower(params.Server))
	params.Scope = strings.TrimSpace(params.Scope)
	if params.Platform == "" || params.PlatformUserID == "" {
		return params, fmt.Errorf("bridge: missing binding identity context")
	}
	if params.Server != "" {
		normalized := renderregion.Normalize(params.Server)
		if normalized.IsZero() {
			return params, fmt.Errorf("bridge: invalid binding server %q", params.Server)
		}
		params.Server = normalized.String()
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
		if params.Server != "" {
			items = filterBindingsByServer(items, params.Server)
		}
		return []byte(formatBindingListText(items, params.Server)), nil
	case ProfileModeBindSwap:
		items, err := service.Swap(ctx, params.Platform, params.PlatformUserID, params.Selector, params.SelectorOther, params.Server)
		if err != nil {
			return nil, err
		}
		if params.Server != "" {
			items = filterBindingsByServer(items, params.Server)
		}
		return []byte(formatBindingSwapResultText(params.Selector, params.SelectorOther, params.Server, items)), nil
	case ProfileModeUnbind:
		result, err := service.Unbind(ctx, params.Platform, params.PlatformUserID, params.Selector, params.Server)
		if err != nil {
			return nil, err
		}
		return []byte(formatUnbindResultText(result)), nil
	case ProfileModeDefaultSet:
		result, err := service.SetDefault(ctx, params.Platform, params.PlatformUserID, params.Selector, params.Server, params.Scope)
		if err != nil {
			return nil, err
		}
		return []byte(formatDefaultBindingResultText("已设置", result)), nil
	case ProfileModeDefaultClear:
		result, err := service.ClearDefault(ctx, params.Platform, params.PlatformUserID, params.Selector, params.Server, params.Scope)
		if err != nil {
			return nil, err
		}
		return []byte(formatDefaultBindingResultText("已取消", result)), nil
	case ProfileModeQueryUID:
		result, err := service.ResolveOwnBindingForUIDQuery(ctx, params.Platform, params.PlatformUserID, params.Selector, params.Server)
		if err != nil {
			return nil, err
		}
		return []byte(result.UserID), nil
	default:
		return nil, fmt.Errorf("bridge: unsupported profile binding mode %q", mode)
	}
}

func formatBindingListText(items []BindingListItem, server string) string {
	if len(items) == 0 {
		if server != "" {
			return fmt.Sprintf("你还没有绑定任何%s服PJSK账号", strings.ToUpper(server))
		}
		return "你还没有绑定任何PJSK账号"
	}

	lines := []string{"已绑定账号列表（u序号全局编号）:"}
	if server != "" {
		lines[0] = fmt.Sprintf("已绑定%s服账号列表（u序号按该区服编号）:", strings.ToUpper(server))
	}
	for i, item := range items {
		displayIdx := i + 1
		if server != "" {
			displayIdx = item.Index
		}
		line := fmt.Sprintf("u%d [%s] %s", displayIdx, strings.ToUpper(item.Server), formatBindingUID(item))
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

func formatBindingSwapResultText(left, right, server string, items []BindingListItem) string {
	header := fmt.Sprintf("已交换 %s 和 %s 的顺序", strings.TrimSpace(left), strings.TrimSpace(right))
	if server != "" {
		header = fmt.Sprintf("已交换%s服 %s 和 %s 的顺序", strings.ToUpper(server), strings.TrimSpace(left), strings.TrimSpace(right))
	}
	return header + "\n" + formatBindingListText(items, server)
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
