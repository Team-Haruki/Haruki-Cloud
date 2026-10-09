package handler

import (
	"log/slog"
	"strings"

	harukiConfig "haruki-cloud/config"
	"haruki-cloud/internal/i18n"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/accountdata"
)

// cnMySekaiNotice is the reply to a blocked CN MySekai command.
func cnMySekaiNotice() string {
	return i18n.T("mysekai.cn_gate.notice", i18n.Data{"Region": i18n.RegionLabel("cn")})
}

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

// rejectCNMySekai shares the requester's three warnings across blocked CN
// MySekai commands. Tracking failures still return the notice.
func rejectCNMySekai(rc *RequestContext) (onebot11.Message, error) {
	if rc == nil || rc.App == nil || rc.Cmd == nil {
		return onebot11.Message{onebot11.Text(cnMySekaiNotice())}, nil
	}
	attempt, err := rc.App.BanChecker.RecordCNMySekaiAttempt(
		rc.Ctx,
		rc.Cmd.RequesterPlatform,
		rc.Cmd.RequesterUserID,
	)
	if err != nil {
		slog.WarnContext(rc.Ctx, "cn mysekai attempt tracking failed",
			slog.String("platform", rc.Cmd.RequesterPlatform),
			slog.String("user_id", rc.Cmd.RequesterUserID),
			slog.String("error", err.Error()),
		)
		return onebot11.Message{onebot11.Text(cnMySekaiNotice())}, nil
	}
	if attempt.Silenced {
		return onebot11.Message{}, nil
	}
	return onebot11.Message{onebot11.Text(cnMySekaiRejectionText(attempt))}, nil
}

func cnMySekaiRejectionText(attempt accountdata.CNMySekaiAttempt) string {
	if attempt.Attempts <= 0 {
		return cnMySekaiNotice()
	}
	region := i18n.RegionLabel("cn")
	if attempt.Attempts >= attempt.Threshold {
		return i18n.T("mysekai.cn_gate.notice_last", i18n.Data{"Region": region, "Attempts": attempt.Attempts, "Threshold": attempt.Threshold})
	}
	return i18n.T("mysekai.cn_gate.notice_counted", i18n.Data{"Region": region, "Attempts": attempt.Attempts, "Threshold": attempt.Threshold})
}
