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
	case errors.Is(err, sekaiapi.ErrAccountBindingNotFound):
		return onebot11.NewReplayError(
			"你还没有在工具箱绑定账号，请前往工具箱绑定你的账号并上传 suite 数据后再使用此命令\n" +
				ErrMsgToolboxURL,
		)
	case errors.Is(err, sekaiapi.ErrGameDataNotFound):
		return newSuiteDataNotFoundReplayError()
	case errors.Is(err, sekaiapi.ErrInvalidPlatformUser):
		return onebot11.NewReplayError("你无权查看这个账号的数据")
	case errors.Is(err, sekaiapi.ErrAccountOwnerBanned):
		return onebot11.NewReplayError("你被禁止使用此命令")
	case strings.Contains(message, "当前账号没有可用的 Suite 抓包数据"),
		strings.Contains(message, "找不到用户的 Suite 数据"),
		strings.Contains(message, "local user snapshot is not configured"):
		return newSuiteDataNotFoundReplayError()
	case strings.Contains(message, "当前账号没有可用的 MySekai 抓包数据"),
		strings.Contains(message, "找不到用户的 MySekai 数据"),
		strings.Contains(message, "user snapshot is not available (bind Toolbox or provide snapshot)"):
		return newMySekaiDataNotFoundReplayError()
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
