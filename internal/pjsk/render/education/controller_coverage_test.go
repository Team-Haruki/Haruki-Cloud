package education

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	rendersnapshot "haruki-cloud/internal/pjsk/render/snapshot"
)

type educationSnapshotStub struct {
	err       error
	profile   *drawing.DetailedProfileCardRequest
	challenge *rendersnapshot.ChallengeLiveData
	raw       *rendersnapshot.RawUserData
}

func (s *educationSnapshotStub) Require() error { return s.err }

func (s *educationSnapshotStub) DetailedProfile(renderregion.Value) *drawing.DetailedProfileCardRequest {
	return s.profile
}

func (*educationSnapshotStub) ProfileCard(renderregion.Value) *drawing.ProfileCardRequest { return nil }
func (*educationSnapshotStub) MusicResults(string) map[int]string                         { return nil }
func (*educationSnapshotStub) GetMusicResult(int, string) string                          { return "" }
func (s *educationSnapshotStub) ChallengeLive() *rendersnapshot.ChallengeLiveData         { return s.challenge }
func (*educationSnapshotStub) RawBytes() ([]byte, error)                                  { return nil, nil }
func (*educationSnapshotStub) RawValue(string) ([]byte, error)                            { return nil, nil }
func (*educationSnapshotStub) RawFilePath() string                                        { return "" }
func (s *educationSnapshotStub) RawData() *rendersnapshot.RawUserData                     { return s.raw }
func (*educationSnapshotStub) MusicMetaBytes() []byte                                     { return nil }
func (*educationSnapshotStub) MusicMetaPath() string                                      { return "" }

func newEducationDrawingTestServer(t *testing.T) (*httptest.Server, *drawing.HarukiDrawingClient) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		_, _ = w.Write([]byte("PNG:" + r.URL.Path))
	}))
	t.Cleanup(server.Close)
	client := drawing.NewHarukiDrawingClient(server.URL, drawing.WithRetryCount(0), drawing.WithTimeout(time.Second))
	return server, client
}

func TestEducationControllerContextAndChallengeValidation(t *testing.T) {
	assertEducationControllerContext(t)
	assertEducationChallengeValidation(t)
}

func assertEducationControllerContext(t *testing.T) {
	t.Helper()
	var nilController *Controller
	if got := nilController.WithContext(context.Background()); got != nil {
		t.Fatalf("nil WithContext() = %#v", got)
	}
	if nilController.traceContext() != nil {
		t.Fatal("nil traceContext() is non-nil")
	}
	nilController.RegisterSource(&testSource{})
	if _, err := nilController.BuildChallengeLiveDetailsRequest(ChallengeLiveQuery{}); err == nil {
		t.Fatal("nil controller BuildChallengeLiveDetailsRequest() error = nil")
	}

	controller := NewController(nil, nil, nil, renderregion.JP)
	plainSource := &testSource{region: renderregion.JP}
	controller.RegisterSource(plainSource)
	ctx := context.WithValue(context.Background(), educationContextKey("controller-coverage"), "present")
	clone := controller.WithContext(ctx)
	if clone == controller || clone.traceContext() != ctx {
		t.Fatalf("WithContext() = %#v, context = %#v", clone, clone.traceContext())
	}
	if got, ok := clone.sources.SourceForRegion(renderregion.JP); !ok || got != plainSource {
		t.Fatalf("cloned source = %#v, ok=%v", got, ok)
	}
}

