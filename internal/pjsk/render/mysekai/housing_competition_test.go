package mysekai

import (
	"context"
	"encoding/base64"
	stdjson "encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	renderregion "haruki-cloud/internal/pjsk/region"
)

type fakeHousingCompetitionListClient struct {
	responses      []stdjson.RawMessage
	calls          []fakeHousingCompetitionListCall
	thumbnailCalls []fakeHousingCompetitionThumbnailCall
}

type fakeHousingCompetitionListCall struct {
	server    string
	housingID int
	isLottery bool
}

type fakeHousingCompetitionThumbnailCall struct {
	server    string
	imagePath string
}

func (f *fakeHousingCompetitionListClient) GetMySekaiHousingCompetitionList(server string, housingID int, isLottery bool) (stdjson.RawMessage, error) {
	f.calls = append(f.calls, fakeHousingCompetitionListCall{server: server, housingID: housingID, isLottery: isLottery})
	if len(f.responses) == 0 {
		return stdjson.RawMessage(`{"lotteryAt":2000,"results":[]}`), nil
	}
	idx := len(f.calls) - 1
	if idx >= len(f.responses) {
		idx = len(f.responses) - 1
	}
	return f.responses[idx], nil
}

func (f *fakeHousingCompetitionListClient) GetMySekaiHousingThumbnail(server, imagePath string) ([]byte, error) {
	f.thumbnailCalls = append(f.thumbnailCalls, fakeHousingCompetitionThumbnailCall{server: server, imagePath: imagePath})
	return []byte("thumb-" + imagePath), nil
}

func TestBuildHousingCompetitionLineSamplesAndRanksByReviewCount(t *testing.T) {
	root := t.TempDir()
	writeHousingCompetitionMasterdata(t, root)

	api := &fakeHousingCompetitionListClient{
		responses: []stdjson.RawMessage{
			stdjson.RawMessage(`{"lotteryAt":2000,"results":[
				{"mysekaiHousingCompetitionId":25,"isDisplayable":true,"mysekaiOwnerUserId":101,"mysekaiOwnerUserName":"owner-a","userMysekaiHousingCompetitionName":"entry-a","thumbnailPath":"hash/a","submittedAt":1100,"reviewCount":10},
				{"mysekaiHousingCompetitionId":25,"isDisplayable":true,"mysekaiOwnerUserId":102,"mysekaiOwnerUserName":"owner-b","userMysekaiHousingCompetitionName":"entry-b","thumbnailPath":"hash/b","submittedAt":1200,"reviewCount":12}
			]}`),
			stdjson.RawMessage(`{"lotteryAt":2600,"results":[
				{"mysekaiHousingCompetitionId":25,"isDisplayable":true,"mysekaiOwnerUserId":101,"mysekaiOwnerUserName":"owner-a","userMysekaiHousingCompetitionName":"entry-a","thumbnailPath":"hash/a","submittedAt":1100,"reviewCount":15},
				{"mysekaiHousingCompetitionId":25,"isDisplayable":true,"mysekaiOwnerUserId":103,"mysekaiOwnerUserName":"owner-c","userMysekaiHousingCompetitionName":"entry-c","thumbnailPath":"hash/c","submittedAt":1300,"reviewCount":1}
			]}`),
		},
	}

	controller := NewController(nil, nil, renderregion.JP, nil, MasterdataOptions{LocalDir: root, AllowFallback: true})
	result, err := controller.BuildHousingCompetitionLine(context.Background(), api, HousingCompetitionLineQuery{
		Region:               "jp",
		Ranks:                []int{1, 2, 3},
		SampleCount:          2,
		SampleIntervalMillis: -1,
		Now:                  time.UnixMilli(1500),
	})
	if err != nil {
		t.Fatalf("BuildHousingCompetitionLine() error = %v", err)
	}
	if len(api.calls) != 2 {
		t.Fatalf("calls = %+v", api.calls)
	}
	for _, call := range api.calls {
		if call.server != "jp" || call.housingID != 25 || !call.isLottery {
			t.Fatalf("unexpected api call: %+v", call)
		}
	}
	if result.Competition.ID != 25 || result.UniqueCount != 3 || result.SampleCount != 2 {
		t.Fatalf("unexpected result meta: %+v", result)
	}
	gotNames := []string{result.Entries[0].EntryName, result.Entries[1].EntryName, result.Entries[2].EntryName}
	if !reflect.DeepEqual(gotNames, []string{"entry-a", "entry-b", "entry-c"}) {
		t.Fatalf("unexpected rank order: %+v", gotNames)
	}
	gotScores := []int{result.Request.Entries[0].ReviewCount, result.Request.Entries[1].ReviewCount, result.Request.Entries[2].ReviewCount}
	if !reflect.DeepEqual(gotScores, []int{15, 12, 1}) {
		t.Fatalf("unexpected request scores: %+v", gotScores)
	}
	if result.Request.CompetitionID != 25 || result.Request.Name != "烤森百景 ブロックアート" {
		t.Fatalf("unexpected drawing request: %+v", result.Request)
	}
	if result.Request.Description == nil || *result.Request.Description != HousingCompetitionNotice {
		t.Fatalf("unexpected notice: %v", result.Request.Description)
	}
	if result.Request.BannerImagePath == nil || *result.Request.BannerImagePath != "asset/jp-assets/ondemand/mysekai/effect/ui_anim/mysekai_housing_competition/lottery_result/bg_competition_contest_1.png" {
		t.Fatalf("unexpected banner path: %v", result.Request.BannerImagePath)
	}
	requestPayload, err := stdjson.Marshal(result.Request)
	if err != nil {
		t.Fatalf("marshal drawing request: %v", err)
	}
	if strings.Contains(string(requestPayload), "owner_user_id") {
		t.Fatalf("owner user id should not be sent to drawing api: %s", string(requestPayload))
	}
	if result.Request.Entries[0].NextReviewCount == nil || *result.Request.Entries[0].NextReviewCount != 12 {
		t.Fatalf("unexpected next review count: %+v", result.Request.Entries[0])
	}
	if result.Request.Entries[1].PreviousDelta == nil || *result.Request.Entries[1].PreviousDelta != 3 {
		t.Fatalf("unexpected previous delta: %+v", result.Request.Entries[1])
	}
	if result.Request.Entries[0].ThumbnailImageBase64 == nil || *result.Request.Entries[0].ThumbnailImageBase64 != base64.StdEncoding.EncodeToString([]byte("thumb-hash/a")) {
		t.Fatalf("unexpected thumbnail payload: %+v", result.Request.Entries[0].ThumbnailImageBase64)
	}
	if len(api.thumbnailCalls) != 3 {
		t.Fatalf("thumbnail calls = %+v", api.thumbnailCalls)
	}
}

