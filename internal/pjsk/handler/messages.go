package handler

import (
	"errors"
	"fmt"
	"strings"

	"haruki-cloud/internal/pjsk/accountdata"
)

// Common error messages for user-facing errors (Chinese).
const (
	// Data availability errors
	ErrMsgSuiteDataUnavailable   = "当前账号没有可用的 Suite 抓包数据"
	ErrMsgSuiteDataNotFound      = "找不到用户的 Suite 数据"
	ErrMsgMySekaiDataUnavailable = "当前账号没有可用的 MySekai 抓包数据"

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

func stringPtr(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}
