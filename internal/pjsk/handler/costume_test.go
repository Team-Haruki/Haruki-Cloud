package handler

import (
	"context"
	"encoding/json"
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

func TestCostumeHeadLookupKeepsNormalizedIDQuery(t *testing.T) {
	EnsureCommandHandlersRegistered()

	resolved, err := dispatchForTest(context.Background(), Event{
		Platform: "qq",
		Message: onebot11.Message{
			{Type: "text", Data: map[string]any{"text": "/jp查头饰 20 miku ln 颜色3"}},
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
	if query.AccessoryID != 20 || query.Character3DID != 23 || query.ColorID != 3 || query.ExpectedPartType != "head" {
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
