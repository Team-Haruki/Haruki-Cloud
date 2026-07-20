package handler

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/parser"
	rendercostume "haruki-cloud/internal/pjsk/render/costume"
)

func TestCostumeNameLookupCommandsKeepMasterNameAsKeyword(t *testing.T) {
	EnsureCommandHandlersRegistered()

	tests := []struct {
		command  string
		region   string
		keyword  string
		partType string
	}{
		{command: "/en查服装 MIKU MIKU POP!", region: "en", keyword: "MIKU MIKU POP!", partType: "body"},
		{command: "/tw查头饰 快樂小雞洋裝", region: "tw", keyword: "快樂小雞洋裝", partType: "head"},
		{command: "/en查发型 Candy Wheel", region: "en", keyword: "Candy Wheel", partType: "hair"},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			resolved, err := dispatchForTest(context.Background(), Event{
				Platform: "qq",
				Message: onebot11.Message{
					{Type: "text", Data: map[string]any{"text": tt.command}},
				},
				UserId: "12345",
			})
			if err != nil {
				t.Fatalf("dispatch: %v", err)
			}
			if resolved == nil {
				t.Fatal("expected command request, got nil")
			}
			if resolved.Module != parser.ModuleCostume || resolved.Mode != "costume-list" {
				t.Fatalf("unexpected resolved target: module=%v mode=%s", resolved.Module, resolved.Mode)
			}
			if resolved.Region != tt.region {
				t.Fatalf("region = %q, want %q", resolved.Region, tt.region)
			}
			if resolved.Query != "" {
				t.Fatalf("generic query parser must not receive a component name, got %q", resolved.Query)
			}

			var query rendercostume.ListQuery
			if err := json.Unmarshal(resolved.Params, &query); err != nil {
				t.Fatalf("unmarshal costume list params: %v", err)
			}
			if query.Query != "" || query.Keyword != tt.keyword || query.PartType != tt.partType || query.Region != tt.region {
				t.Fatalf("unexpected costume name query: %+v", query)
			}
		})
	}
}

func TestCostumeNameLookupWithRoleUsesDetail(t *testing.T) {
	EnsureCommandHandlersRegistered()

	tests := []struct {
		command  string
		region   string
		name     string
		partType string
		role     int
	}{
		{command: "/en查服装 MIKU MIKU POP! 角色23", region: "en", name: "MIKU MIKU POP!", partType: "body", role: 23},
		{command: "/en查服装 角色1 Christmas Style 2021 (F)", region: "en", name: "Christmas Style 2021 (F)", partType: "body", role: 1},
		{command: "/tw查头饰 快樂小雞洋裝 角色5", region: "tw", name: "快樂小雞洋裝", partType: "head", role: 5},
		{command: "/en查发型 Candy Wheel 角色5", region: "en", name: "Candy Wheel", partType: "hair", role: 5},
		{command: "/jp查服装 Alice Good Night mnr", region: "jp", name: "Alice Good Night", partType: "body", role: 5},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			resolved, err := dispatchForTest(context.Background(), Event{
				Platform: "qq",
				Message: onebot11.Message{
					{Type: "text", Data: map[string]any{"text": tt.command}},
				},
				UserId: "12345",
			})
			if err != nil {
				t.Fatalf("dispatch: %v", err)
			}
			if resolved == nil || resolved.Module != parser.ModuleCostume || resolved.Mode != "costume-detail" {
				t.Fatalf("unexpected resolved target: %+v", resolved)
			}
			if resolved.Region != tt.region {
				t.Fatalf("region = %q, want %q", resolved.Region, tt.region)
			}

			var query rendercostume.Query
			if err := json.Unmarshal(resolved.Params, &query); err != nil {
				t.Fatalf("unmarshal costume detail params: %v", err)
			}
			if query.Query != tt.name || query.ExpectedPartType != tt.partType || query.Character3DID != tt.role || query.ColorID != 1 {
				t.Fatalf("unexpected named costume detail query: %+v", query)
			}
		})
	}
}

func TestCostumeHeadLookupKeepsAccessoryIDQuery(t *testing.T) {
	EnsureCommandHandlersRegistered()

	resolved, err := dispatchForTest(context.Background(), Event{
		Platform: "qq",
		Message: onebot11.Message{
			{Type: "text", Data: map[string]any{"text": "/jp查头饰 2003001 miku ln 颜色3"}},
		},
		UserId: "12345",
	})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleCostume || resolved.Mode != "costume-detail" {
		t.Fatalf("unexpected resolved target: module=%v mode=%s", resolved.Module, resolved.Mode)
	}

	var query rendercostume.Query
	if err := json.Unmarshal(resolved.Params, &query); err != nil {
		t.Fatalf("unmarshal costume detail params: %v", err)
	}
	if query.AccessoryID != 2003001 || query.Character3DID != 23 || query.ColorID != 3 || query.ExpectedPartType != "head" {
		t.Fatalf("unexpected normalized head query: %+v", query)
	}
}

func TestCostumeListCommandKeepsFilterSyntax(t *testing.T) {
	EnsureCommandHandlersRegistered()

	const args = "miku ln p2"
	resolved, err := dispatchForTest(context.Background(), Event{
		Platform: "qq",
		Message: onebot11.Message{
			{Type: "text", Data: map[string]any{"text": "/jp查发型列表 " + args}},
		},
		UserId: "12345",
	})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if resolved == nil || resolved.Mode != "costume-list" {
		t.Fatalf("unexpected command request: %+v", resolved)
	}
	if resolved.Query != args {
		t.Fatalf("list filter query = %q, want %q", resolved.Query, args)
	}

	var query rendercostume.ListQuery
	if err := json.Unmarshal(resolved.Params, &query); err != nil {
		t.Fatalf("unmarshal costume list params: %v", err)
	}
	if query.Query != args || query.Keyword != "" || query.PartType != "hair" {
		t.Fatalf("unexpected costume list filter query: %+v", query)
	}
}

func TestLegacyAccessoryLookupRedirectsToCandidateList(t *testing.T) {
	detail := rendercostume.Query{Region: "jp", Character3DID: 2, AccessoryID: 2003}
	list, ok := legacyAccessoryListQuery(&rendercostume.LegacyAccessoryIDError{
		LegacyID:      2003,
		Character3DID: 2,
		AccessoryIDs:  []int{2003001, 2003017},
	}, detail)
	if !ok || list.Region != "jp" || list.PartType != "head" || list.Character3DID != 2 || !slices.Equal(list.AccessoryIDs, []int{2003001, 2003017}) {
		t.Fatalf("unexpected redirected accessory list query: ok=%v query=%+v", ok, list)
	}
	if _, ok := legacyAccessoryListQuery(errors.New("ordinary detail failure"), detail); ok {
		t.Fatal("ordinary detail errors must not redirect to an accessory list")
	}
}