func TestHousingCompetitionStatsCachePersistsWithoutOwnerUserID(t *testing.T) {
	root := t.TempDir()
	writeHousingCompetitionMasterdata(t, root)
	cachePath := filepath.Join(t.TempDir(), "housing_stats.json")

	api := &fakeHousingCompetitionListClient{
		responses: []stdjson.RawMessage{
			stdjson.RawMessage(`{"lotteryAt":2000,"results":[
				{"mysekaiHousingCompetitionId":25,"isDisplayable":true,"mysekaiOwnerUserId":777777777777,"mysekaiOwnerUserName":"owner-a","userMysekaiHousingCompetitionName":"entry-a","thumbnailPath":"hash/a","submittedAt":1100,"reviewCount":10}
			]}`),
		},
	}
	controller := NewController(nil, nil, renderregion.JP, nil, MasterdataOptions{
		LocalDir:                         root,
		AllowFallback:                    true,
		HousingCompetitionStatsCachePath: cachePath,
	})
	if _, err := controller.BuildHousingCompetitionLine(context.Background(), api, HousingCompetitionLineQuery{
		Region: "jp",
		Ranks:  []int{1},
		Now:    time.UnixMilli(1500),
	}); err != nil {
		t.Fatalf("initial BuildHousingCompetitionLine() error = %v", err)
	}
	if len(api.calls) != 1 {
		t.Fatalf("initial api calls = %+v", api.calls)
	}
	payload, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatalf("read cache file: %v", err)
	}
	if strings.Contains(string(payload), "777777777777") || strings.Contains(string(payload), "owner_user_id") {
		t.Fatalf("cache file should not expose owner user id: %s", string(payload))
	}

	cachedAPI := &fakeHousingCompetitionListClient{}
	loaded := NewController(nil, nil, renderregion.JP, nil, MasterdataOptions{
		LocalDir:                         root,
		AllowFallback:                    true,
		HousingCompetitionStatsCachePath: cachePath,
	})
	result, err := loaded.BuildHousingCompetitionLine(context.Background(), cachedAPI, HousingCompetitionLineQuery{
		Region: "jp",
		Ranks:  []int{1},
		Now:    time.UnixMilli(1500),
	})
	if err != nil {
		t.Fatalf("cached BuildHousingCompetitionLine() error = %v", err)
	}
	if len(cachedAPI.calls) != 0 {
		t.Fatalf("fresh persisted cache should avoid list api calls: %+v", cachedAPI.calls)
	}
	if result.Entries[0].OwnerUserID != 0 {
		t.Fatalf("cached entry should not retain owner user id: %+v", result.Entries[0])
	}
}

func writeHousingCompetitionMasterdata(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "jp"), 0o755); err != nil {
		t.Fatalf("mkdir masterdata: %v", err)
	}
	masterdata := `[
		{
			"id":25,
			"name":"ブロックアート",
			"description":"test",
			"submitStartAt":1000,
			"reviewStartAt":1000,
			"submitEndAt":2500,
			"aggregateAt":3000,
			"backgroundImageAssetbundleFileName":"bg_competition_contest_1"
		}
	]`
	if err := os.WriteFile(filepath.Join(root, "jp", "mysekaiHousingCompetitions.json"), []byte(masterdata), 0o644); err != nil {
		t.Fatalf("write masterdata: %v", err)
	}
}
