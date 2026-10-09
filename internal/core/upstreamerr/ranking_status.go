package upstreamerr

import "strings"

// The tracker's event status ("statusDesc" of /sk/status) is free text. These
// fragments decide whether the tracker is healthy; they are written by the
// tracker, so TestRankingStatusContract pins them. A description matching
// neither list falls back to the numeric status (0 and 1 are healthy).
var (
	rankingStatusUnhealthy = []string{
		"error", "fail", "down", "maintenance", "timeout",
		"异常", "错误", "失败", "维护", "超时", "不可用", //copylint:ignore 查榜服务状态文本（上游约定），不展示
	}
	rankingStatusHealthy = []string{
		"ok", "normal", "healthy", "running", "success", "available", "active",
		"正常", "可用", "运行", "成功", //copylint:ignore 查榜服务状态文本（上游约定），不展示
	}
)

// RankingStatusHealthy reports whether the tracker's event status says the
// ranking data is being updated.
func RankingStatusHealthy(status int, description string) bool {
	desc := strings.ToLower(strings.TrimSpace(description))
	if desc != "" {
		if containsAny(desc, rankingStatusUnhealthy) {
			return false
		}
		if containsAny(desc, rankingStatusHealthy) {
			return true
		}
	}
	return status == 0 || status == 1
}

func containsAny(text string, fragments []string) bool {
	for _, fragment := range fragments {
		if strings.Contains(text, fragment) {
			return true
		}
	}
	return false
}
