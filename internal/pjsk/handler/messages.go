package handler

import "fmt"

// Common error messages for user-facing errors (Chinese).
const (
	// Data availability errors
	ErrMsgSuiteDataUnavailable   = "当前账号没有可用的 Suite 抓包数据"
	ErrMsgMySekaiDataUnavailable = "当前账号没有可用的 MySekai 抓包数据"

	// Permission errors
	ErrMsgSelfQueryOnly = "%s仅支持查询自己的数据"

	// Service errors
	ErrMsgBindingServiceUnavailable = "绑定服务未就绪"
)

// unsupportedModeError returns a standardized error for unhandled command modes.
func unsupportedModeError(module, mode string) error {
	return fmt.Errorf("bridge: unsupported %s mode %q", module, mode)
}
