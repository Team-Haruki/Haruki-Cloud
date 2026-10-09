package i18n

import "time"

// UploadAge is how long ago data was uploaded, rounded down to minutes:
// "刚刚", "5分钟前", "3小时20分钟前", "2天3小时前". A negative duration (clock
// skew) counts as "刚刚".
func UploadAge(d time.Duration) Message {
	if d < time.Minute {
		return M("profile.data_status.age.just_now")
	}
	totalMinutes := int64(d / time.Minute)
	days, hours, minutes := totalMinutes/(24*60), totalMinutes%(24*60)/60, totalMinutes%60
	switch {
	case days > 0 && hours == 0:
		return M("profile.data_status.age.days", Data{"Days": days})
	case days > 0:
		return M("profile.data_status.age.days_hours", Data{"Days": days, "Hours": hours})
	case hours > 0 && minutes == 0:
		return M("profile.data_status.age.hours", Data{"Hours": hours})
	case hours > 0:
		return M("profile.data_status.age.hours_minutes", Data{"Hours": hours, "Minutes": minutes})
	default:
		return M("profile.data_status.age.minutes", Data{"Minutes": minutes})
	}
}

// UploadedLine is the "上次上传：…" line shared by the data status replies
// (/sud, /msd) and the expired MySekai data reply: the upload time in the
// requester's zone (FormatUserTime) and how long ago it was (UploadAge),
// measured from now. A zero uploadedAt gives the "unknown" line.
func UploadedLine(uploadedAt, now time.Time, loc *time.Location) Message {
	if uploadedAt.IsZero() {
		return M("profile.data_status.uploaded_unknown")
	}
	return M("profile.data_status.uploaded", Data{
		"Time": FormatUserTime(uploadedAt, loc),
		"Age":  UploadAge(now.Sub(uploadedAt)),
	})
}
