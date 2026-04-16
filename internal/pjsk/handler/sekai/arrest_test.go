package sekai

import (
	"context"
	"encoding/json"
	"testing"

	"haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/parser"
	renderregion "haruki-cloud/internal/pjsk/region"
)

func TestArrestHandleParsesUserTargetModes(t *testing.T) {
	h := sekaiHandlers{}.ArrestHandle()
	h.Regions = []renderregion.Value{renderregion.JP}
	if h.ParseUIDArg == nil || !*h.ParseUIDArg {
		t.Fatalf("ArrestHandle should explicitly enable UID parsing")
	}

	tests := []struct {
		name string
		ctx  *handler.HandlerContext
		want UserQueryParams
	}{
		{
			name: "self",
			ctx: &handler.HandlerContext{
				Context:    context.Background(),
				Platform:   "qq",
				UserId:     "10086",
				TriggerCmd: "/逮捕",
			},
			want: UserQueryParams{
				Mode:           "self",
				Platform:       "qq",
				PlatformUserID: "10086",
			},
		},
		{
			name: "at user",
			ctx: &handler.HandlerContext{
				Context:    context.Background(),
				Platform:   "qq",
				UserId:     "10086",
				TriggerCmd: "/逮捕",
				AtIds:      []string{"987654321"},
			},
			want: UserQueryParams{
				Mode:           "at_user",
				Platform:       "qq",
				PlatformUserID: "10086",
				AtUserID:       "987654321",
			},
		},
		{
			name: "game uid",
			ctx: &handler.HandlerContext{
				Context:    context.Background(),
				Platform:   "qq",
				UserId:     "10086",
				TriggerCmd: "/逮捕",
				ArgText:    "12345678901234",
			},
			want: UserQueryParams{
				Mode:           "uid",
				Platform:       "qq",
				PlatformUserID: "10086",
				PJSKUserID:     "12345678901234",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := h.Handle(tt.ctx)
			if err != nil {
				t.Fatalf("Handle() error = %v", err)
			}

			resolved, ok := result.(*parser.ResolvedCommand)
			if !ok {
				t.Fatalf("handler returned %T", result)
			}
			if resolved.Module != parser.ModuleArrest || resolved.Mode != "arrest" {
				t.Fatalf("unexpected resolved command: %+v", resolved)
			}

			var got UserQueryParams
			if err := json.Unmarshal(resolved.Params, &got); err != nil {
				t.Fatalf("unmarshal params: %v", err)
			}
			if got != tt.want {
				t.Fatalf("params = %+v, want %+v", got, tt.want)
			}
		})
	}
}
