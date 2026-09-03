package handler

import (
	"fmt"
	"log/slog"
	"strings"

	harukiConfig "haruki-cloud/config"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/accountdata"
)

const cnMySekaiNeverOpensNotice = "国服 MySekai 功能永不开启，请勿再尝试。"

func isMySekaiRegionAllowed(cmd *CommandRequest, region string) bool {
	region = strings.ToLower(strings.TrimSpace(region))
	if region != "cn" {
		return true
	}
	if cmd == nil {
		return false
	}
	for _, entry := range harukiConfig.Cfg.PJSK.AllowCNMySekai {
		if strings.EqualFold(entry.Platform, cmd.RequesterPlatform) &&
			entry.GroupID == cmd.RequesterGroupID &&
			(strings.TrimSpace(entry.BotID) == "" || entry.BotID == cmd.RequesterBotID) {
			return true
		}
	}
	return false
}

func isMySekaiDeckRegionAllowed(cmd *CommandRequest, region string) bool {
	region = strings.ToLower(strings.TrimSpace(region))
	if region != "cn" {
		return true
	}
	if cmd != nil && strings.EqualFold(strings.TrimSpace(cmd.Mode), "deck-mysekai") {
		return true
	}
	return isMySekaiRegionAllowed(cmd, region)
}

func isMySekaiRegionAllowedForMode(cmd *CommandRequest, region string) bool {
	region = strings.ToLower(strings.TrimSpace(region))
	if region != "cn" {
		return true
	}
	if cmd != nil && strings.EqualFold(strings.TrimSpace(cmd.Mode), "mysekai-housing-sk") {
		return true
	}
	return isMySekaiRegionAllowed(cmd, region)
}

// rejectCNMySekai answers a CN MySekai request that failed the whitelist. The
// attempt is counted against the requester; the third one within the current
// cycle turns into a temporary global ban (see BanService.RecordCNMySekaiAttempt).
// Tracking failures never hide the notice itself.
func rejectCNMySekai(rc *RequestContext) (onebot11.Message, error) {
	if rc == nil || rc.App == nil || rc.Cmd == nil {
		return onebot11.Message{onebot11.Text(cnMySekaiNeverOpensNotice)}, nil
	}
	attempt, err := rc.App.BanChecker.RecordCNMySekaiAttempt(
		rc.Ctx,
		rc.Cmd.RequesterPlatform,
		rc.Cmd.RequesterUserID,
		harukiConfig.Cfg.PJSK.CNMySekaiBanDuration,
	)
	if err != nil {
		slog.WarnContext(rc.Ctx, "cn mysekai attempt tracking failed",
			slog.String("platform", rc.Cmd.RequesterPlatform),
			slog.String("user_id", rc.Cmd.RequesterUserID),
			slog.String("error", err.Error()),
		)
		return onebot11.Message{onebot11.Text(cnMySekaiNeverOpensNotice)}, nil
	}
	return onebot11.Message{onebot11.Text(cnMySekaiRejectionText(attempt))}, nil
}

func cnMySekaiRejectionText(attempt accountdata.CNMySekaiAttempt) string {
	if attempt.Attempts <= 0 {
		return cnMySekaiNeverOpensNotice
	}
	text := fmt.Sprintf("%s（%d/%d）", cnMySekaiNeverOpensNotice, attempt.Attempts, attempt.Threshold)
	if attempt.Banned {
		text += "\n已被短暂封禁至 " + attempt.ExpiresAt.Format("2006-01-02 15:04:05 MST")
	}
	return text
}
