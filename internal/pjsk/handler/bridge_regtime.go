package handler

import (
	"fmt"
	"strconv"

	"haruki-cloud/internal/pjsk/displaytime"
	"haruki-cloud/internal/pjsk/onebot11"
	renderregion "haruki-cloud/internal/pjsk/region"
)

func executeRegTime(rc *RequestContext) (onebot11.Message, error) {
	var p userQueryParams
	mergeParams(rc.Cmd.Params, &p)

	region := regionWithDefault(rc.Cmd.Region)

	target, err := resolveGameTarget(rc.Ctx, p, region, rc.Cmd.RegionExplicit, rc.App)
	if err != nil {
		return nil, err
	}
	pjskUserID := target.PJSKUserID
	bindingServer := resolvedTargetRegion(region, target)

	ts, err := calcRegistrationTime(pjskUserID, bindingServer)
	if err != nil {
		return nil, err
	}

	timeZone := resolveHarukiUserTimeZone(rc.Ctx, rc.App, target.HarukiUserID)
	regTime := displaytime.TimeFromUnixSeconds(ts, timeZone)
	relDur := displaytime.FormatRelativeDuration(displaytime.Now(timeZone).Sub(displaytime.TimeFromUnixSeconds(ts, timeZone)))
	maskedUID := maskPJSKUID(pjskUserID, target.Visible)

	text := fmt.Sprintf("UID %s 注册时间如下\n%s (%s) (%s)",
		maskedUID, displaytime.FormatTime(regTime, "2006-01-02 15:04:05"), timeZone, relDur)
	return onebot11.Message{onebot11.Text(text)}, nil
}

// calcRegistrationTime derives the approximate Unix registration timestamp from
// a PJSK game user ID and server region.
//
// JP/EN: the upper bits encode seconds since 2020-09-16T03:00:00 UTC.
// TW/KR/CN: the raw bits encode an absolute Unix timestamp.
func calcRegistrationTime(userID string, server string) (int64, error) {
	switch renderregion.Normalize(server) {
	case renderregion.JP, renderregion.EN:
		if len(userID) <= 3 {
			return 0, fmt.Errorf("账号ID格式不正确")
		}
		n, err := strconv.ParseInt(userID[:len(userID)-3], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("无效的账号ID：%w", err)
		}
		return 1600218000 + int64(float64(n)/(1024*4096)), nil
	case renderregion.TW, renderregion.KR, renderregion.CN:
		n, err := strconv.ParseInt(userID, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("无效的账号ID：%w", err)
		}
		return int64(float64(n) / (1024 * 1024 * 4096)), nil
	default:
		return 0, fmt.Errorf("不支持的服务器：%s", server)
	}
}
