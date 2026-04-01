package sekai

import (
	"context"
	"encoding/json"
	"testing"

	"haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/parser"
	"haruki-cloud/internal/pjsk/render/education"
)

func TestAreaItemHandleBuildsResolvedCommand(t *testing.T) {
	tests := []struct {
		name        string
		args        string
		expectError bool
		checkFunc   func(*testing.T, education.AreaItemQuery)
	}{
		{
			name:        "no filter returns usage error",
			args:        "",
			expectError: true,
		},
		{
			name: "all filters are parsed and passed through",
			args: "25h miku 可爱 树 花",
			checkFunc: func(t *testing.T, query education.AreaItemQuery) {
				t.Helper()
				if query.Unit != "school_refusal" || query.Cid != 0 || query.CharacterQuery != "miku" || query.Attr != "cute" || !query.Tree || !query.Flower {
					t.Fatalf("unexpected query: %+v", query)
				}
			},
		},
		{
			name: "piapro alias is normalized",
			args: "vs",
			checkFunc: func(t *testing.T, query education.AreaItemQuery) {
				t.Helper()
				if query.Unit != "piapro" {
					t.Fatalf("unexpected query: %+v", query)
				}
			},
		},
		{
			name: "unknown nickname is preserved as character query",
			args: "初音未来",
			checkFunc: func(t *testing.T, query education.AreaItemQuery) {
				t.Helper()
				if query.Cid != 0 || query.CharacterQuery != "初音未来" {
					t.Fatalf("unexpected query: %+v", query)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := sekaiHandlers{}.AreaItemHandle()
			result, err := h.Handle(&handler.HandlerContext{
				Context:    context.Background(),
				TriggerCmd: "/区域道具",
				ArgText:    tt.args,
			})
			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Handle() error = %v", err)
			}

			resolved, ok := result.(*parser.ResolvedCommand)
			if !ok {
				t.Fatalf("handler returned %T", result)
			}
			if resolved.Module != parser.ModuleEducation || resolved.Mode != "education-area" {
				t.Fatalf("unexpected resolved command: %+v", resolved)
			}

			var query education.AreaItemQuery
			if err := json.Unmarshal(resolved.Params, &query); err != nil {
				t.Fatalf("unmarshal params: %v", err)
			}
			tt.checkFunc(t, query)
		})
	}
}
