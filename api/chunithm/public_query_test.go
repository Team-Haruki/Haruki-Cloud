package chunithm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	chunithmMainDB "haruki-cloud/database/chunithm/maindb"
	chunithmMusicDB "haruki-cloud/database/chunithm/music"

	"github.com/gofiber/fiber/v3"
	_ "github.com/mattn/go-sqlite3"
)

type apiEnvelope struct {
	Status  int             `json:"status"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func TestPublicChunithmQueryEndpoints(t *testing.T) {
	ctx := context.Background()
	mainClient := openChunithmMainTestClient(t)
	musicClient := openChunithmMusicTestClient(t)
	defer func() {
		_ = mainClient.Close()
		_ = musicClient.Close()
	}()

	if err := mainClient.Schema.Create(ctx); err != nil {
		t.Fatalf("create main schema: %v", err)
	}
	if err := musicClient.Schema.Create(ctx); err != nil {
		t.Fatalf("create music schema: %v", err)
	}

	releaseAt := time.Now().Add(-2 * time.Hour)
	mainClient.ChunithmMusicAlias.Create().SetMusicID(1001).SetAlias("test-song").SaveX(ctx)
	mainClient.ChunithmMusicAlias.Create().SetMusicID(1001).SetAlias("ts").SaveX(ctx)
	musicClient.ChunithmMusic.Create().SetMusicID(1001).SetTitle("Test Song").SetArtist("Artist").SetCategory("POPS").SetVersion("v2").SetReleaseDate(releaseAt).SaveX(ctx)
	musicClient.ChunithmMusicDifficulty.Create().SetMusicID(1001).SetVersion("v1").SetDiff0Const(12.1).SetDiff1Const(12.8).SetDiff2Const(13.4).SetDiff3Const(13.9).SetDiff4Const(14.6).SaveX(ctx)
	musicClient.ChunithmChartData.Create().SetMusicID(1001).SetDifficulty(3).SetCreator("ChartMaster").SetBpm(180).SetTapCount(500).SetHoldCount(120).SetSlideCount(80).SetAirCount(70).SetFlickCount(30).SetTotalCount(800).SaveX(ctx)

	app := fiber.New()
	RegisterChunithmRoutes(app, mainClient, musicClient, nil)

	aliasResp := requestAPI(t, app, http.MethodGet, "/chunithm/alias/music-id?alias=test-song", "")
	if aliasResp.Status != fiber.StatusOK {
		t.Fatalf("alias/music-id failed: status=%d message=%s", aliasResp.Status, aliasResp.Message)
	}
	var aliasData struct {
		MatchIDs []int `json:"match_ids"`
	}
	if err := json.Unmarshal(aliasResp.Data, &aliasData); err != nil {
		t.Fatalf("decode alias response: %v", err)
	}
	if len(aliasData.MatchIDs) != 1 || aliasData.MatchIDs[0] != 1001 {
		t.Fatalf("unexpected alias/music-id data: %+v", aliasData)
	}

	aliasListResp := requestAPI(t, app, http.MethodGet, "/chunithm/alias/1001", "")
	if aliasListResp.Status != fiber.StatusOK {
		t.Fatalf("alias/:music_id failed: status=%d message=%s", aliasListResp.Status, aliasListResp.Message)
	}

	allMusicResp := requestAPI(t, app, http.MethodGet, "/chunithm/music/all-music", "")
	if allMusicResp.Status != fiber.StatusOK {
		t.Fatalf("music/all-music failed: status=%d message=%s", allMusicResp.Status, allMusicResp.Message)
	}

	basicResp := requestAPI(t, app, http.MethodGet, "/chunithm/music/1001/basic-info", "")
	if basicResp.Status != fiber.StatusOK {
		t.Fatalf("music/basic-info failed: status=%d message=%s", basicResp.Status, basicResp.Message)
	}

	difficultyResp := requestAPI(t, app, http.MethodGet, "/chunithm/music/1001/difficulty-info?version=v1", "")
	if difficultyResp.Status != fiber.StatusOK {
		t.Fatalf("music/difficulty-info failed: status=%d message=%s", difficultyResp.Status, difficultyResp.Message)
	}

	chartResp := requestAPI(t, app, http.MethodGet, "/chunithm/music/1001/chart-data", "")
	if chartResp.Status != fiber.StatusOK {
		t.Fatalf("music/chart-data failed: status=%d message=%s", chartResp.Status, chartResp.Message)
	}

	batchBody := `{"music_ids":[1001,9999],"version":"v1"}`
	batchCompatResp := requestAPI(t, app, http.MethodPost, "/chunithm/query-batch", batchBody)
	if batchCompatResp.Status != fiber.StatusOK {
		t.Fatalf("compat /chunithm/query-batch failed: status=%d message=%s", batchCompatResp.Status, batchCompatResp.Message)
	}

	batchMusicResp := requestAPI(t, app, http.MethodPost, "/chunithm/music/query-batch", batchBody)
	if batchMusicResp.Status != fiber.StatusOK {
		t.Fatalf("/chunithm/music/query-batch failed: status=%d message=%s", batchMusicResp.Status, batchMusicResp.Message)
	}
}

func requestAPI(t *testing.T, app *fiber.App, method, path, body string) apiEnvelope {
	t.Helper()
	req, err := http.NewRequest(method, path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
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

	var envelope apiEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		t.Fatalf("decode response: %v raw=%s", err, string(payload))
	}
	return envelope
}

func openChunithmMainTestClient(t *testing.T) *chunithmMainDB.Client {
	t.Helper()
	dsn := fmt.Sprintf("file:api_chuni_main_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano())
	client, err := chunithmMainDB.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open main sqlite: %v", err)
	}
	return client
}

func openChunithmMusicTestClient(t *testing.T) *chunithmMusicDB.Client {
	t.Helper()
	dsn := fmt.Sprintf("file:api_chuni_music_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano())
	client, err := chunithmMusicDB.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open music sqlite: %v", err)
	}
	return client
}
