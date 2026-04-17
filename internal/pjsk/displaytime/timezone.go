package displaytime

import (
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	DefaultTimeZone = "Asia/Shanghai"
	defaultLayout   = "2006-01-02 15:04:05"
)

var locationCache sync.Map

func NormalizeTimeZone(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return DefaultTimeZone
	}
	if strings.EqualFold(trimmed, DefaultTimeZone) {
		return DefaultTimeZone
	}
	if _, err := time.LoadLocation(trimmed); err == nil {
		return trimmed
	}
	return DefaultTimeZone
}

func LoadLocation(value string) (*time.Location, string) {
	normalized := NormalizeTimeZone(value)
	if cached, ok := locationCache.Load(normalized); ok {
		return cached.(*time.Location), normalized
	}

	loc, err := time.LoadLocation(normalized)
	if err != nil {
		loc = time.FixedZone(DefaultTimeZone, 8*3600)
		normalized = DefaultTimeZone
	}
	locationCache.Store(normalized, loc)
	return loc, normalized
}

func Now(value string) time.Time {
	loc, _ := LoadLocation(value)
	return time.Now().In(loc)
}

func NormalizeUnixMillis(ts int64) int64 {
	switch {
	case ts <= 0:
		return 0
	case ts < 1_000_000_000_000:
		return ts * 1000
	default:
		return ts
	}
}

func TimeFromUnixMillis(ts int64, value string) time.Time {
	normalized := NormalizeUnixMillis(ts)
	if normalized <= 0 {
		return time.Time{}
	}
	loc, _ := LoadLocation(value)
	return time.UnixMilli(normalized).In(loc)
}

func TimeFromUnixSeconds(ts int64, value string) time.Time {
	if ts <= 0 {
		return time.Time{}
	}
	loc, _ := LoadLocation(value)
	return time.Unix(ts, 0).In(loc)
}

func FormatTime(t time.Time, layout string) string {
	if t.IsZero() {
		return ""
	}
	if strings.TrimSpace(layout) == "" {
		layout = defaultLayout
	}
	return t.Format(layout)
}

func FormatRelativeDuration(d time.Duration) string {
	if d < time.Minute {
		return "刚刚"
	}
	mins := int(d.Minutes()) % 60
	hrs := int(d.Hours()) % 24
	days := int(d.Hours()) / 24
	if days > 0 {
		if hrs == 0 && mins == 0 {
			return "约" + itoa(days) + "天前"
		}
		return "约" + itoa(days) + "天" + itoa(hrs) + "小时前"
	}
	if hrs > 0 {
		if mins == 0 {
			return "约" + itoa(hrs) + "小时前"
		}
		return "约" + itoa(hrs) + "小时" + itoa(mins) + "分钟前"
	}
	return "约" + itoa(mins) + "分钟前"
}

func itoa(v int) string {
	return strconv.Itoa(v)
}
