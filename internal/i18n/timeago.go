package i18n

import "time"

// TimeAgo is how long ago something happened, rounded down to minutes:
// "刚刚", "5分钟前", "3小时20分钟前", "2天3小时前". A negative duration (clock
// skew) counts as "刚刚".
func TimeAgo(d time.Duration) Message {
	if d < time.Minute {
		return M("format.ago.just_now")
	}
	totalMinutes := int64(d / time.Minute)
	days, hours, minutes := totalMinutes/(24*60), totalMinutes%(24*60)/60, totalMinutes%60
	switch {
	case days > 0 && hours == 0:
		return M("format.ago.days", Data{"Days": days})
	case days > 0:
		return M("format.ago.days_hours", Data{"Days": days, "Hours": hours})
	case hours > 0 && minutes == 0:
		return M("format.ago.hours", Data{"Hours": hours})
	case hours > 0:
		return M("format.ago.hours_minutes", Data{"Hours": hours, "Minutes": minutes})
	default:
		return M("format.ago.minutes", Data{"Minutes": minutes})
	}
}

// UploadedLine is the "上次上传：…" line shared by the data status replies
// (/sud, /msd) and the expired MySekai data reply: the upload time in the
// requester's zone (FormatUserTime) and how long ago it was (TimeAgo),
// measured from now. A zero uploadedAt gives the "unknown" line.
func UploadedLine(uploadedAt, now time.Time, loc *time.Location) Message {
	if uploadedAt.IsZero() {
		return M("profile.data_status.uploaded_unknown")
	}
	return M("profile.data_status.uploaded", Data{
		"Time": FormatUserTime(uploadedAt, loc),
		"Age":  TimeAgo(now.Sub(uploadedAt)),
	})
}
