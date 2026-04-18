package handler

import (
	"strings"

	harukiConfig "haruki-cloud/config"
	"haruki-cloud/internal/pjsk/onebot11"
	"haruki-cloud/internal/pjsk/parser"
)

func isMySekaiRegionAllowed(cmd *parser.ResolvedCommand, region string) bool {
	region = strings.ToLower(strings.TrimSpace(region))
	if region != "cn" {
		return true
	}
	if cmd == nil {
		return false
	}
	for _, entry := range harukiConfig.Cfg.PJSK.AllowCNMySekai {
		if strings.EqualFold(entry.Platform, cmd.RequesterPlatform) &&
			entry.GroupID == cmd.RequesterGroupID {
			return true
		}
	}
	return false
}

func mySekaiRegionUnavailableMessage() onebot11.Message {
	return onebot11.Message{onebot11.Text("MySekai 功能在此区服暂未开放")}
}
