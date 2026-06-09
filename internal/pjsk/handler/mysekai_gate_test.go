package handler

import (
	"testing"

	harukiConfig "haruki-cloud/config"
)

func TestIsMySekaiRegionAllowedMatchesWhitelistedBot(t *testing.T) {
	original := harukiConfig.Cfg.PJSK.AllowCNMySekai
	harukiConfig.Cfg.PJSK.AllowCNMySekai = []harukiConfig.MySekaiCNWhitelistEntry{
		{Platform: "qq", GroupID: "123456", BotID: "11451419"},
	}
	t.Cleanup(func() {
		harukiConfig.Cfg.PJSK.AllowCNMySekai = original
	})

	allowed := isMySekaiRegionAllowed(&CommandRequest{
		RequesterPlatform: "qq",
		RequesterGroupID:  "123456",
		RequesterBotID:    "11451419",
	}, "cn")
	if !allowed {
		t.Fatal("expected whitelisted bot to be allowed")
	}
}

func TestIsMySekaiRegionAllowedRejectsDifferentBot(t *testing.T) {
	original := harukiConfig.Cfg.PJSK.AllowCNMySekai
	harukiConfig.Cfg.PJSK.AllowCNMySekai = []harukiConfig.MySekaiCNWhitelistEntry{
		{Platform: "qq", GroupID: "123456", BotID: "11451419"},
	}
	t.Cleanup(func() {
		harukiConfig.Cfg.PJSK.AllowCNMySekai = original
	})

	allowed := isMySekaiRegionAllowed(&CommandRequest{
		RequesterPlatform: "qq",
		RequesterGroupID:  "123456",
		RequesterBotID:    "1919810",
	}, "cn")
	if allowed {
		t.Fatal("expected different bot to be rejected")
	}
}

func TestIsMySekaiRegionAllowedKeepsLegacyGroupWhitelist(t *testing.T) {
	original := harukiConfig.Cfg.PJSK.AllowCNMySekai
	harukiConfig.Cfg.PJSK.AllowCNMySekai = []harukiConfig.MySekaiCNWhitelistEntry{
		{Platform: "qq", GroupID: "123456"},
	}
	t.Cleanup(func() {
		harukiConfig.Cfg.PJSK.AllowCNMySekai = original
	})

	allowed := isMySekaiRegionAllowed(&CommandRequest{
		RequesterPlatform: "qq",
		RequesterGroupID:  "123456",
		RequesterBotID:    "1919810",
	}, "cn")
	if !allowed {
		t.Fatal("expected legacy whitelist entry without bot id to remain valid")
	}
}

func TestIsMySekaiDeckRegionAllowedAlwaysAllowsCNDeckMode(t *testing.T) {
	original := harukiConfig.Cfg.PJSK.AllowCNMySekai
	harukiConfig.Cfg.PJSK.AllowCNMySekai = nil
	t.Cleanup(func() {
		harukiConfig.Cfg.PJSK.AllowCNMySekai = original
	})

	allowed := isMySekaiDeckRegionAllowed(&CommandRequest{
		Mode:              "deck-mysekai",
		RequesterPlatform: "qq",
		RequesterGroupID:  "123456",
		RequesterBotID:    "1919810",
	}, "cn")
	if !allowed {
		t.Fatal("expected deck-mysekai to bypass CN mysekai whitelist")
	}
}

func TestIsMySekaiRegionAllowedForModeAllowsCNHousingSK(t *testing.T) {
	original := harukiConfig.Cfg.PJSK.AllowCNMySekai
	harukiConfig.Cfg.PJSK.AllowCNMySekai = nil
	t.Cleanup(func() {
		harukiConfig.Cfg.PJSK.AllowCNMySekai = original
	})

	allowed := isMySekaiRegionAllowedForMode(&CommandRequest{
		Mode:              "mysekai-housing-sk",
		RequesterPlatform: "qq",
		RequesterGroupID:  "123456",
		RequesterBotID:    "1919810",
	}, "cn")
	if !allowed {
		t.Fatal("expected mysekai-housing-sk to bypass CN mysekai whitelist")
	}
}

func TestIsMySekaiRegionAllowedForModeStillRejectsOtherCNMySekai(t *testing.T) {
	original := harukiConfig.Cfg.PJSK.AllowCNMySekai
	harukiConfig.Cfg.PJSK.AllowCNMySekai = nil
	t.Cleanup(func() {
		harukiConfig.Cfg.PJSK.AllowCNMySekai = original
	})

	allowed := isMySekaiRegionAllowedForMode(&CommandRequest{
		Mode:              "mysekai-resource",
		RequesterPlatform: "qq",
		RequesterGroupID:  "123456",
		RequesterBotID:    "1919810",
	}, "cn")
	if allowed {
		t.Fatal("expected other CN mysekai modes to keep using the whitelist")
	}
}
