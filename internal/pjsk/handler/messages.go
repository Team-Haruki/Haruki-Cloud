package handler

import (
	"errors"
	"fmt"
	"strings"

	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/accountdata"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
)

// Common error messages for user-facing errors (Chinese).
const (
	ErrMsgBindingNotFound = "未找到绑定的游戏账号，请先使用 \"/绑定<id>\" 绑定后再使用此命令\n" +
		"如果已经绑定，请确保已设置默认账号或绑定的账号在当前服务器可见"
	ErrMsgToolboxURL            = "工具箱地址：https://haruki.seiunx.com/"
	ErrMsgPrivateDataSetupGuide = "请前往工具箱先注册账号、绑定自己QQ账号、再绑定游戏账号后上传自己的数据，才能使用此功能"

	// Data availability errors
	ErrMsgSuiteDataUnavailable     = "没有找到有效的 suite 数据，" + ErrMsgPrivateDataSetupGuide + "\n" + ErrMsgToolboxURL
	ErrMsgSuiteDataNotFound        = ErrMsgSuiteDataUnavailable
	ErrMsgMySekaiDataUnavailable   = "没有找到有效的 mysekai 数据，" + ErrMsgPrivateDataSetupGuide + "\n" + ErrMsgToolboxURL
	ErrMsgMySekaiDataNotFound      = ErrMsgMySekaiDataUnavailable
	ErrMsgCardCatalogRequiresSuite = ErrMsgSuiteDataNotFound

	// Permission errors
	ErrMsgSelfQueryOnly = "%s仅支持查询自己的数据"

	// Service errors
	ErrMsgBindingServiceUnavailable = "绑定服务未就绪"

	// Card catalog notice titles
	CardCatalogTitleNoBinding = "未绑定账号，当前显示全服卡牌"
	CardCatalogTitleNoSuite   = "未获取到 Suite 卡牌数据，当前显示全服卡牌"
)

// unsupportedModeError returns a standardized error for unhandled command modes.
func unsupportedModeError(module, mode string) error {
	return fmt.Errorf("bridge: unsupported %s mode %q", module, mode)
}

func normalizeBindingLookupError(err error, fallback string) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, accountdata.ErrNoBinding), errors.Is(err, accountdata.ErrBindingServiceUnavailable):
		return err
	case strings.TrimSpace(fallback) == "":
		return err
	default:
		return fmt.Errorf("%s：%w", fallback, err)
	}
}

func newBindingRequiredReplayError() error {
	return onebot11.NewReplayError(ErrMsgBindingNotFound)
}

func newSuiteDataNotFoundReplayError() error {
	return onebot11.NewReplayError(ErrMsgSuiteDataNotFound)
}

func newSuiteDataNotFoundReplayErrorForBinding(binding *accountdata.ResolvedBinding) error {
	return onebot11.NewReplayError("%s", buildPrivateDataNotFoundMessage("suite", binding))
}

func newMySekaiDataNotFoundReplayError() error {
	return onebot11.NewReplayError(ErrMsgMySekaiDataNotFound)
}

func newMySekaiDataNotFoundReplayErrorForBinding(binding *accountdata.ResolvedBinding) error {
	return onebot11.NewReplayError("%s", buildPrivateDataNotFoundMessage("mysekai", binding))
}

func buildPrivateDataNotFoundMessage(dataLabel string, binding *accountdata.ResolvedBinding) string {
	dataLabel = strings.TrimSpace(strings.ToLower(dataLabel))
	if dataLabel == "" {
		dataLabel = "suite"
	}

	if binding == nil {
		return fmt.Sprintf("没有找到有效的 %s 数据，%s\n%s", dataLabel, ErrMsgPrivateDataSetupGuide, ErrMsgToolboxURL)
	}

	server := strings.ToUpper(strings.TrimSpace(binding.Server))
	uid := maskUserFacingGameID(binding.PJSKUserID, binding.Visible)
	if server == "" || uid == "" {
		return fmt.Sprintf("没有找到有效的 %s 数据，%s\n%s", dataLabel, ErrMsgPrivateDataSetupGuide, ErrMsgToolboxURL)
	}
	return fmt.Sprintf("当前%s服%s没有找到有效的 %s 数据，%s\n%s", server, uid, dataLabel, ErrMsgPrivateDataSetupGuide, ErrMsgToolboxURL)
}

