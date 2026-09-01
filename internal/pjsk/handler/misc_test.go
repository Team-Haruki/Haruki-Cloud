package handler

import (
	"context"
	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/testutil"
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
				{
					err := json.Unmarshal(raw, &params)
					testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
				}
				{

					testutil.Require(t, !(params.UpcomingIndex != 1), "unexpected params: %+v", params)
					testutil.Require(t, !(params.Cid != 0), "unexpected params: %+v", params)
				}

			},
		},
		{
			name: "nth upcoming birthday",
			args: "2",
			checkFunc: func(t *testing.T, raw []byte) {
				t.Helper()
				var params miscBirthdayParams
				{
					err := json.Unmarshal(raw, &params)
					testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
				}
				{

					testutil.Require(t, !(params.UpcomingIndex != 2), "unexpected params: %+v", params)
					testutil.Require(t, !(params.Cid != 0), "unexpected params: %+v", params)
				}

			},
		},
		{
			name: "character nickname",
			args: "miku",
			checkFunc: func(t *testing.T, raw []byte) {
				t.Helper()
				var params miscBirthdayParams
				{
					err := json.Unmarshal(raw, &params)
					testutil.Require(t, !(err != nil), "unmarshal params: %v", err)
				}
				{

					testutil.Require(t, !(params.Query != "miku"), "unexpected params: %+v", params)
					testutil.Require(t, !(params.Cid != 0), "unexpected params: %+v", params)
					testutil.Require(t, !(params.UpcomingIndex != 0), "unexpected params: %+v", params)
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
			testutil.Require(t, !(err != nil), "Handle() error = %v", err)

			resolved := result
			testutil.RequireArgs(t, !(resolved == nil), "expected command request, got nil")
			{

				testutil.Require(t, !(resolved.Module != parser.ModuleMisc), "unexpected command request: %+v", resolved)
				testutil.Require(t, !(resolved.Mode != "misc-birthday"), "unexpected command request: %+v", resolved)
			}

			tt.checkFunc(t, resolved.Params)
		})
	}
}