func assertEducationChallengeValidation(t *testing.T) {
	t.Helper()
	plainSource := &testSource{region: renderregion.JP}
	controller := NewController(nil, nil, nil, renderregion.JP)
	controller.RegisterSource(plainSource)
	if _, err := controller.BuildChallengeLiveDetailsRequest(ChallengeLiveQuery{}); err == nil || !strings.Contains(err.Error(), "snapshot") {
		t.Fatalf("missing snapshot error = %v", err)
	}
	wantErr := errors.New("snapshot rejected")
	if _, err := NewController(nil, nil, &educationSnapshotStub{err: wantErr}, renderregion.JP).
		BuildChallengeLiveDetailsRequest(ChallengeLiveQuery{}); !errors.Is(err, wantErr) {
		t.Fatalf("Require() error = %v, want %v", err, wantErr)
	}

	profile := &drawing.DetailedProfileCardRequest{ID: "1001", Region: "JP", Nickname: "tester"}
	validSnapshot := &educationSnapshotStub{profile: profile, challenge: &rendersnapshot.ChallengeLiveData{}}
	if _, err := NewController(nil, nil, validSnapshot, renderregion.JP).
		BuildChallengeLiveDetailsRequest(ChallengeLiveQuery{}); err == nil || !strings.Contains(err.Error(), "data source") {
		t.Fatalf("missing source error = %v", err)
	}

	controller = NewController(nil, nil, &educationSnapshotStub{profile: profile}, renderregion.JP)
	controller.RegisterSource(plainSource)
	if _, err := controller.BuildChallengeLiveDetailsRequest(ChallengeLiveQuery{}); err == nil || !strings.Contains(err.Error(), "challenge live") {
		t.Fatalf("missing challenge error = %v", err)
	}

	controller = NewController(nil, nil, &educationSnapshotStub{challenge: &rendersnapshot.ChallengeLiveData{}}, renderregion.JP)
	controller.RegisterSource(plainSource)
	if _, err := controller.BuildChallengeLiveDetailsRequest(ChallengeLiveQuery{}); err == nil || !strings.Contains(err.Error(), "profile") {
		t.Fatalf("missing profile error = %v", err)
	}
}

func TestEducationControllerBuildAndRenderChallengeLive(t *testing.T) {
	_, client := newEducationDrawingTestServer(t)
	profile := &drawing.DetailedProfileCardRequest{ID: "1001", Region: "JP", Nickname: "tester"}
	snap := &educationSnapshotStub{
		profile: profile,
		challenge: &rendersnapshot.ChallengeLiveData{
			Results: []rendersnapshot.ChallengeLiveResult{
				{CharacterID: 2, HighScore: 1_000_000},
				{CharacterID: 1, HighScore: 4_000_000},
			},
			Stages: []rendersnapshot.ChallengeLiveStage{
				{CharacterID: 1, Rank: 2},
				{CharacterID: 1, Rank: 5},
				{CharacterID: 1, Rank: 3},
			},
			Rewards: []rendersnapshot.ChallengeLiveReward{{RewardID: 11, CharacterID: 1}},
		},
	}
	source := &challengeTestSource{
		testSource: &testSource{
			region: renderregion.JP,
			boxes: map[string]map[int]*ResourceBox{
				"challenge_live_high_score": {
					1: {
						ID: 1,
						Details: []ResourceBoxDetail{
							{ResourceType: "JEWEL", ResourceQuantity: 100},
							{ResourceType: "material", ResourceID: 15, ResourceQuantity: 10},
							{ResourceType: "material", ResourceID: 7, ResourceQuantity: 999},
						},
					},
				},
			},
		},
		rewards: map[int][]*ChallengeReward{
			1: {
				nil,
				{ID: 11, CharacterID: 1, HighScore: 100_000, ResourceBoxID: 1},
				{ID: 12, CharacterID: 1, HighScore: 3_500_000, ResourceBoxID: 1},
				{ID: 13, CharacterID: 1, HighScore: 3_600_000, ResourceBoxID: 999},
			},
		},
	}
	controller := NewController(client, nil, nil, renderregion.JP)
	controller.RegisterSource(source)

	payload, err := controller.BuildChallengeLiveDetailsRequest(ChallengeLiveQuery{
		Region:   renderregion.Unknown,
		Profile:  profile,
		Snapshot: snap,
	})
	if err != nil {
		t.Fatalf("BuildChallengeLiveDetailsRequest() error = %v", err)
	}
	if len(payload.CharacterChallenges) != 26 {
		t.Fatalf("character challenges = %d, want 26", len(payload.CharacterChallenges))
	}
	first := payload.CharacterChallenges[0]
	if first.CharaID != 1 || first.Rank != 5 || first.Score != 4_000_000 || first.Jewel != 100 || first.Shard != 10 {
		t.Fatalf("first challenge = %+v", first)
	}
	if first.CharaIconPath == "" || payload.MaxScore != 4_000_000 || payload.JewelIconPath == nil || payload.ShardIconPath == nil {
		t.Fatalf("challenge payload paths/max = %+v", payload)
	}

	body, err := controller.RenderChallengeLiveDetails(ChallengeLiveQuery{Profile: profile, Snapshot: snap})
	if err != nil {
		t.Fatalf("RenderChallengeLiveDetails() error = %v", err)
	}
	if got, want := string(body), "PNG:/api/pjsk/education/challenge-live"; got != want {
		t.Fatalf("RenderChallengeLiveDetails() = %q, want %q", got, want)
	}

	if _, err := (*Controller)(nil).RenderChallengeLiveDetails(ChallengeLiveQuery{}); err == nil {
		t.Fatal("nil RenderChallengeLiveDetails() error = nil")
	}
	if _, err := NewController(client, nil, nil, renderregion.JP).RenderChallengeLiveDetails(ChallengeLiveQuery{}); err == nil {
		t.Fatal("invalid RenderChallengeLiveDetails() error = nil")
	}
}

