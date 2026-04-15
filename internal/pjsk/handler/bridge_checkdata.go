package handler

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"haruki-cloud/api/bot/onebot11"
	accountdata "haruki-cloud/internal/pjsk/userdata"
	"haruki-cloud/utils/query"
	sekaiutils "haruki-cloud/internal/pjsk/sekai"
)

func executeCheckData(rc *RequestContext) (onebot11.Message, error) {
	var p userQueryParams
	mergeParams(rc.Cmd.Params, &p)

	region := regionWithDefault(rc.Cmd.Region)

	var dataType sekaiutils.ToolboxDataType
	var label string
	var uid int64
	var platform string
	var platformUserID string
	var pjskUID string
	var bindingVisible bool
	var resolvedHarukiID int
	var bindingServer string

	resolveBinding := func(requireSuite, requireMySekai bool) (*accountdata.ResolvedBinding, int, error) {
		hid, binding, err := resolveBindingWithFallback(
			rc.Ctx, rc.App.Bindings, p.Platform, p.PlatformUserID, region,
			rc.Cmd.RegionExplicit,
			bindingResolutionOptions{
				RequireSuite:   requireSuite,
				RequireMySekai: requireMySekai,
				Selector:       p.Selector,
			},
		)
		if err != nil {
			return nil, 0, fmt.Errorf("解析绑定账号失败：%w", err)
		}
		return binding, hid, nil
	}

	switch rc.Cmd.Mode {
	case "mysekai":
		if p.Mode != "self" {
			return nil, fmt.Errorf("MySekai抓包相关内容仅支持查询自己的数据")
		}
		binding, hid, err := resolveBinding(false, true)
		if err != nil {
			return nil, err
		}
		if !hasUsableMySekaiData(binding) {
			return nil, fmt.Errorf("当前账号没有可用的 MySekai 抓包数据")
		}
		uid, err = strconv.ParseInt(binding.PJSKUserID, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("无效的账号ID：%w", err)
		}
		platform = p.Platform
		platformUserID = p.PlatformUserID
		dataType = sekaiutils.ToolboxDataTypeMySekai
		label = "MySekai"
		pjskUID = binding.PJSKUserID
		bindingVisible = binding.Visible
		resolvedHarukiID = hid
		bindingServer = binding.Server
	default:
		if p.Mode != "self" {
			return nil, fmt.Errorf("Suite抓包相关内容仅支持查询自己的数据")
		}
		binding, hid, err := resolveBinding(true, false)
		if err != nil {
			return nil, err
		}
		if !hasUsableSuiteData(binding) {
			return nil, fmt.Errorf("当前账号没有可用的 Suite 抓包数据")
		}
		uid, err = strconv.ParseInt(binding.PJSKUserID, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("无效的账号ID：%w", err)
		}
		platform = p.Platform
		platformUserID = p.PlatformUserID
		dataType = sekaiutils.ToolboxDataTypeSuite
		label = "Suite"
		pjskUID = binding.PJSKUserID
		bindingVisible = binding.Visible
		resolvedHarukiID = hid
		bindingServer = binding.Server
	}

	if bindingServer == "" {
		bindingServer = region
	}

	raw, err := sekaiutils.GetToolboxClient().GetUploadTime(bindingServer, dataType, uid, platform, platformUserID)
	if err != nil {
		return nil, fmt.Errorf("获取%s更新时间失败：%w", label, err)
	}

	ts, err := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("解析更新时间失败：%w", err)
	}

	var tzOffset string
	if rc.App.PJSK != nil && resolvedHarukiID > 0 {
		if settings, sErr := query.NewClient(nil, nil, rc.App.PJSK, nil).GetPJSKSettings(rc.Ctx, resolvedHarukiID); sErr == nil && settings != nil {
			tzOffset = settings.TimeZoneOffset
		}
	}
	tz, tzLabel := parseUserTimeZone(tzOffset)
	uploadTime := time.Unix(ts, 0).In(tz)
	relDur := formatRelativeDuration(time.Since(time.Unix(ts, 0)))
	maskedUID := maskPJSKUID(pjskUID, bindingVisible)

	text := fmt.Sprintf("UID %s 的%s数据更新时间:\n%s (%s) (%s)",
		maskedUID, label, uploadTime.Format("2006-01-02 15:04:05"), tzLabel, relDur)
	return onebot11.Message{onebot11.Text(text)}, nil
}
