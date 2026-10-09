package i18n

import (
	"testing"
	"time"
)

func TestUploadAgeZhCN(t *testing.T) {
	cases := []struct {
		age  time.Duration
		want string
	}{
		{-time.Minute, "刚刚"},
		{59 * time.Second, "刚刚"},
		{5*time.Minute + 59*time.Second, "5分钟前"},
		{3 * time.Hour, "3小时前"},
		{3*time.Hour + 20*time.Minute, "3小时20分钟前"},
		{48 * time.Hour, "2天前"},
		{51*time.Hour + 59*time.Minute, "2天3小时前"},
	}
	for _, tc := range cases {
		if got := UploadAge(tc.age).String(); got != tc.want {
			t.Errorf("UploadAge(%v) = %q, want %q", tc.age, got, tc.want)
		}
	}
}

func TestUploadedLineZhCN(t *testing.T) {
	uploaded := time.Date(2026, 10, 9, 6, 5, 0, 0, time.UTC)
	now := uploaded.Add(3*time.Hour + 20*time.Minute)
	tokyo := time.FixedZone("UTC+9", 9*3600)
	if got := UploadedLine(uploaded, now, tokyo).String(); got != "上次上传：2026-10-09 15:05 (UTC+9)（3小时20分钟前）" {
		t.Errorf("UploadedLine = %q", got)
	}
	if got := UploadedLine(uploaded, now, nil).String(); got != "上次上传：2026-10-09 14:05 (UTC+8)（3小时20分钟前）" {
		t.Errorf("UploadedLine (default zone) = %q", got)
	}
	if got := UploadedLine(time.Time{}, now, nil); got.ID != "profile.data_status.uploaded_unknown" {
		t.Errorf("UploadedLine(zero) = %+v", got)
	}
}

// TestDataStatusRepliesShareOneStructure locks decision K: /sud, /msd and the
// expired MySekai reply name the account and data the same way and carry the
// same upload line.
func TestDataStatusRepliesShareOneStructure(t *testing.T) {
	uploaded := time.Date(2026, 10, 9, 6, 5, 0, 0, time.UTC)
	data := Data{
		"Account":  AccountLabel("jp", "123456789", false),
		"Uploaded": UploadedLine(uploaded, uploaded.Add(3*time.Hour), nil),
	}
	want := map[string]string{
		"profile.data_status.suite":   "[日服(JP)] 123***789 的抓包数据（Suite）\n上次上传：2026-10-09 14:05 (UTC+8)（3小时前）",
		"profile.data_status.mysekai": "[日服(JP)] 123***789 的烤森（MySekai）数据\n上次上传：2026-10-09 14:05 (UTC+8)（3小时前）",
		"profile.data_status.mysekai_expired": "[日服(JP)] 123***789 的烤森（MySekai）数据已过期\n上次上传：2026-10-09 14:05 (UTC+8)（3小时前）\n" +
			"要查看最新数据，请在 Haruki 工具箱重新上传烤森数据\n要继续查看已过期的数据，请在指令后加上“force”",
	}
	for id, text := range want {
		if got := T(id, data); got != text {
			t.Errorf("%s =\n%s\nwant\n%s", id, got, text)
		}
	}
}