func normalizeToolboxDataLabel(dataLabel string) string {
	dataLabel = strings.TrimSpace(strings.ToLower(dataLabel))
	switch dataLabel {
	case "mysekai":
		return "mysekai"
	default:
		return "suite"
	}
}

func normalizeToolboxDataFetchError(err error, dataLabel string, binding *accountdata.ResolvedBinding) error {
	if err == nil {
		return nil
	}
	if _, ok := errors.AsType[onebot11.ReplayError](err); ok {
		return err
	}

	dataLabel = normalizeToolboxDataLabel(dataLabel)
	switch {
	case errors.Is(err, sekaiapi.ErrAccountBindingNotFound):
		return onebot11.NewReplayError(
			"你还没有在工具箱绑定游戏账号，无法获取%s数据，请前往工具箱绑定游戏账号并上传数据后重试\n%s",
			dataLabel,
			ErrMsgToolboxURL,
		)
	case errors.Is(err, sekaiapi.ErrGameDataNotFound):
		if dataLabel == "mysekai" {
			return newMySekaiDataNotFoundReplayErrorForBinding(binding)
		}
		return newSuiteDataNotFoundReplayErrorForBinding(binding)
	case errors.Is(err, sekaiapi.ErrInvalidPlatformUser):
		return onebot11.NewReplayError(
			"当前QQ号未在工具箱完成绑定，或无权访问该%s数据，请前往工具箱绑定当前QQ号后重试\n%s",
			dataLabel,
			ErrMsgToolboxURL,
		)
	case errors.Is(err, sekaiapi.ErrAccountOwnerBanned):
		return onebot11.NewReplayError("工具箱账号已被封禁，无法获取%s数据", dataLabel)
	}

	message := strings.TrimSpace(err.Error())
	switch {
	case strings.Contains(message, "toolbox: request failed after retries"),
		strings.Contains(message, "context deadline exceeded"),
		strings.Contains(message, "Client.Timeout exceeded"):
		return onebot11.NewReplayError("连接工具箱超时或网络异常，请稍后再试")
	case strings.Contains(message, "toolbox: zstd reader init failed"),
		strings.Contains(message, "toolbox: zstd decompression failed"),
		strings.Contains(message, "toolbox: failed to parse game bindings response"):
		return onebot11.NewReplayError("工具箱返回数据解析失败，请稍后再试")
	}

	var apiErr *sekaiapi.ToolboxAPIError
	if errors.As(err, &apiErr) {
		if translated, ok := translateToolboxAPIDetail(dataLabel, apiErr.Message); ok {
			return onebot11.NewReplayError("%s", translated)
		}
		switch apiErr.StatusCode {
		case 503:
			return onebot11.NewReplayError("工具箱服务暂时不可用，请稍后再试")
		case 403:
			return onebot11.NewReplayError("工具箱拒绝了当前%s数据请求", dataLabel)
		case 404:
			return onebot11.NewReplayError("工具箱未找到当前%s数据", dataLabel)
		default:
			return onebot11.NewReplayError("工具箱请求失败（状态 %d）", apiErr.StatusCode)
		}
	}
	return err
}

