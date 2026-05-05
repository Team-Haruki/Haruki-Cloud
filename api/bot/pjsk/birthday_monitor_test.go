package pjsk

import (
	"testing"

	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/subscription"
)

func TestBirthdayMonitorCommandTextPrependsMatchedCommandForArgumentOnlyMessage(t *testing.T) {
	req := BotCommandRequest{
		MatchedCommand: "/烤森生日监听",
		Message:        onebot11.Message{onebot11.Text("u2 钻石")},
	}
	text := birthdayMonitorCommandText(req)
	cmd, err := subscription.ParseBirthdayMonitorCommand(text)
	if err != nil {
		t.Fatalf("ParseBirthdayMonitorCommand(%q) returned error: %v", text, err)
	}
	if cmd.Selector != "u2" {
		t.Fatalf("selector = %q, want u2", cmd.Selector)
	}
}

func TestBirthdayMonitorCommandTextSupportsRegionPrefixedMatchedCommand(t *testing.T) {
	req := BotCommandRequest{
		MatchedCommand: "/jp烤森生日监听",
		Message:        onebot11.Message{onebot11.Text("钻石 10")},
	}
	text := birthdayMonitorCommandText(req)
	cmd, err := subscription.ParseBirthdayMonitorCommand(text)
	if err != nil {
		t.Fatalf("ParseBirthdayMonitorCommand(%q) returned error: %v", text, err)
	}
	if !cmd.RegionExplicit || cmd.Region != "jp" {
		t.Fatalf("region = %q explicit=%t, want jp explicit", cmd.Region, cmd.RegionExplicit)
	}
	if cmd.DurationMinutes != 10 {
		t.Fatalf("duration = %d, want 10", cmd.DurationMinutes)
	}
}
