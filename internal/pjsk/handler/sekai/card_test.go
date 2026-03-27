package sekai

import (
	"context"
	"encoding/json"
	"testing"

	"haruki-cloud/internal/pjsk/handler"
	"haruki-cloud/internal/pjsk/parser"
	"haruki-cloud/internal/pjsk/render/card"
	renderregion "haruki-cloud/internal/pjsk/render/region"
)

func TestCardDetailAndListHandlersShareDispatchRules(t *testing.T) {
	tests := []struct {
		name       string
		args       string
		wantMode   string
		checkParam func(*testing.T, []byte)
	}{
		{
			name:     "card detail query",
			args:     "1001",
			wantMode: "card-detail",
			checkParam: func(t *testing.T, raw []byte) {
				t.Helper()
				var params card.Query
				if err := json.Unmarshal(raw, &params); err != nil {
					t.Fatalf("unmarshal params: %v", err)
				}
				if params.Query != "1001" || params.Region != "jp" {
					t.Fatalf("unexpected params: %+v", params)
				}
			},
		},
		{
			name:     "card list query",
			args:     "mnr 4星",
			wantMode: "card-list",
			checkParam: func(t *testing.T, raw []byte) {
				t.Helper()
				var params card.ListRequest
				if err := json.Unmarshal(raw, &params); err != nil {
					t.Fatalf("unmarshal params: %v", err)
				}
				if params.Query != "mnr 4星" || params.Region != "jp" {
					t.Fatalf("unexpected params: %+v", params)
				}
			},
		},
		{
			name:     "card box query",
			args:     "mnr 4星 id box before",
			wantMode: "card-box",
			checkParam: func(t *testing.T, raw []byte) {
				t.Helper()
				var params struct {
					ShowID           bool `json:"show_id"`
					ShowBox          bool `json:"show_box"`
					UseAfterTraining bool `json:"use_after_training"`
				}
				if err := json.Unmarshal(raw, &params); err != nil {
					t.Fatalf("unmarshal params: %v", err)
				}
				if !params.ShowID || !params.ShowBox || params.UseAfterTraining {
					t.Fatalf("unexpected params: %+v", params)
				}
			},
		},
	}

	builders := []struct {
		name string
		make func() SekaiCommandHandler
	}{
		{name: "detail", make: func() SekaiCommandHandler { return sekaiHandlers{}.CardDetailHandle() }},
		{name: "list", make: func() SekaiCommandHandler { return sekaiHandlers{}.CardListHandle() }},
	}

	for _, builder := range builders {
		for _, tt := range tests {
			t.Run(builder.name+"_"+tt.name, func(t *testing.T) {
				h := builder.make()
				h.Regions = []renderregion.Value{renderregion.JP}

				result, err := h.Handle(&handler.HandlerContext{
					Context:    context.Background(),
					TriggerCmd: "/查卡",
					ArgText:    tt.args,
				})
				if err != nil {
					t.Fatalf("Handle() error = %v", err)
				}

				resolved, ok := result.(*parser.ResolvedCommand)
				if !ok {
					t.Fatalf("handler returned %T", result)
				}
				if resolved.Module != parser.ModuleCard || resolved.Mode != tt.wantMode {
					t.Fatalf("unexpected resolved command: %+v", resolved)
				}
				tt.checkParam(t, resolved.Params)

				if tt.wantMode == "card-box" && resolved.Query != "mnr 4星" {
					t.Fatalf("unexpected cleaned query: %q", resolved.Query)
				}
			})
		}
	}
}
