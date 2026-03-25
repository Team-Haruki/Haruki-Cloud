package sekai

import (
	"context"
	"testing"

	"haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/parser"
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

	resolved, ok := result.(*parser.ResolvedCommand)
	if !ok {
		t.Fatalf("expected *ResolvedCommand, got %T", result)
	}
	if string(resolved.Params) != `{"skill":true}` {
		t.Fatalf("params = %s", string(resolved.Params))
	}
}
