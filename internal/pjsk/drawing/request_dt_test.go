package drawing

import (
	"context"
	"testing"
	"time"

	"haruki-cloud/internal/pjsk/displaytime"
)

func TestPrepareDrawingRequestBodyInjectsProfileUpdateTime(t *testing.T) {
	now := time.Unix(1776322800, 0)
	body := prepareDrawingRequestBody("/api/pjsk/profile", &ProfileRequest{
		Profile: BasicProfile{
			ID:              "123",
			Region:          "JP",
			Nickname:        "tester",
			LeaderImagePath: "asset/test.png",
		},
	}, now, nil)

	root := mapAt(body)
	if root == nil {
		t.Fatalf("expected request body map, got %#v", body)
	}
	got, ok := parseDrawingUpdateTime(root["update_time"])
	if !ok {
		t.Fatalf("expected update_time value, got %#v", root["update_time"])
	}
	if got != now.UnixMilli() {
		t.Fatalf("expected update_time %d, got %d", now.UnixMilli(), got)
	}
}

func TestPrepareDrawingRequestBodyInjectsProfileCardDataSourceUpdateTime(t *testing.T) {
	now := time.Unix(1776322800, 0)
	body := prepareDrawingRequestBody("/api/pjsk/music/progress", &PlayProgressRequest{
		Difficulty: "expert",
		Profile: ProfileCardRequest{
			Profile: &BasicProfile{
				ID:              "123",
				Region:          "JP",
				Nickname:        "tester",
				LeaderImagePath: "asset/test.png",
			},
			DataSources: []ProfileDataSource{
				{Name: "User Data"},
			},
		},
	}, now, nil)

	root := mapAt(body)
	profile := mapAt(root, "profile")
	sources := sliceAt(profile, "data_sources")
	if len(sources) != 1 {
		t.Fatalf("expected 1 data source, got %d", len(sources))
	}
	source := mapAt(sources[0])
	got, ok := parseDrawingUpdateTime(source["update_time"])
	if !ok {
		t.Fatalf("expected nested update_time value, got %#v", source["update_time"])
	}
	if got != now.UnixMilli() {
		t.Fatalf("expected nested update_time %d, got %d", now.UnixMilli(), got)
	}
}

func TestPrepareDrawingRequestBodyNormalizesDetailedProfileUpdateTimeToMilliseconds(t *testing.T) {
	now := time.Unix(1776322800, 0)
	body := prepareDrawingRequestBody("/api/pjsk/card/list", &CardListRequest{
		Region: "JP",
		UserInfo: &DetailedProfileCardRequest{
			ID:              "123",
			Region:          "JP",
			Nickname:        "tester",
			Source:          "sekai_api_public",
			UpdateTime:      now.Unix(),
			LeaderImagePath: "asset/test.png",
		},
	}, now, nil)

	root := mapAt(body)
	userInfo := mapAt(root, "user_info")
	if userInfo == nil {
		t.Fatalf("expected user_info payload, got %#v", body)
	}
	got, ok := parseDrawingUpdateTime(userInfo["update_time"])
	if !ok {
		t.Fatalf("expected normalized update_time value, got %#v", userInfo["update_time"])
	}
	if got != now.UnixMilli() {
		t.Fatalf("expected normalized update_time %d, got %d", now.UnixMilli(), got)
	}
}

func TestPrepareDrawingRequestBodyInjectsTimeZoneFromContext(t *testing.T) {
	now := time.Unix(1776322800, 0)
	ctx := displaytime.WithRequestTimeZone(context.Background(), "Asia/Tokyo")
	body := prepareDrawingRequestBody("/api/pjsk/event/list", &EventListRequest{}, now, ctx)

	root := mapAt(body)
	if root == nil {
		t.Fatalf("expected request body map, got %#v", body)
	}
	if got := scalarString(root["timezone"]); got != "Asia/Tokyo" {
		t.Fatalf("expected timezone Asia/Tokyo, got %q", got)
	}
}

func TestPrepareDrawingRequestBodyKeepsExplicitTimeZone(t *testing.T) {
	now := time.Unix(1776322800, 0)
	ctx := displaytime.WithRequestTimeZone(context.Background(), "Asia/Tokyo")
	body := prepareDrawingRequestBody("/api/pjsk/event/list", map[string]any{
		"timezone": "Asia/Seoul",
	}, now, ctx)

	root := mapAt(body)
	if root == nil {
		t.Fatalf("expected request body map, got %#v", body)
	}
	if got := scalarString(root["timezone"]); got != "Asia/Seoul" {
		t.Fatalf("expected explicit timezone Asia/Seoul, got %q", got)
	}
}

func TestPrepareDrawingRequestBodyInjectsRootDT(t *testing.T) {
	now := time.Unix(1776322800, 0)
	ctx := displaytime.WithRequestTimeZone(context.Background(), "Asia/Tokyo")
	body := prepareDrawingRequestBody("/api/pjsk/event/list", &EventListRequest{}, now, ctx)

	root := mapAt(body)
	if root == nil {
		t.Fatalf("expected request body map, got %#v", body)
	}
	got, ok := parseDrawingUpdateTime(root["dt"])
	if !ok {
		t.Fatalf("expected dt value, got %#v", root["dt"])
	}
	if got != now.UnixMilli() {
		t.Fatalf("expected dt %d, got %d", now.UnixMilli(), got)
	}
}

func TestPrepareDrawingRequestBodyInjectsDTAndTimeZoneIntoSliceRequests(t *testing.T) {
	now := time.Unix(1776322800, 0)
	ctx := displaytime.WithRequestTimeZone(context.Background(), "Asia/Tokyo")
	body := prepareDrawingRequestBody("/api/pjsk/score/music-meta", []MusicMetaRequest{{
		MusicID:        1,
		MusicTitle:     "test",
		MusicCoverPath: "music/jacket/test.png",
	}}, now, ctx)

	items := sliceAt(body)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	root := mapAt(items[0])
	if root == nil {
		t.Fatalf("expected request item map, got %#v", items[0])
	}
	if got := scalarString(root["timezone"]); got != "Asia/Tokyo" {
		t.Fatalf("expected timezone Asia/Tokyo, got %q", got)
	}
	got, ok := parseDrawingUpdateTime(root["dt"])
	if !ok {
		t.Fatalf("expected dt value, got %#v", root["dt"])
	}
	if got != now.UnixMilli() {
		t.Fatalf("expected dt %d, got %d", now.UnixMilli(), got)
	}
}
