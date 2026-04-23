package handler

import (
	"context"
	json "github.com/bytedance/sonic"
	"testing"

	"haruki-cloud/internal/pjsk/parser"
)

func TestMiscBirthdayHandleBuildsCommandRequest(t *testing.T) {
	tests := []struct {
		name      string
		args      string
		checkFunc func(*testing.T, []byte)
	}{
		{
			name: "nearest birthday by default",
			args: "",
			checkFunc: func(t *testing.T, raw []byte) {
				t.Helper()
				var params miscBirthdayParams
				if err := json.Unmarshal(raw, &params); err != nil {
					t.Fatalf("unmarshal params: %v", err)
				}
				if params.UpcomingIndex != 1 || params.Cid != 0 {
					t.Fatalf("unexpected params: %+v", params)
				}
			},
		},
		{
			name: "nth upcoming birthday",
			args: "2",
			checkFunc: func(t *testing.T, raw []byte) {
				t.Helper()
				var params miscBirthdayParams
				if err := json.Unmarshal(raw, &params); err != nil {
					t.Fatalf("unmarshal params: %v", err)
				}
				if params.UpcomingIndex != 2 || params.Cid != 0 {
					t.Fatalf("unexpected params: %+v", params)
				}
			},
		},
		{
			name: "character nickname",
			args: "miku",
			checkFunc: func(t *testing.T, raw []byte) {
				t.Helper()
				var params miscBirthdayParams
				if err := json.Unmarshal(raw, &params); err != nil {
					t.Fatalf("unmarshal params: %v", err)
				}
				if params.Query != "miku" || params.Cid != 0 || params.UpcomingIndex != 0 {
					t.Fatalf("unexpected params: %+v", params)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := sekaiHandlers{}.MiscBirthdayHandle()

			result, err := h.Handle(&PjskHandlerContext{
				Context:    context.Background(),
				TriggerCmd: "/生日",
				ArgText:    tt.args,
			})
			if err != nil {
				t.Fatalf("Handle() error = %v", err)
			}

			resolved := result
			if resolved == nil {
				t.Fatal("expected command request, got nil")
			}
			if resolved.Module != parser.ModuleMisc || resolved.Mode != "misc-birthday" {
				t.Fatalf("unexpected command request: %+v", resolved)
			}
			tt.checkFunc(t, resolved.Params)
		})
	}
}
