package snapshot

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	sekaienttest "haruki-cloud/database/sekai/enttest"
	"haruki-cloud/internal/pjsk/accountdata"
	renderregion "haruki-cloud/internal/pjsk/region"

	_ "github.com/mattn/go-sqlite3"
)

type fakeMusicMetaSource struct {
	payloads map[string][]byte
	calls    int
}

func (f *fakeMusicMetaSource) Get(region string) []byte {
	if f == nil {
		return nil
	}
	f.calls++
	return append([]byte(nil), f.payloads[region]...)
}

type fakeBindingLookup struct {
	resolveCalls []string
	bindings     map[string]*accountdata.ResolvedBinding
	listItems    []accountdata.BindingListItem
}

func (f *fakeBindingLookup) ResolveUserBinding(_ context.Context, _, _, server string) (int, *accountdata.ResolvedBinding, error) {
	f.resolveCalls = append(f.resolveCalls, server)
	item, ok := f.bindings[server]
	if !ok {
		return 0, nil, errors.New("binding not found")
	}
	return 1, item, nil
}

func (f *fakeBindingLookup) List(_ context.Context, _, _ string) ([]accountdata.BindingListItem, error) {
	return append([]accountdata.BindingListItem(nil), f.listItems...), nil
}

type fakePrivateDataClient struct {
	suiteCalls             []string
	mysekaiCalls           []string
	suiteCtx               context.Context
	mysekaiCtx             context.Context
	suiteJSON              []byte
	mysekaiJSON            []byte
	uploadTime             string
	suiteUploadTimeCalls   int
	mysekaiUploadTimeCalls int
	uploadTimeErr          error
}

func (f *fakePrivateDataClient) GetSuiteDataContext(ctx context.Context, server string, userID int64, platform, platformUserID string) ([]byte, error) {
	f.suiteCtx = ctx
	f.suiteCalls = append(f.suiteCalls, server+":"+platform+":"+platformUserID)
	return append([]byte(nil), f.suiteJSON...), nil
}

func (f *fakePrivateDataClient) GetMySekaiDataContext(ctx context.Context, server string, userID int64, platform, platformUserID string) ([]byte, error) {
	f.mysekaiCtx = ctx
	f.mysekaiCalls = append(f.mysekaiCalls, server+":"+platform+":"+platformUserID)
	return append([]byte(nil), f.mysekaiJSON...), nil
}

func (f *fakePrivateDataClient) parseUploadTime() (int64, error) {
	if f.uploadTimeErr != nil {
		return 0, f.uploadTimeErr
	}
	trimmed := strings.TrimSpace(f.uploadTime)
	if trimmed == "" {
		return 0, nil
	}
	return strconv.ParseInt(trimmed, 10, 64)
}

func (f *fakePrivateDataClient) GetSuiteUploadTimeContext(_ context.Context, _ string, _ int64, _, _ string) (int64, error) {
	f.suiteUploadTimeCalls++
	return f.parseUploadTime()
}

func (f *fakePrivateDataClient) GetMySekaiUploadTimeContext(_ context.Context, _ string, _ int64, _, _ string) (int64, error) {
	f.mysekaiUploadTimeCalls++
	return f.parseUploadTime()
}

