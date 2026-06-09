package handler

import (
	"strings"

	harukiConfig "haruki-cloud/config"
	"haruki-cloud/internal/onebot11"
)

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

func mySekaiRegionUnavailableMessage() onebot11.Message {
	return onebot11.Message{onebot11.Text("MySekai 功能在此区服暂未开放")}
}
