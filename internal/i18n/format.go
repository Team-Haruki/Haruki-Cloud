package i18n

import (
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Verbatim shows value as is, e.g. an unknown region code or an unknown
// difficulty. It keeps call sites typed when a label has no catalog entry.
func Verbatim(value string) Message {
	return M("common.verbatim", Data{"Value": value})
}

var regionNames = map[string]Message{
	"jp": M("format.region.name.jp"),
	"cn": M("format.region.name.cn"),
	"tw": M("format.region.name.tw"),
	"kr": M("format.region.name.kr"),
	"en": M("format.region.name.en"),
}

// RegionCodes lists the region codes in display order (jp, cn, tw, kr, en).
var RegionCodes = []string{"jp", "cn", "tw", "kr", "en"}

// RegionLabel is the display name of a region, e.g. "日服(JP)". region is a
// region code in any case; an unknown code is shown upper-cased.
func RegionLabel(region string) Message {
	code := strings.ToLower(strings.TrimSpace(region))
	name, ok := regionNames[code]
	if !ok {
		return Verbatim(strings.ToUpper(code))
	}
	return M("format.region.label", Data{"Name": name, "Code": strings.ToUpper(code)})
}

// MaskUID hides the middle of a game UID ("123***789") unless visible is
// true. UIDs of six characters or fewer are shown as is.
func MaskUID(uid string, visible bool) string {
	uid = strings.TrimSpace(uid)
	if visible || len(uid) <= 6 {
		return uid
	}
	return uid[:3] + strings.Repeat("*", len(uid)-6) + uid[len(uid)-3:]
}

// AccountLabel is one bound game account, e.g. "[日服(JP)] 123***789".
func AccountLabel(region, uid string, visible bool) Message {
	return M("format.account.label", Data{"Region": RegionLabel(region), "UID": MaskUID(uid, visible)})
}

var defaultLocation = time.FixedZone("UTC+8", 8*3600)

// FormatUserTime shows t in the requester's time zone with its UTC offset,
// e.g. "2026-10-09 14:05 (UTC+8)". A nil loc means Asia/Shanghai (UTC+8);
// pass displaytime.LoadLocation(tz) or the request's location.
func FormatUserTime(t time.Time, loc *time.Location) Message {
	if t.IsZero() {
		return M("format.time.unknown")
	}
	if loc == nil {
		loc = defaultLocation
	}
	local := t.In(loc)
	return M("format.time.datetime", Data{
		"DateTime": local.Format("2006-01-02 15:04"),
		"Offset":   utcOffset(local),
	})
}

func utcOffset(t time.Time) string {
	_, seconds := t.Zone()
	sign := "+"
	if seconds < 0 {
		sign = "-"
		seconds = -seconds
	}
	hours, minutes := seconds/3600, seconds%3600/60
	if minutes == 0 {
		return sign + strconv.Itoa(hours)
	}
	return sign + strconv.Itoa(hours) + ":" + twoDigits(minutes)
}

func twoDigits(v int) string {
	if v < 10 {
		return "0" + strconv.Itoa(v)
	}
	return strconv.Itoa(v)
}

// FormatDuration shows a duration rounded to seconds: "45秒", "2分03秒",
// "1小时02分03秒". Negative durations are shown as zero.
func FormatDuration(d time.Duration) Message {
	if d < 0 {
		d = 0
	}
	total := int64(d.Round(time.Second) / time.Second)
	hours, minutes, seconds := total/3600, total%3600/60, total%60
	switch {
	case hours > 0:
		return M("format.duration.hours", Data{
			"Hours":   hours,
			"Minutes": twoDigits(int(minutes)),
			"Seconds": twoDigits(int(seconds)),
		})
	case minutes > 0:
		return M("format.duration.minutes", Data{
			"Minutes": minutes,
			"Seconds": twoDigits(int(seconds)),
		})
	default:
		return M("format.duration.seconds", Data{"Seconds": seconds})
	}
}

// Thousands groups the digits of integers with five or more digits
// ("12,345", "1,234,567"); shorter numbers stay ungrouped ("1234").
func Thousands(n int64) string {
	digits := strconv.FormatInt(n, 10)
	sign := ""
	if strings.HasPrefix(digits, "-") {
		sign, digits = "-", digits[1:]
	}
	if len(digits) < 5 {
		return sign + digits
	}
	var b strings.Builder
	head := len(digits) % 3
	if head > 0 {
		b.WriteString(digits[:head])
	}
	for i := head; i < len(digits); i += 3 {
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(digits[i : i+3])
	}
	return sign + b.String()
}

// Wan shows large counts in units of 万 with up to two decimals ("12.35万",
// "300万"); values below 10000 are shown as plain integers.
func Wan(n float64) Message {
	if math.Abs(n) < 10000 {
		return Verbatim(strconv.FormatInt(int64(math.Round(n)), 10))
	}
	return M("format.wan", Data{"Value": trimDecimals(n/10000, 2)})
}

// Percent shows a value that is already in percent units with one decimal
// and without a trailing ".0": 12.34 -> "12.3%", 50 -> "50%".
func Percent(value float64) string {
	return trimDecimals(value, 1) + "%"
}

// PercentN is Percent with an explicit number of decimals.
func PercentN(value float64, decimals int) string {
	return trimDecimals(value, decimals) + "%"
}

func trimDecimals(value float64, decimals int) string {
	if decimals < 0 {
		decimals = 0
	}
	text := strconv.FormatFloat(value, 'f', decimals, 64)
	if strings.Contains(text, ".") {
		text = strings.TrimRight(strings.TrimRight(text, "0"), ".")
	}
	if text == "-0" {
		return "0"
	}
	return text
}

// PageLabel is the current page of a paged list, e.g. "第 1/3 页".
func PageLabel(page, total int) Message {
	return M("format.page", Data{"Page": page, "Total": total})
}

var difficultyLabels = map[string]Message{
	"easy":   M("format.difficulty.easy"),
	"normal": M("format.difficulty.normal"),
	"hard":   M("format.difficulty.hard"),
	"expert": M("format.difficulty.expert"),
	"master": M("format.difficulty.master"),
	"append": M("format.difficulty.append"),
}

// DifficultyLabel is the display name of a chart difficulty key
// ("expert" -> "EXPERT"). An unknown key is shown upper-cased.
func DifficultyLabel(difficulty string) Message {
	key := strings.ToLower(strings.TrimSpace(difficulty))
	if label, ok := difficultyLabels[key]; ok {
		return label
	}
	return Verbatim(strings.ToUpper(key))
}

var liveTypeLabels = map[string]Message{
	"solo":      M("format.live_type.solo"),
	"multi":     M("format.live_type.multi"),
	"auto":      M("format.live_type.auto"),
	"challenge": M("format.live_type.challenge"),
}

// LiveTypeLabel is the display name of a live type key (solo, multi, auto,
// challenge). An unknown key is shown as is.
func LiveTypeLabel(liveType string) Message {
	key := strings.ToLower(strings.TrimSpace(liveType))
	if label, ok := liveTypeLabels[key]; ok {
		return label
	}
	return Verbatim(liveType)
}

// QueryEchoLimit is the number of runes of a user's query echoed back in a
// message; longer queries are cut with "……".
const QueryEchoLimit = 30

// EchoQuery trims a user's query for display in a message.
func EchoQuery(query string) string {
	query = strings.Join(strings.Fields(query), " ")
	if utf8.RuneCountInString(query) <= QueryEchoLimit {
		return query
	}
	return string([]rune(query)[:QueryEchoLimit]) + "……"
}