func TestToolboxSnapshotProviderFallsBackToRegionBinding(t *testing.T) {
	provider := NewToolboxSnapshotProvider(
		&fakeBindingLookup{
			bindings: map[string]*accountdata.ResolvedBinding{
				"jp": {
					PJSKUserID:     "123456789",
					Server:         "jp",
					SuiteVisible:   true,
					MySekaiVisible: true,
				},
			},
		},
		&fakePrivateDataClient{
			suiteJSON:   []byte(minimalSuiteJSON),
			mysekaiJSON: []byte(`{"updatedResources":{}}`),
			uploadTime:  "1710000000",
		},
		nil,
		nil,
	)

	snapshot, err := provider.Resolve(context.Background(), Selector{
		IMPlatform: "qq",
		IMUserID:   "10001",
		Region:     renderregion.JP,
	}, ResolveOptions{
		PreferGlobalDefault: true,
		NeedMySekai:         true,
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if err := snapshot.Require(); err != nil {
		t.Fatalf("snapshot.Require() error = %v", err)
	}
	raw := snapshot.RawData()
	if raw == nil || raw.UserGamedata.UserID != 123456789 {
		t.Fatalf("unexpected raw snapshot: %+v", raw)
	}
}

func TestToolboxSnapshotProviderReusesRequestCachedSuiteData(t *testing.T) {
	client := &fakePrivateDataClient{
		suiteJSON:   []byte(minimalSuiteJSON),
		mysekaiJSON: []byte(`{"updatedResources":{}}`),
	}
	provider := NewToolboxSnapshotProvider(
		&fakeBindingLookup{
			bindings: map[string]*accountdata.ResolvedBinding{
				"jp": {
					PJSKUserID:     "123456789",
					Server:         "jp",
					SuiteVisible:   true,
					MySekaiVisible: true,
				},
			},
		},
		client,
		nil,
		nil,
	)

	ctx := WithRequestCache(context.Background())
	selector := Selector{
		IMPlatform: "qq",
		IMUserID:   "10001",
		Region:     renderregion.JP,
	}
	if _, err := provider.Resolve(ctx, selector, ResolveOptions{NeedMySekai: false}); err != nil {
		t.Fatalf("Resolve(suite) error = %v", err)
	}
	if _, err := provider.Resolve(ctx, selector, ResolveOptions{NeedMySekai: true}); err != nil {
		t.Fatalf("Resolve(mysekai) error = %v", err)
	}

	if got := len(client.suiteCalls); got != 1 {
		t.Fatalf("expected one suite fetch with request cache, got %d", got)
	}
	if got := len(client.mysekaiCalls); got != 1 {
		t.Fatalf("expected one mysekai fetch, got %d", got)
	}
}

func TestToolboxSnapshotProviderPassesRequestContextToPrivateDataClient(t *testing.T) {
	client := &fakePrivateDataClient{
		suiteJSON:   []byte(minimalSuiteJSON),
		mysekaiJSON: []byte(`{"updatedResources":{}}`),
	}
	provider := NewToolboxSnapshotProvider(
		&fakeBindingLookup{
			bindings: map[string]*accountdata.ResolvedBinding{
				"jp": {
					PJSKUserID:     "123456789",
					Server:         "jp",
					SuiteVisible:   true,
					MySekaiVisible: true,
				},
			},
		},
		client,
		nil,
		nil,
	)

	type contextKey struct{}
	ctx := context.WithValue(context.Background(), contextKey{}, "request")
	if _, err := provider.Resolve(ctx, Selector{
		IMPlatform: "qq",
		IMUserID:   "10001",
		Region:     renderregion.JP,
	}, ResolveOptions{NeedMySekai: true}); err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if client.suiteCtx != ctx {
		t.Fatal("suite request did not receive the caller context")
	}
	if client.mysekaiCtx != ctx {
		t.Fatal("mysekai request did not receive the caller context")
	}
}

func TestToolboxSnapshotProviderSupportsExplicitBoundAccount(t *testing.T) {
	client := &fakePrivateDataClient{
		suiteJSON: []byte(minimalSuiteJSON),
	}
	provider := NewToolboxSnapshotProvider(
		&fakeBindingLookup{
			listItems: []accountdata.BindingListItem{
				{
					BindingID:      1,
					Server:         "jp",
					UserID:         "123456789",
					SuiteVisible:   true,
					MySekaiVisible: true,
				},
			},
		},
		client,
		nil,
		nil,
	)

	snapshot, err := provider.Resolve(context.Background(), Selector{
		IMPlatform: "qq",
		IMUserID:   "10001",
		Region:     renderregion.JP,
		PJSKUserID: "123456789",
	}, ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if err := snapshot.Require(); err != nil {
		t.Fatalf("snapshot.Require() error = %v", err)
	}
	if len(client.suiteCalls) != 1 {
		t.Fatalf("expected exactly one suite call, got %d", len(client.suiteCalls))
	}
}

func TestToolboxSnapshotProviderInjectsMusicMetaPayload(t *testing.T) {
	provider := NewToolboxSnapshotProvider(
		&fakeBindingLookup{
			bindings: map[string]*accountdata.ResolvedBinding{
				"jp": {
					PJSKUserID:     "123456789",
					Server:         "jp",
					SuiteVisible:   true,
					MySekaiVisible: true,
				},
			},
		},
		&fakePrivateDataClient{
			suiteJSON: []byte(minimalSuiteJSON),
		},
		nil,
		nil,
	).WithMusicMetaSource(&fakeMusicMetaSource{
		payloads: map[string][]byte{
			"jp": []byte(`[{"music_id":10000,"difficulty":"master"}]`),
		},
	})

	snapshot, err := provider.Resolve(context.Background(), Selector{
		IMPlatform: "qq",
		IMUserID:   "10001",
		Region:     renderregion.JP,
	}, ResolveOptions{NeedMusicMeta: true})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if len(snapshot.MusicMetaBytes()) == 0 {
		t.Fatalf("expected music meta bytes on toolbox snapshot")
	}
}

func TestToolboxSnapshotProviderSkipsMusicMetaUnlessRequested(t *testing.T) {
	metas := &fakeMusicMetaSource{payloads: map[string][]byte{"jp": []byte(`[]`)}}
	provider := NewToolboxSnapshotProvider(
		&fakeBindingLookup{bindings: map[string]*accountdata.ResolvedBinding{
			"jp": {
				PJSKUserID:   "123456789",
				Server:       "jp",
				SuiteVisible: true,
			},
		}},
		&fakePrivateDataClient{suiteJSON: []byte(minimalSuiteJSON)},
		nil,
		nil,
	).WithMusicMetaSource(metas)

	_, err := provider.Resolve(context.Background(), Selector{
		IMPlatform: "qq",
		IMUserID:   "10001",
		Region:     renderregion.JP,
	}, ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if metas.calls != 0 {
		t.Fatalf("metadata Get calls = %d, want 0", metas.calls)
	}
}

func TestToolboxSnapshotProviderBuildsSnapshotUsingBindingServerRegion(t *testing.T) {
	sekaiClient := sekaienttest.Open(t, "sqlite3", fmt.Sprintf("file:toolbox_snapshot_region_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano()))
	t.Cleanup(func() { _ = sekaiClient.Close() })

	if _, err := sekaiClient.Card.Create().
		SetServerRegion("en").
		SetGameID(1001).
		SetCharacterID(1).
		SetAssetbundleName("res001_no001").
		Save(context.Background()); err != nil {
		t.Fatalf("create card: %v", err)
	}

	provider := NewToolboxSnapshotProvider(
		&fakeBindingLookup{
			bindings: map[string]*accountdata.ResolvedBinding{
				"jp": {
					PJSKUserID:     "123456789",
					Server:         "en",
					SuiteVisible:   true,
					MySekaiVisible: true,
				},
			},
		},
		&fakePrivateDataClient{
			suiteJSON: []byte(minimalSuiteJSON),
		},
		sekaiClient,
		nil,
	)

	snapshot, err := provider.Resolve(context.Background(), Selector{
		IMPlatform: "qq",
		IMUserID:   "10001",
		Region:     renderregion.JP,
	}, ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	profile := snapshot.DetailedProfile(renderregion.JP)
	if profile == nil {
		t.Fatal("expected detailed profile")
	}
	if profile.LeaderImagePath == "" {
		t.Fatal("expected leader image path")
	}
	if profile.LeaderImagePath == "static_images/unknown.jpg" {
		t.Fatalf("expected real leader image path, got placeholder %q", profile.LeaderImagePath)
	}
	if wantPrefix := "asset/en-assets/startapp/thumbnail/chara/res001_no001_after_training.png"; profile.LeaderImagePath != wantPrefix {
		t.Fatalf("unexpected leader image path: %q", profile.LeaderImagePath)
	}
}

const minimalSuiteJSON = `{
  "now": 1710000000,
  "upload_time": 1710000000,
  "userGamedata": {"userId": 123456789, "name": "SnapshotUser", "deck": 1, "rank": 100, "coin": 0},
  "userProfile": {"profileImageType": "default", "profileImageId": 0, "word": "", "twitterId": ""},
  "userDecks": [{"deckId": 1, "leader": 1001, "subLeader": 0, "member1": 1001, "member2": 0, "member3": 0, "member4": 0, "member5": 0}],
  "userCards": [{"cardId": 1001, "level": 60, "masterRank": 0, "specialTrainingStatus": "done", "defaultImage": "special_training", "episodes": []}],
  "userMusicResults": [],
  "userChallengeLiveSoloResults": [],
  "userChallengeLiveSoloStages": [],
  "userChallengeLiveSoloHighScoreRewards": []
}`
