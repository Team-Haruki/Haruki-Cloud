package drawing

import (
	"testing"
	"time"
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
	}, now)

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
	}, now)

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
	}, now)

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
