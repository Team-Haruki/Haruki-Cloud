package subscription

import (
	"slices"
	"strings"
	"testing"
)

func TestParseBirthdayMonitorCommandDefaultsToDiamond(t *testing.T) {
	cmd, err := ParseBirthdayMonitorCommand("/烤森生日监听")
	if err != nil {
		t.Fatalf("ParseBirthdayMonitorCommand returned error: %v", err)
	}
	if cmd.Cancel {
		t.Fatalf("expected monitor command")
	}
	if cmd.DurationMinutes != DefaultBirthdayMonitorMinutes {
		t.Fatalf("duration = %d, want %d", cmd.DurationMinutes, DefaultBirthdayMonitorMinutes)
	}
	if !slices.Equal(cmd.Materials, []string{"diamond"}) {
		t.Fatalf("materials = %+v, want [diamond]", cmd.Materials)
	}
}

func TestParseBirthdayMonitorCommandSupportsSelectorDurationAndMaterials(t *testing.T) {
	cmd, err := ParseBirthdayMonitorCommand("/ms生日监听 u2 120 夕桐 四叶草")
	if err != nil {
		t.Fatalf("ParseBirthdayMonitorCommand returned error: %v", err)
	}
	if cmd.Selector != "u2" {
		t.Fatalf("selector = %q, want u2", cmd.Selector)
	}
	if cmd.DurationMinutes != 120 {
		t.Fatalf("duration = %d, want 120", cmd.DurationMinutes)
	}
	if !slices.Equal(cmd.Materials, []string{"yuugiri", "clover"}) {
		t.Fatalf("materials = %+v, want [yuugiri clover]", cmd.Materials)
	}
}

func TestParseBirthdayMonitorCommandRejectsAllMaterialsDisabled(t *testing.T) {
	_, err := ParseBirthdayMonitorCommand("/烤森生日监听 钻石关闭")
	if err == nil || !strings.Contains(err.Error(), "至少需要开启一种监听材料") {
		t.Fatalf("error = %v, want all-disabled error", err)
	}
}

func TestParseBirthdayMonitorCommandRejectsDurationOverLimit(t *testing.T) {
	_, err := ParseBirthdayMonitorCommand("/烤森生日监听 121")
	if err == nil || !strings.Contains(err.Error(), "监听时长不能超过 120 分钟") {
		t.Fatalf("error = %v, want duration limit error", err)
	}
}

func TestParseBirthdayMonitorCancelCommand(t *testing.T) {
	cmd, err := ParseBirthdayMonitorCommand("/mysekai birthday unmonitor u1")
	if err != nil {
		t.Fatalf("ParseBirthdayMonitorCommand returned error: %v", err)
	}
	if !cmd.Cancel {
		t.Fatalf("expected cancel command")
	}
	if cmd.Selector != "u1" {
		t.Fatalf("selector = %q, want u1", cmd.Selector)
	}
}
