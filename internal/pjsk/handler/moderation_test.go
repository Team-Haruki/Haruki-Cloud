package handler

import (
	"context"
	"strings"
	"testing"

	"haruki-cloud/internal/pjsk/parser"

	json "github.com/bytedance/sonic"
)

func TestGlobalKillHandleParsesPermanentBan(t *testing.T) {
	h := sekaiHandlers{}.GlobalKillHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "9001",
		TriggerCmd: "/kill",
		ArgText:    "123456789 恶意滥用",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if result.Module != parser.ModuleAdmin {
		t.Fatalf("unexpected module: %v", result.Module)
	}
	var params globalKillParams
	if err := json.Unmarshal(result.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.QQID != "123456789" || params.Reason != "恶意滥用" || params.Days != nil {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestGlobalKillHandleParsesLastIntegerAsDays(t *testing.T) {
	h := sekaiHandlers{}.GlobalKillHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "9001",
		TriggerCmd: "/kill",
		ArgText:    "00123456789 频繁攻击服务 30",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	var params globalKillParams
	if err := json.Unmarshal(result.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.QQID != "123456789" || params.Reason != "频繁攻击服务" || params.Days == nil || *params.Days != 30 {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestGlobalKillHandleRejectsSelfBan(t *testing.T) {
	h := sekaiHandlers{}.GlobalKillHandle()
	_, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "123456789",
		TriggerCmd: "/kill",
		ArgText:    "123456789 测试",
	})
	if err == nil || !strings.Contains(err.Error(), "不能使用 /kill 封禁自己") {
		t.Fatalf("expected self-ban error, got %v", err)
	}
}

func TestGlobalKillHandleKeepsNumericReasonWhenDurationIsOmitted(t *testing.T) {
	h := sekaiHandlers{}.GlobalKillHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "9001",
		TriggerCmd: "/kill",
		ArgText:    "123456789 404",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	var params globalKillParams
	if err := json.Unmarshal(result.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Reason != "404" || params.Days != nil {
		t.Fatalf("unexpected numeric reason params: %+v", params)
	}
}

func TestGlobalKillHandleRejectsMalformedDuration(t *testing.T) {
	h := sekaiHandlers{}.GlobalKillHandle()
	_, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "9001",
		TriggerCmd: "/kill",
		ArgText:    "123456789 滥用 999999999999999999999999999999",
	})
	if err == nil || !strings.Contains(err.Error(), "封禁天数必须为正整数") {
		t.Fatalf("expected duration error, got %v", err)
	}
}

func TestGlobalBackHandleRequiresOneQQID(t *testing.T) {
	h := sekaiHandlers{}.GlobalBackHandle()
	result, err := h.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		Platform:   "qq",
		UserId:     "9001",
		TriggerCmd: "/back",
		ArgText:    "123456789",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	var params globalBackParams
	if err := json.Unmarshal(result.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.QQID != "123456789" {
		t.Fatalf("unexpected params: %+v", params)
	}
}