func normalizeSekaiAPIFetchError(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := errors.AsType[onebot11.ReplayError](err); ok {
		return err
	}

	switch {
	case errors.Is(err, sekaiapi.ErrClientNotConfigured):
		return onebot11.NewReplayError("SekaiAPI 服务未就绪，请稍后再试")
	case errors.Is(err, sekaiapi.ErrServerMaintenance):
		return onebot11.NewReplayError("SekaiAPI 拉取失败：当前游戏服务器维护中，请稍后再试")
	case errors.Is(err, sekaiapi.ErrUserNotFound):
		return onebot11.NewReplayError("SekaiAPI 拉取失败：找不到该玩家公开信息")
	}

	message := strings.TrimSpace(err.Error())
	switch {
	case strings.Contains(message, "sekai api: request failed after retries"),
		strings.Contains(message, "context deadline exceeded"),
		strings.Contains(message, "Client.Timeout exceeded"):
		return onebot11.NewReplayError("SekaiAPI 拉取失败：连接超时或网络异常，请稍后再试")
	}

	var apiErr *sekaiapi.APIError
	if errors.As(err, &apiErr) {
		if translated, ok := translateSekaiAPIDetail(apiErr.Message); ok {
			return onebot11.NewReplayError("SekaiAPI 拉取失败：%s", translated)
		}
		if strings.TrimSpace(apiErr.Message) == "" {
			return onebot11.NewReplayError("SekaiAPI 拉取失败（状态 %d）", apiErr.StatusCode)
		}
		return onebot11.NewReplayError("SekaiAPI 拉取失败（状态 %d）", apiErr.StatusCode)
	}

	if translated, ok := translateSekaiAPIDetail(message); ok && (strings.Contains(message, "sekai api") || strings.Contains(strings.ToLower(message), "sekaiapi")) {
		return onebot11.NewReplayError("SekaiAPI 拉取失败：%s", translated)
	}

	if strings.Contains(message, "sekai api:") || strings.Contains(strings.ToLower(message), "sekaiapi") {
		return onebot11.NewReplayError("SekaiAPI 拉取失败，请稍后再试")
	}
	return err
}

func maskUserFacingGameID(uid string, visible bool) string {
	uid = strings.TrimSpace(uid)
	if uid == "" || visible || len(uid) <= 6 {
		return uid
	}
	return uid[:3] + strings.Repeat("*", len(uid)-6) + uid[len(uid)-3:]
}

// WrapDomainError converts well-known domain errors into ReplayError so that
// transport layers (e.g. api/bot/pjsk) only need to distinguish ReplayError
// from unexpected errors, without duplicating Chinese user-facing messages.
func WrapDomainError(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := errors.AsType[onebot11.ReplayError](err); ok {
		return err
	}
	message := strings.TrimSpace(err.Error())
	switch {
	case errors.Is(err, accountdata.ErrNoBinding):
		return newBindingRequiredReplayError()
	case errors.Is(err, accountdata.ErrBindingServiceUnavailable):
		return onebot11.NewReplayError("绑定服务未就绪，请稍后再试")
	case errors.Is(err, sekaiapi.ErrAccountBindingNotFound),
		errors.Is(err, sekaiapi.ErrGameDataNotFound),
		errors.Is(err, sekaiapi.ErrInvalidPlatformUser),
		errors.Is(err, sekaiapi.ErrAccountOwnerBanned):
		return normalizeToolboxDataFetchError(err, "suite", nil)
	case strings.Contains(message, "当前账号没有可用的 Suite 抓包数据"),
		strings.Contains(message, "找不到用户的 Suite 数据"),
		strings.Contains(message, "local user snapshot is not configured"):
		return newSuiteDataNotFoundReplayError()
	case strings.Contains(message, "当前账号没有可用的 MySekai 抓包数据"),
		strings.Contains(message, "找不到用户的 MySekai 数据"),
		strings.Contains(message, "user snapshot is not available (bind Toolbox or provide snapshot)"):
		return newMySekaiDataNotFoundReplayError()
	case strings.Contains(message, "toolbox:"),
		strings.Contains(message, "toolbox api error:"):
		return normalizeToolboxDataFetchError(err, "suite", nil)
	case strings.Contains(message, "sekai api:"),
		strings.Contains(strings.ToLower(message), "sekaiapi"):
		if normalized := normalizeSekaiAPIFetchError(err); normalized != err {
			return normalized
		}
	case strings.Contains(message, "tracker:"),
		strings.Contains(message, "tracker api error:"):
		if normalized := normalizeTrackerUserFacingError(err); normalized != err {
			return normalized
		}
	case strings.Contains(strings.ToLower(message), "deck-service"):
		if normalized := normalizeDeckServiceUserFacingError(err); normalized != err {
			return normalized
		}
	case message == "drawing client is not configured",
		message == "image storage is not configured",
		strings.Contains(strings.ToLower(message), "drawing "),
		strings.HasPrefix(strings.ToLower(message), "api request failed with status:"),
		strings.Contains(strings.ToLower(message), "asset path is empty"):
		if normalized := normalizeDrawingUserFacingError(err); normalized != err {
			return normalized
		}
	}
	return err
}

func stringPtr(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}
