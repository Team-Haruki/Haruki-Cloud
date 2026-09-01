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
	"haruki-cloud/internal/testutil"
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
			testutil.Require(t, !(err != nil), "dispatch: %v", err)
			testutil.RequireArgs(t, !(resolved == nil), "expected command request, got nil")
			{

				testutil.Require(t, !(resolved.Module != parser.ModuleCostume), "unexpected resolved target: module=%v mode=%s", resolved.Module, resolved.Mode)
				testutil.Require(t, !(resolved.Mode != "costume-list"), "unexpected resolved target: module=%v mode=%s", resolved.Module, resolved.Mode)
			}
			testutil.Require(t, !(resolved.Region != tt.region), "region = %q, want %q", resolved.Region, tt.region)
			testutil.Require(t, !(resolved.Query != ""), "generic query parser must not receive a component name, got %q", resolved.Query)

			var query rendercostume.ListQuery
			{
				err := json.Unmarshal(resolved.Params, &query)
				testutil.Require(t, !(err != nil), "unmarshal costume list params: %v", err)
			}
			{

				testutil.Require(t, !(query.Query != ""), "unexpected costume name query: %+v", query)
				testutil.Require(t, !(query.Keyword != tt.keyword), "unexpected costume name query: %+v", query)
				testutil.Require(t, !(query.PartType != tt.partType), "unexpected costume name query: %+v", query)
				testutil.Require(t, !(query.Region != tt.region), "unexpected costume name query: %+v", query)
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
			testutil.Require(t, !(err != nil), "dispatch: %v", err)
			{
				testutil.Require(t, !(resolved == nil), "unexpected resolved target: %+v", resolved)
				testutil.Require(t, !(resolved.Module != parser.ModuleCostume), "unexpected resolved target: %+v", resolved)
				testutil.Require(t, !(resolved.Mode != "costume-detail"), "unexpected resolved target: %+v", resolved)
			}
			testutil.Require(t, !(resolved.Region != tt.region), "region = %q, want %q", resolved.Region, tt.region)

			var query rendercostume.Query
			{
				err := json.Unmarshal(resolved.Params, &query)
				testutil.Require(t, !(err != nil), "unmarshal costume detail params: %v", err)
			}
			{

				testutil.Require(t, !(query.Query != tt.name), "unexpected named costume detail query: %+v", query)
				testutil.Require(t, !(query.ExpectedPartType != tt.partType), "unexpected named costume detail query: %+v", query)
				testutil.Require(t, !(query.Character3DID != tt.role), "unexpected named costume detail query: %+v", query)
				testutil.Require(t, !(query.ColorID != 1), "unexpected named costume detail query: %+v", query)
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
	testutil.Require(t, !(err != nil), "dispatch: %v", err)
	testutil.RequireArgs(t, !(resolved == nil), "expected command request, got nil")
	{

		testutil.Require(t, !(resolved.Module != parser.ModuleCostume), "unexpected resolved target: module=%v mode=%s", resolved.Module, resolved.Mode)
		testutil.Require(t, !(resolved.Mode != "costume-detail"), "unexpected resolved target: module=%v mode=%s", resolved.Module, resolved.Mode)
	}

	var query rendercostume.Query
	{
		err := json.Unmarshal(resolved.Params, &query)
		testutil.Require(t, !(err != nil), "unmarshal costume detail params: %v", err)
	}
	{

		testutil.Require(t, !(query.AccessoryID != 2003001), "unexpected normalized head query: %+v", query)
		testutil.Require(t, !(query.Character3DID != 23), "unexpected normalized head query: %+v", query)
		testutil.Require(t, !(query.ColorID != 3), "unexpected normalized head query: %+v", query)
		testutil.Require(t, !(query.ExpectedPartType != "head"), "unexpected normalized head query: %+v", query)
	}

}

func TestCostumeHairLookupUsesRoleLocalHairID(t *testing.T) {
	EnsureCommandHandlersRegistered()

	resolved, err := dispatchForTest(context.Background(), Event{
		Platform: "qq",
		Message: onebot11.Message{
			{Type: "text", Data: map[string]any{"text": "/jp查发型 1 mnr"}},
		},
		UserId: "12345",
	})
	testutil.Require(t, !(err != nil), "dispatch: %v", err)
	{
		testutil.Require(t, !(resolved == nil), "unexpected resolved target: %+v", resolved)
		testutil.Require(t, !(resolved.Module != parser.ModuleCostume), "unexpected resolved target: %+v", resolved)
		testutil.Require(t, !(resolved.Mode != "costume-detail"), "unexpected resolved target: %+v", resolved)
	}

	var query rendercostume.Query
	{
		err := json.Unmarshal(resolved.Params, &query)
		testutil.Require(t, !(err != nil), "unmarshal costume detail params: %v", err)
	}
	{

		testutil.Require(t, !(query.HairID != 1), "unexpected normalized hair query: %+v", query)
		testutil.Require(t, !(query.Character3DID != 5), "unexpected normalized hair query: %+v", query)
		testutil.Require(t, !(query.ExpectedPartType != "hair"), "unexpected normalized hair query: %+v", query)
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
	testutil.Require(t, !(err != nil), "dispatch: %v", err)
	{
		testutil.Require(t, !(resolved == nil), "unexpected command request: %+v", resolved)
		testutil.Require(t, !(resolved.Mode != "costume-list"), "unexpected command request: %+v", resolved)
	}
	testutil.Require(t, !(resolved.Query != args), "list filter query = %q, want %q", resolved.Query, args)

	var query rendercostume.ListQuery
	{
		err := json.Unmarshal(resolved.Params, &query)
		testutil.Require(t, !(err != nil), "unmarshal costume list params: %v", err)
	}
	{

		testutil.Require(t, !(query.Query != args), "unexpected costume list filter query: %+v", query)
		testutil.Require(t, !(query.Keyword != ""), "unexpected costume list filter query: %+v", query)
		testutil.Require(t, !(query.PartType != "hair"), "unexpected costume list filter query: %+v", query)
	}

}

func TestLegacyAccessoryLookupRedirectsToCandidateList(t *testing.T) {
	detail := rendercostume.Query{Region: "jp", Character3DID: 2, AccessoryID: 2003}
	list, ok := legacyAccessoryListQuery(&rendercostume.LegacyAccessoryIDError{
		LegacyID:      2003,
		Character3DID: 2,
		AccessoryIDs:  []int{2003001, 2003017},
	}, detail)
	{
		testutil.Require(t, ok, "unexpected redirected accessory list query: ok=%v query=%+v", ok, list)
		testutil.Require(t, !(list.Region != "jp"), "unexpected redirected accessory list query: ok=%v query=%+v", ok, list)
		testutil.Require(t, !(list.PartType != "head"), "unexpected redirected accessory list query: ok=%v query=%+v", ok, list)
		testutil.Require(t, !(list.Character3DID != 2), "unexpected redirected accessory list query: ok=%v query=%+v", ok, list)
		testutil.Require(t, slices.Equal(list.AccessoryIDs, []int{2003001, 2003017}), "unexpected redirected accessory list query: ok=%v query=%+v", ok, list)
	}
	{

		_, ok := legacyAccessoryListQuery(errors.New("ordinary detail failure"), detail)
		testutil.RequireArgs(t, !(ok), "ordinary detail errors must not redirect to an accessory list")
	}

}
