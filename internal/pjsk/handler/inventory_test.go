package handler

import (
	"context"
	"errors"
	"strings"
	"testing"

	json "haruki-cloud/internal/jsonutil"

	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/parser"
	renderinventory "haruki-cloud/internal/pjsk/render/inventory"
)

func TestInventoryListHandleParsesFilterAndSelector(t *testing.T) {
	h := sekaiHandlers{}.InventoryListHandle()
	h.Regions = AllRegions

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "12345",
		TriggerCmd: "/持有物",
		ArgText:    "u2 火罐",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if result == nil {
		t.Fatal("expected command request, got nil")
	}
	if result.Module != parser.ModuleMisc || result.Mode != "inventory-list" {
		t.Fatalf("unexpected command request: %+v", result)
	}

	var params inventoryListParams
	if err := json.Unmarshal(result.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Mode != "self" || params.Selector != "u2" {
		t.Fatalf("unexpected self params: %+v", params.userQueryParams)
	}
	if params.Filter != renderinventory.FilterBoost {
		t.Fatalf("Filter = %q, want %q", params.Filter, renderinventory.FilterBoost)
	}
}

func TestInventoryListHandleRejectsCNMysekaiFilter(t *testing.T) {
	h := sekaiHandlers{}.InventoryListHandle()
	h.Regions = AllRegions

	_, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "12345",
		TriggerCmd: "/cn查背包",
		ArgText:    "ms材料",
	})
	if err == nil {
		t.Fatal("Handle() error = nil, want replay error")
	}
	if !strings.Contains(err.Error(), "国服 MySekai 功能永不开启") {
		t.Fatalf("error = %v", err)
	}
}

func TestInventoryListHandleRejectsUnknownFilter(t *testing.T) {
	h := sekaiHandlers{}.InventoryListHandle()
	h.Regions = AllRegions

	_, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "12345",
		TriggerCmd: "/查背包",
		ArgText:    "不存在",
	})
	if err == nil {
		t.Fatal("Handle() error = nil, want replay error")
	}
	var replay onebot11.ReplayError
	if !errors.As(err, &replay) {
		t.Fatalf("error = %T %v, want ReplayError", err, err)
	}
}
