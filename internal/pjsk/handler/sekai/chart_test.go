package sekai

import (
	"context"
	"testing"

	"haruki-cloud/internal/pjsk/handler"
)

func TestChartHandleSkillPreviewSetsSkillFlag(t *testing.T) {
	chartHandler := (sekaiHandlers{}).ChartHandle()
	result, err := (&chartHandler).Handle(&handler.HandlerContext{
		Context:    context.Background(),
		TriggerCmd: "/技能预览",
		ArgText:    "初音 future",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	resolved := result
	if resolved == nil {
		t.Fatal("expected resolved command, got nil")
	}
	if string(resolved.Params) != `{"skill":true}` {
		t.Fatalf("params = %s", string(resolved.Params))
	}
}
