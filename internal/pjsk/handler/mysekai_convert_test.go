package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	corehandler "haruki-cloud/internal/handler"
	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	rendermysekai "haruki-cloud/internal/pjsk/render/mysekai"
	"haruki-cloud/utils/usererror"
)

func TestMysekaiConvertRecipeCommand(t *testing.T) {
	for _, tc := range []struct {
		args  string
		query string
	}{
		{"", ""},
		{"唱片", "唱片"},
		{"22", "22"},
		{" Blueprint   Choice ", "Blueprint Choice"},
		{"蓝图", "蓝图"},
	} {
		t.Run(tc.args, func(t *testing.T) {
			h := sekaiHandlers{}.MysekaiConvertRecipesHandle()
			h.Regions = []renderregion.Value{renderregion.JP}
			cmd, err := h.Handle(&PjskHandlerContext{Context: context.Background(), TriggerCmd: "/烤森合成表", ArgText: tc.args})
			if err != nil {
				t.Fatal(err)
			}
			var query rendermysekai.ConvertRecipeQuery
			if err := json.Unmarshal(cmd.Params, &query); err != nil {
				t.Fatal(err)
			}
			if cmd.CommandPath != "mysekai/convert-recipes" || cmd.Mode != mySekaiConvertRecipeCommand || query.Query != tc.query || cmd.executor == nil {
				t.Fatalf("unexpected command: %+v query: %+v", cmd, query)
			}
		})
	}
	EnsureCommandHandlersRegistered()
	found := false
	for _, route := range corehandler.ListBotRoutes() {
		if route.Path != "mysekai/convert-recipes" {
			continue
		}
		for _, command := range route.Commands {
			if command == "/烤森合成表" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("converter command missing from bot manifest")
	}
	h := sekaiHandlers{}.MysekaiConvertRecipesHandle()
	cmd, err := h.Handle(&PjskHandlerContext{Context: context.Background(), TriggerCmd: "/烤森合成表", ArgText: "-h"})
	if err != nil {
		t.Fatal(err)
	}
	md, err := commandHelpMarkdown(cmd)
	if err != nil || !cmd.IsHelp || !strings.Contains(md, "转换器") {
		t.Fatalf("help command: %+v, markdown=%q error=%v", cmd, md, err)
	}
}

func TestExecuteMysekaiConvertRecipesWithoutUserData(t *testing.T) {
	root := t.TempDir()
	for name, value := range map[string]string{
		"mysekaiConvertItems.json":                       `[{"id":22,"mysekaiConvertItemTabId":3,"consumeResourceBoxId":1,"mysekaiConvertItemObtainType":"mysekai_blueprint","externalId":1,"convertMinutes":60,"requiredConvertFixtureLevel":1,"onceConvertLimit":1}]`,
		"mysekaiConvertItemTabs.json":                    `[{"id":3,"seq":30,"mysekaiConvertItemTabType":"mysekai_blueprint"}]`,
		"mysekaiConvertItemObtainMysekaiBlueprints.json": `[{"id":1,"mysekaiConvertItemObtainMysekaiBlueprintType":"random","quantity":1}]`,
		"resourceBoxes.json":                             `[{"id":1,"resourceBoxPurpose":"mysekai_convert_consumption","details":[{"resourceType":"coin","resourceQuantity":1000}]}]`,
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, mode := range []string{"text", "image", "drawing failure"} {
		t.Run(mode, func(t *testing.T) {
			app := &renderapp.App{MySekai: rendermysekai.NewController(nil, nil, renderregion.JP, nil, rendermysekai.MasterdataOptions{LocalDir: root, AllowFallback: true})}
			if mode != "text" {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path != "/api/pjsk/mysekai/convert-recipes" {
						t.Errorf("unexpected render path: %s", r.URL.Path)
					}
					if mode == "drawing failure" {
						http.Error(w, "unavailable", http.StatusBadRequest)
						return
					}
					var payload drawing.MysekaiConvertRecipesRequest
					if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
						t.Error(err)
					}
					if len(payload.Recipes) != 1 || len(payload.Recipes[0].Inputs) != 1 || payload.Recipes[0].Inputs[0].Quantity == nil || *payload.Recipes[0].Inputs[0].Quantity != 1000 || !strings.Contains(payload.Title, "JP") {
						t.Errorf("unexpected recipe payload: %+v", payload)
					}
					if payload.Labels["mysekai.convert.group_count"] == "" || payload.Labels["mysekai.convert.other"] == "" {
						t.Errorf("missing localized group labels: %+v", payload.Labels)
					}
					w.Header().Set("Content-Type", "image/png")
					_, _ = w.Write([]byte("recipe-image"))
				}))
				defer server.Close()
				app.Drawing = drawing.NewHarukiDrawingClient(server.URL)
			}
			h := sekaiHandlers{}.MysekaiConvertRecipesHandle()
			h.Regions = []renderregion.Value{renderregion.JP}
			cmd, err := h.Handle(&PjskHandlerContext{Context: context.Background(), TriggerCmd: "/烤森合成表"})
			if err != nil {
				t.Fatal(err)
			}
			message, err := ExecuteCommandRequest(context.Background(), cmd, app)
			if err != nil {
				t.Fatal(err)
			}
			if len(message) == 0 {
				t.Fatal("empty converter reply")
			}
			if mode == "image" {
				if message[0].Type != onebot11.TypeImage {
					t.Fatalf("expected image: %+v", message)
				}
			} else if message[0].Type != onebot11.TypeText || !strings.Contains(message[0].Data.(onebot11.TextData).Text, "1000") {
				t.Fatalf("expected recipe text: %+v", message)
			}
			if mode == "text" {
				filtered, err := h.Handle(&PjskHandlerContext{
					Context: context.Background(), TriggerCmd: "/烤森合成表",
					ArgText: "蓝图",
				})
				if err != nil {
					t.Fatal(err)
				}
				filteredMessage, err := ExecuteCommandRequest(context.Background(), filtered, app)
				if err != nil || len(filteredMessage) == 0 || !strings.Contains(filteredMessage[0].Data.(onebot11.TextData).Text, "1000") {
					t.Fatalf("product name filter failed: message=%+v error=%v", filteredMessage, err)
				}
				for _, args := range []string{"不存在的产物", "金币", "22", "产物=蓝图", "分类=蓝图", "等级<=1", "耗时<=1h"} {
					invalid, err := h.Handle(&PjskHandlerContext{Context: context.Background(), TriggerCmd: "/烤森合成表", ArgText: args})
					if err != nil {
						t.Fatal(err)
					}
					_, err = ExecuteCommandRequest(context.Background(), invalid, app)
					if _, ok := usererror.As(err); !ok {
						t.Fatalf("%q should return user-facing query error, got %v", args, err)
					}
				}
			}
		})
	}
}