func TestEducationControllerSimpleRenderEntrypoints(t *testing.T) {
	_, client := newEducationDrawingTestServer(t)
	controller := NewController(client, nil, nil, renderregion.JP)

	tests := []struct {
		name       string
		path       string
		render     func(*Controller) ([]byte, error)
		invalid    func(*Controller) error
		nilDrawing func(*Controller) error
	}{
		{
			name: "power bonus", path: "/api/pjsk/education/power-bonus",
			render: func(c *Controller) ([]byte, error) {
				return c.RenderPowerBonusDetail(drawing.PowerBonusDetailRequest{CharaBonuses: []drawing.CharacterBonus{{CharaID: 1}}})
			},
			invalid: func(c *Controller) error {
				_, err := c.RenderPowerBonusDetail(drawing.PowerBonusDetailRequest{})
				return err
			},
			nilDrawing: func(c *Controller) error {
				_, err := c.RenderPowerBonusDetail(drawing.PowerBonusDetailRequest{UnitBonuses: []drawing.UnitBonus{{Unit: "idol"}}})
				return err
			},
		},
		{
			name: "area item", path: "/api/pjsk/education/area-item",
			render: func(c *Controller) ([]byte, error) {
				return c.RenderAreaItemUpgradeMaterials(drawing.AreaItemUpgradeMaterialsRequest{AreaItems: []drawing.AreaItemInfo{{ItemID: 1}}})
			},
			invalid: func(c *Controller) error {
				_, err := c.RenderAreaItemUpgradeMaterials(drawing.AreaItemUpgradeMaterialsRequest{})
				return err
			},
			nilDrawing: func(c *Controller) error {
				_, err := c.RenderAreaItemUpgradeMaterials(drawing.AreaItemUpgradeMaterialsRequest{AreaItems: []drawing.AreaItemInfo{{ItemID: 1}}})
				return err
			},
		},
		{
			name: "bonds", path: "/api/pjsk/education/bonds",
			render: func(c *Controller) ([]byte, error) {
				return c.RenderBonds(drawing.BondsRequest{Bonds: []drawing.BondInfo{{CharaID1: 1, CharaID2: 2}}})
			},
			invalid: func(c *Controller) error {
				_, err := c.RenderBonds(drawing.BondsRequest{})
				return err
			},
			nilDrawing: func(c *Controller) error {
				_, err := c.RenderBonds(drawing.BondsRequest{Bonds: []drawing.BondInfo{{CharaID1: 1}}})
				return err
			},
		},
		{
			name: "leader count", path: "/api/pjsk/education/leader-count",
			render: func(c *Controller) ([]byte, error) {
				return c.RenderLeaderCount(drawing.LeaderCountRequest{LeaderCounts: []drawing.LeaderCountInfo{{CharaID: 1}}})
			},
			invalid: func(c *Controller) error {
				_, err := c.RenderLeaderCount(drawing.LeaderCountRequest{})
				return err
			},
			nilDrawing: func(c *Controller) error {
				_, err := c.RenderLeaderCount(drawing.LeaderCountRequest{LeaderCounts: []drawing.LeaderCountInfo{{CharaID: 1}}})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := tt.render(controller)
			if err != nil {
				t.Fatalf("render error = %v", err)
			}
			if got, want := string(body), "PNG:"+tt.path; got != want {
				t.Fatalf("render body = %q, want %q", got, want)
			}
			if err := tt.invalid(controller); err == nil {
				t.Fatal("invalid render error = nil")
			}
			if err := tt.nilDrawing(&Controller{}); err == nil {
				t.Fatal("nil drawing render error = nil")
			}
		})
	}
}

