package handler

import (
	"context"
	"testing"
)

func TestChartHandleSkillPreviewSetsSkillFlag(t *testing.T) {
	handler := (&sekaiHandlers{}).ChartHandle()
	result, err := handler.Handle(&PjskHandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/技能预览",
		ArgText:    "初音 future",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected command request, got nil")
	}
	if string(resolved.Params) != `{"skill":true}` {
		t.Fatalf("params = %s", string(resolved.Params))
	}
}
