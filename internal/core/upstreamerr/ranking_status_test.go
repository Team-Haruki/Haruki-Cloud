package upstreamerr

import "testing"

// TestRankingStatusContract pins the tracker status descriptions Cloud reads
// as healthy or unhealthy (docs/sk-tracker-cloud-contract.cn.md, /sk/status).
func TestRankingStatusContract(t *testing.T) {
	cases := []struct {
		status int
		desc   string
		want   bool
	}{
		{1, "running", true},
		{1, "healthy", true},
		{2, "OK", true},
		{1, "正常", true},
		{1, "sekai api timeout", false},
		{1, "maintenance", false},
		{1, "服务异常", false},
		{1, "不可用", false},
		{0, "", true},
		{1, "", true},
		{2, "", false},
		{2, "unrecognized", false},
		{1, "unrecognized", true},
	}
	for _, tc := range cases {
		if got := RankingStatusHealthy(tc.status, tc.desc); got != tc.want {
			t.Errorf("RankingStatusHealthy(%d, %q) = %v, want %v", tc.status, tc.desc, got, tc.want)
		}
	}
}
