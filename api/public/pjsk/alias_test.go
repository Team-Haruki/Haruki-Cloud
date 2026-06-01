package pjsk

import (
	"context"
	"encoding/json"
	"fmt"
	sonic "github.com/bytedance/sonic"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"haruki-cloud/database/pjsk"

	"github.com/gofiber/fiber/v3"
	_ "github.com/mattn/go-sqlite3"
)

type pjskEnvelope struct {
	Status  int             `json:"status"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func TestPublicPJSKAliasEndpoints(t *testing.T) {
	ctx := context.Background()
	client := openPJSKTestClient(t)
	defer func() { _ = client.Close() }()

	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	client.Alias.Create().SetAliasType("music").SetAliasTypeID(2001).SetAlias("sekai-song").SaveX(ctx)
	client.Alias.Create().SetAliasType("music").SetAliasTypeID(2001).SetAlias("ss").SaveX(ctx)
	client.GroupAlias.Create().SetPlatform("qq").SetGroupID("g1").SetAliasType("character").SetAliasTypeID(3001).SetAlias("miku").SaveX(ctx)

	app := fiber.New()
	RegisterPJSKRoutes(app, client, nil)

	aliasToID := requestPJSK(t, app, http.MethodGet, "/api/v2/public/pjsk/alias/music/by-alias?alias=sekai-song", "")
	if aliasToID.Status != fiber.StatusOK {
		t.Fatalf("/api/v2/public/pjsk/alias/:alias_type/by-alias failed: status=%d message=%s", aliasToID.Status, aliasToID.Message)
	}
	var toIDData struct {
		MatchIDs []int `json:"match_ids"`
	}
	if err := sonic.Unmarshal(aliasToID.Data, &toIDData); err != nil {
		t.Fatalf("decode alias->id data: %v", err)
	}
	if len(toIDData.MatchIDs) != 1 || toIDData.MatchIDs[0] != 2001 {
		t.Fatalf("unexpected alias->id data: %+v", toIDData)
	}

	aliasList := requestPJSK(t, app, http.MethodGet, "/api/v2/public/pjsk/alias/music/2001", "")
	if aliasList.Status != fiber.StatusOK {
		t.Fatalf("/api/v2/public/pjsk/alias/:alias_type/:alias_type_id failed: status=%d message=%s", aliasList.Status, aliasList.Message)
	}
	var listData struct {
		Aliases []string `json:"aliases"`
	}
	if err := sonic.Unmarshal(aliasList.Data, &listData); err != nil {
		t.Fatalf("decode aliases data: %v", err)
	}
	if len(listData.Aliases) != 2 {
		t.Fatalf("expected 2 aliases, got %+v", listData)
	}

	notGroup := requestPJSK(t, app, http.MethodGet, "/api/v2/public/pjsk/alias/character/by-alias?alias=miku", "")
	if notGroup.Status != fiber.StatusNotFound {
		t.Fatalf("group alias should not be exposed on public API, got status=%d message=%s", notGroup.Status, notGroup.Message)
	}
}

func requestPJSK(t *testing.T, app *fiber.App, method, path, body string) pjskEnvelope {
	t.Helper()
	req, err := http.NewRequest(method, path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Host = "localhost"
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("execute request: %v", err)
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}

	var envelope pjskEnvelope
	if err := sonic.Unmarshal(payload, &envelope); err != nil {
		t.Fatalf("decode response: %v raw=%s", err, string(payload))
	}
	return envelope
}

func openPJSKTestClient(t *testing.T) *pjsk.Client {
	t.Helper()
	dsn := fmt.Sprintf("file:api_pjsk_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano())
	client, err := pjsk.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	return client
}
