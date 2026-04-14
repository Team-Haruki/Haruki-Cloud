package handler

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"haruki-cloud/api/bot/onebot11"
	"haruki-cloud/utils/query"
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

	var tzOffset string
	if rc.App.PJSK != nil && target.HarukiUserID > 0 {
		if settings, sErr := query.NewClient(nil, nil, rc.App.PJSK, nil).GetPJSKSettings(rc.Ctx, target.HarukiUserID); sErr == nil && settings != nil {
			tzOffset = settings.TimeZoneOffset
		}
	}
	tz, tzLabel := parseUserTimeZone(tzOffset)
	regTime := time.Unix(ts, 0).In(tz)
	relDur := formatRelativeDuration(time.Since(time.Unix(ts, 0)))
	maskedUID := maskPJSKUID(pjskUserID, target.Visible)

	text := fmt.Sprintf("UID %s 注册时间如下\n%s (%s) (%s)",
		maskedUID, regTime.Format("2006-01-02 15:04:05"), tzLabel, relDur)
	return onebot11.Message{onebot11.Text(text)}, nil
}

// parseUserTimeZone parses a timezone offset string (e.g. "+09:00") into a
// time.Location and human-readable label. Empty string defaults to UTC+8.
func parseUserTimeZone(offset string) (*time.Location, string) {
	offset = strings.TrimSpace(offset)
	if offset == "" {
		return time.FixedZone("UTC+8", 8*3600), "UTC+8"
	}
	sign := 1
	raw := offset
	if strings.HasPrefix(raw, "-") {
		sign = -1
		raw = raw[1:]
	} else if strings.HasPrefix(raw, "+") {
		raw = raw[1:]
	}
	parts := strings.SplitN(raw, ":", 2)
	hours, err := strconv.Atoi(parts[0])
	if err != nil || hours < 0 || hours > 14 {
		return time.FixedZone("UTC+8", 8*3600), "UTC+8"
	}
	minutes := 0
	if len(parts) == 2 {
		minutes, _ = strconv.Atoi(parts[1])
	}
	totalSecs := sign * (hours*3600 + minutes*60)
	var label string
	if sign < 0 {
		label = fmt.Sprintf("UTC-%d", hours)
	} else {
		label = fmt.Sprintf("UTC+%d", hours)
	}
	if minutes != 0 {
		label = fmt.Sprintf("%s:%02d", label, minutes)
	}
	return time.FixedZone(label, totalSecs), label
}

// formatRelativeDuration formats a duration as a human-readable Chinese relative time.
// Only shows units starting from the largest non-zero unit down to minutes.
// e.g. "约2天5小时30分钟前", "约5小时30分钟前", "约30分钟前", "刚刚"
func formatRelativeDuration(d time.Duration) string {
	if d < time.Minute {
		return "刚刚"
	}
	mins := int(d.Minutes()) % 60
	hrs := int(d.Hours()) % 24
	days := int(d.Hours()) / 24
	if days > 0 {
		if hrs == 0 && mins == 0 {
			return fmt.Sprintf("约%d天前", days)
		}
		return fmt.Sprintf("约%d天%d小时%d分钟前", days, hrs, mins)
	}
	if hrs > 0 {
		if mins == 0 {
			return fmt.Sprintf("约%d小时前", hrs)
		}
		return fmt.Sprintf("约%d小时%d分钟前", hrs, mins)
	}
	return fmt.Sprintf("约%d分钟前", mins)
}

// calcRegistrationTime derives the approximate Unix registration timestamp from
// a PJSK game user ID and server region.
//
// JP/EN: the upper bits encode seconds since 2020-09-16T03:00:00 UTC.
// TW/KR/CN: the raw bits encode an absolute Unix timestamp.
func calcRegistrationTime(userID string, server string) (int64, error) {
	switch strings.ToLower(server) {
	case "jp", "en":
		if len(userID) <= 3 {
			return 0, fmt.Errorf("账号ID格式不正确")
		}
		n, err := strconv.ParseInt(userID[:len(userID)-3], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("无效的账号ID：%w", err)
		}
		return 1600218000 + int64(float64(n)/(1024*4096)), nil
	case "tw", "kr", "cn":
		n, err := strconv.ParseInt(userID, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("无效的账号ID：%w", err)
		}
		return int64(float64(n) / (1024 * 1024 * 4096)), nil
	default:
		return 0, fmt.Errorf("不支持的服务器：%s", server)
	}
}
