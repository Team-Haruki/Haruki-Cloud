package handler

import (
	"context"
	"errors"
	json "github.com/bytedance/sonic"
	"strings"
	"testing"

	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/accountdata"
	"haruki-cloud/internal/pjsk/parser"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	renderevent "haruki-cloud/internal/pjsk/render/event"
)

func TestEventDetailHandleUsesCurrentEventWhenArgsEmpty(t *testing.T) {
	h := sekaiHandlers{}.EventDetailHandle()

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/活动",
		ArgText:    "",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleEvent || resolved.Mode != "event-detail" {
		t.Fatalf("unexpected command request: %+v", resolved)
	}

	var params struct {
		UseCurrent bool `json:"use_current"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if !params.UseCurrent {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestEventListHandleUsesFullRangeWhenArgsEmpty(t *testing.T) {
	h := sekaiHandlers{}.EventHandle()

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/活动列表",
		ArgText:    "",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleEvent || resolved.Mode != "event-list" {
		t.Fatalf("unexpected command request: %+v", resolved)
	}

	var params struct {
		IncludePast   bool `json:"include_past"`
		IncludeFuture bool `json:"include_future"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if !params.IncludePast || !params.IncludeFuture {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestEventDetailHandleFallsBackToListForFilterQuery(t *testing.T) {
	h := sekaiHandlers{}.EventDetailHandle()

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/活动",
		ArgText:    "紫 25h",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleEvent || resolved.Mode != "event-list" {
		t.Fatalf("unexpected command request: %+v", resolved)
	}

	var params struct {
		Attr          string `json:"attr"`
		Unit          string `json:"unit"`
		IncludePast   bool   `json:"include_past"`
		IncludeFuture bool   `json:"include_future"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Attr != "mysterious" || params.Unit != "school_refusal" {
		t.Fatalf("unexpected params: %+v", params)
	}
	if !params.IncludePast || !params.IncludeFuture {
		t.Fatalf("unexpected range params: %+v", params)
	}
}

func TestEventDetailHandleTreatsBare25AsEventID(t *testing.T) {
	h := sekaiHandlers{}.EventDetailHandle()

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/查活动",
		ArgText:    "25",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleEvent || resolved.Mode != "event-detail" {
		t.Fatalf("unexpected command request: %+v", resolved)
	}

	var params struct {
		EventID int `json:"event_id"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.EventID != 25 {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestEventListHandleFallsBackToDetailForSingleEventQuery(t *testing.T) {
	h := sekaiHandlers{}.EventHandle()

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/活动列表",
		ArgText:    "mnr1",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleEvent || resolved.Mode != "event-detail" {
		t.Fatalf("unexpected command request: %+v", resolved)
	}

	var params struct {
		BanCharID int `json:"ban_char_id"`
		BanSeq    int `json:"ban_seq"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.BanCharID != 5 || params.BanSeq != 1 {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestEventListHandleTreatsBare25AsUnitFilter(t *testing.T) {
	h := sekaiHandlers{}.EventHandle()

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/活动列表",
		ArgText:    "25",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleEvent || resolved.Mode != "event-list" {
		t.Fatalf("unexpected command request: %+v", resolved)
	}

	var params struct {
		Unit          string `json:"unit"`
		IncludePast   bool   `json:"include_past"`
		IncludeFuture bool   `json:"include_future"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Unit != "school_refusal" {
		t.Fatalf("unexpected params: %+v", params)
	}
	if !params.IncludePast || !params.IncludeFuture {
		t.Fatalf("unexpected range params: %+v", params)
	}
}

func TestEventListHandleSupportsSharedUnitAndAttrAliases(t *testing.T) {
	h := sekaiHandlers{}.EventHandle()

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/活动列表",
		ArgText:    "v 粉花",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleEvent || resolved.Mode != "event-list" {
		t.Fatalf("unexpected command request: %+v", resolved)
	}

	var params struct {
		Attr          string `json:"attr"`
		Unit          string `json:"unit"`
		IncludePast   bool   `json:"include_past"`
		IncludeFuture bool   `json:"include_future"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Attr != "cute" || params.Unit != "piapro" {
		t.Fatalf("unexpected params: %+v", params)
	}
	if !params.IncludePast || !params.IncludeFuture {
		t.Fatalf("unexpected range params: %+v", params)
	}
}

func TestEventDetailHandleParsesOnlyUnitFilter(t *testing.T) {
	h := sekaiHandlers{}.EventDetailHandle()

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/查活动",
		ArgText:    "仅mmj",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleEvent || resolved.Mode != "event-list" {
		t.Fatalf("unexpected command request: %+v", resolved)
	}

	var params struct {
		Unit          string `json:"unit"`
		OnlyUnit      bool   `json:"only_unit"`
		IncludePast   bool   `json:"include_past"`
		IncludeFuture bool   `json:"include_future"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Unit != "idol" || !params.OnlyUnit {
		t.Fatalf("unexpected params: %+v", params)
	}
	if !params.IncludePast || !params.IncludeFuture {
		t.Fatalf("unexpected range params: %+v", params)
	}
}

func TestEventDetailHandleParsesWorldBloomTurnAndCharacterFilter(t *testing.T) {
	h := sekaiHandlers{}.EventDetailHandle()

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/查活动",
		ArgText:    "wl3 miku",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleEvent || resolved.Mode != "event-list" {
		t.Fatalf("unexpected command request: %+v", resolved)
	}

	var params struct {
		EventType      string `json:"event_type"`
		WorldBloomTurn int    `json:"world_bloom_turn"`
		CharacterID    int    `json:"character_id"`
		IncludePast    bool   `json:"include_past"`
		IncludeFuture  bool   `json:"include_future"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.EventType != "world_bloom" || params.WorldBloomTurn != 3 || params.CharacterID != 21 {
		t.Fatalf("unexpected params: %+v", params)
	}
	if !params.IncludePast || !params.IncludeFuture {
		t.Fatalf("unexpected range params: %+v", params)
	}
}

func TestEventDetailHandleKeepsBareWorldBloomFilter(t *testing.T) {
	h := sekaiHandlers{}.EventDetailHandle()

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/查活动",
		ArgText:    "wl",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	var params struct {
		EventType      string `json:"event_type"`
		WorldBloomTurn int    `json:"world_bloom_turn"`
	}
	if err := json.Unmarshal(result.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.EventType != "world_bloom" || params.WorldBloomTurn != 0 {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestEventHandleReturnsCombinedHelpOnInvalidQuery(t *testing.T) {
	h := sekaiHandlers{}.EventDetailHandle()

	_, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/活动",
		ArgText:    "???",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "【查单个活动格式】") || !strings.Contains(err.Error(), "【查多个活动格式】") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEventRecordHandleEmbedsSelfSelector(t *testing.T) {
	h := sekaiHandlers{}.EventRecordHandle()

	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "42",
		TriggerCmd: "/活动记录",
		ArgText:    "u2",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if resolved.Module != parser.ModuleEvent || resolved.Mode != "event-record" {
		t.Fatalf("unexpected command request: %+v", resolved)
	}

	var params struct {
		Mode           string `json:"mode"`
		Platform       string `json:"platform"`
		PlatformUserID string `json:"platform_user_id"`
		Selector       string `json:"selector"`
	}
	if err := json.Unmarshal(resolved.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Mode != "self" || params.Platform != "qq" || params.PlatformUserID != "42" || params.Selector != "u2" {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestExecuteEventRecordReturnsBindingErrorBeforeSuiteMessage(t *testing.T) {
	_, err := executeEvent(NewRequestContext(context.Background(), &CommandRequest{
		Module:            parser.ModuleEvent,
		Mode:              "event-record",
		Region:            "jp",
		RequesterPlatform: "qq",
		RequesterUserID:   "42",
	}, &renderapp.App{
		Events:   renderevent.NewController(nil, nil, nil),
		Bindings: newHandlerTestBindingService(t),
	}))
	if err == nil {
		t.Fatal("expected error")
	}
	var replyErr onebot11.ReplayError
	if !errors.As(WrapDomainError(err), &replyErr) {
		t.Fatalf("expected ReplayError, got %T (%v)", err, err)
	}
	if string(replyErr) != ErrMsgBindingNotFound {
		t.Fatalf("unexpected replay error: %q", replyErr)
	}
}

func TestExecuteEventRecordReturnsContextualSuiteMessageWhenSnapshotMissing(t *testing.T) {
	ctx := context.Background()
	service := newHandlerTestBindingService(t)
	if _, err := service.Bind(ctx, "qq", "42", "12345678901234"); err != nil {
		t.Fatalf("bind: %v", err)
	}

	_, err := executeEvent(NewRequestContext(ctx, &CommandRequest{
		Module:            parser.ModuleEvent,
		Mode:              "event-record",
		Region:            "jp",
		RequesterPlatform: "qq",
		RequesterUserID:   "42",
	}, &renderapp.App{
		Events:   renderevent.NewController(nil, nil, nil),
		Bindings: service,
	}))
	if err == nil || err.Error() != buildPrivateDataNotFoundMessage("suite", &accountdata.ResolvedBinding{
		Server:     "jp",
		PJSKUserID: "12345678901234",
		Visible:    false,
	}) {
		t.Fatalf("unexpected error: %v", err)
	}
}