func TestEducationControllerCharacterMissionRenderEntrypoints(t *testing.T) {
	_, client := newEducationDrawingTestServer(t)
	controller := NewController(client, nil, nil, renderregion.JP)

	tests := []struct {
		name   string
		path   string
		render func(*Controller) ([]byte, error)
	}{
		{
			name: "overview", path: "/api/pjsk/education/character-mission-overview",
			render: func(c *Controller) ([]byte, error) {
				return c.RenderCharacterMissionOverview(drawing.CharacterMissionOverviewRequest{CharacterID: 1})
			},
		},
		{
			name: "all", path: "/api/pjsk/education/character-mission-all",
			render: func(c *Controller) ([]byte, error) {
				return c.RenderCharacterMissionAll(drawing.CharacterMissionAllRequest{CharacterID: 1, Title: "队长次数"})
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := tt.render(controller)
			if err != nil {
				t.Fatalf("render error = %v", err)
			}
			if got, want := string(body), "PNG:"+tt.path; got != want {
				t.Fatalf("render body = %q, want %q", got, want)
			}
			if _, err := tt.render(nil); err == nil {
				t.Fatal("nil drawing render error = nil")
			}
		})
	}
}

func TestEducationBuildSimpleRequests(t *testing.T) {
	controller := &Controller{}
	tests := []struct {
		name  string
		build func() (any, error)
	}{
		{
			name: "power unit only",
			build: func() (any, error) {
				return controller.BuildPowerBonusDetailRequest(drawing.PowerBonusDetailRequest{UnitBonuses: []drawing.UnitBonus{{Unit: "idol"}}})
			},
		},
		{
			name: "power attr only",
			build: func() (any, error) {
				return controller.BuildPowerBonusDetailRequest(drawing.PowerBonusDetailRequest{AttrBonuses: []drawing.AttrBonus{{Attr: "cute"}}})
			},
		},
		{
			name: "area",
			build: func() (any, error) {
				return controller.BuildAreaItemUpgradeMaterialsRequest(drawing.AreaItemUpgradeMaterialsRequest{AreaItems: []drawing.AreaItemInfo{{ItemID: 1}}})
			},
		},
		{
			name: "bonds",
			build: func() (any, error) {
				return controller.BuildBondsRequest(drawing.BondsRequest{Bonds: []drawing.BondInfo{{CharaID1: 1}}})
			},
		},
		{
			name: "leader",
			build: func() (any, error) {
				return controller.BuildLeaderCountRequest(drawing.LeaderCountRequest{LeaderCounts: []drawing.LeaderCountInfo{{CharaID: 1}}})
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, err := tt.build(); err != nil || got == nil {
				t.Fatalf("build = %#v, %v", got, err)
			}
		})
	}

	if got := fmt.Sprint(controller.traceContext()); got != "<nil>" {
		t.Fatalf("traceContext() = %q", got)
	}
}
