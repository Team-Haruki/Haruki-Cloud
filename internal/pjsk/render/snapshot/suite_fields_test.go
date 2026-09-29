package snapshot

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	json "haruki-cloud/internal/jsonutil"
	renderregion "haruki-cloud/internal/pjsk/region"
)

func TestNormalizeSuiteFields(t *testing.T) {
	input := []string{" userCards ", "userGamedata", "userCards"}
	original := slices.Clone(input)
	got, err := normalizeSuiteFields(input)
	if err != nil || !reflect.DeepEqual(got, []string{"upload_time", "userCards", "userGamedata"}) {
		t.Fatalf("fields = %v, %v", got, err)
	}
	if !reflect.DeepEqual(input, original) {
		t.Fatal("caller field slice mutated")
	}
	if got, err := normalizeSuiteFields(nil); err != nil || got != nil {
		t.Fatalf("full Suite = %v, %v", got, err)
	}
	for _, field := range []string{"", " ", "a,b", "updatedResources.userCards", "$where"} {
		if _, err := normalizeSuiteFields([]string{field}); err == nil {
			t.Fatalf("accepted invalid field %q", field)
		}
	}
}

func TestSuiteProjectionIsolatesAllCachesAndKeepsAuthorization(t *testing.T) {
	client := &fakePrivateDataClient{
		suiteJSON:   []byte(`{"upload_time":1710000000,"userGamedata":{"userId":123456789},"userCards":[{"cardId":1}],"userProfile":{"word":"full"}}`),
		mysekaiJSON: []byte(`{"upload_time":1710000000,"updatedResources":{"userMysekaiUnknown":{"id":339871638031728641}}}`),
		uploadTime:  "1710000000",
	}
	provider := newValidToolboxProvider(client).WithPrivateDataCache(NewPrivateDataCache()).WithBuiltSnapshotCache(NewBuiltSnapshotCache())
	var builds int
	factory := provider.factory
	provider.factory = snapshotFactoryFunc(func(ctx context.Context, input BuildInput) (Snapshot, error) {
		builds++
		return factory.Build(ctx, input)
	})
	ctx := WithRequestCache(t.Context())
	resolve := func(ctx context.Context, fields []string) Snapshot {
		t.Helper()
		snap, err := provider.Resolve(ctx, validToolboxSelector(), ResolveOptions{NeedMySekai: true, SuiteFields: fields})
		if err != nil {
			t.Fatal(err)
		}
		return snap
	}
	projected := resolve(ctx, []string{"userCards"})
	same := resolve(ctx, []string{"upload_time", " userCards ", "userGamedata", "userCards"})
	if same != projected || len(client.suiteKnownTimes) != 1 || builds != 1 {
		t.Fatal("normalized scopes did not share per-request work")
	}
	other := resolve(ctx, []string{"userProfile"})
	full := resolve(ctx, nil)
	if len(other.RawData().UserCards) != 0 || len(full.RawData().UserCards) != 1 || projected.RawData().UserProfile.Word != "" || full.RawData().UserProfile.Word != "full" {
		t.Fatal("different projections reused a partial or full snapshot")
	}
	if builds != 3 || !reflect.DeepEqual(client.suiteKnownTimes, []int64{0, 0, 0}) {
		t.Fatalf("cold scopes builds=%d known=%v", builds, client.suiteKnownTimes)
	}
	if len(client.mysekaiKnownTimes) != 1 {
		t.Fatal("full MySekai did not share the existing request cache")
	}
	raw, _ := projected.RawBytes()
	if !strings.Contains(string(raw), `"userMysekaiUnknown":{"id":339871638031728641}`) {
		t.Fatalf("full MySekai unknown data was lost: %s", raw)
	}
	if warm := resolve(WithRequestCache(t.Context()), []string{"userCards"}); warm != projected {
		t.Fatal("same projection did not reuse validated built snapshot")
	}
	if builds != 3 || !reflect.DeepEqual(client.suiteKnownTimes, []int64{0, 0, 0, 1710000000}) {
		t.Fatal("warm projection did not authorize its own read")
	}
	client.suiteErr = errors.New("revoked")
	if snap, err := provider.Resolve(WithRequestCache(t.Context()), validToolboxSelector(), ResolveOptions{SuiteFields: []string{"userCards"}}); snap != nil || !errors.Is(err, client.suiteErr) {
		t.Fatalf("revoked cached read = %v, %v", snap, err)
	}
}

func TestBuiltSnapshotFlightsSeparateSuiteProjections(t *testing.T) {
	cache := NewBuiltSnapshotCache()
	started := make(chan string, 2)
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	results := make(chan error, 2)
	for _, projection := range []string{"upload_time,userGamedata,userCards", "upload_time,userGamedata,userProfile"} {
		go func() {
			key := builtSnapshotKey{Region: "jp", UID: 1, SuiteUploadTime: 100, SuiteProjection: projection}
			_, _, err := cache.getOrBuild(t.Context(), key, 100, func(ctx context.Context) (Snapshot, error) {
				started <- projection
				select {
				case <-release:
				case <-ctx.Done():
					return nil, ctx.Err()
				}
				return NewDefaultSnapshotFactory(nil, nil).Build(ctx, BuildInput{Region: renderregion.JP, SuiteJSON: []byte(minimalSuiteJSON)})
			})
			results <- err
		}()
	}
	for range 2 {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("different projection joined an existing flight")
		}
	}
	releaseOnce.Do(func() { close(release) })
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
}

func BenchmarkSuiteProjectionBuild(b *testing.B) {
	document := map[string]json.RawMessage{}
	if err := json.Unmarshal([]byte(minimalSuiteJSON), &document); err != nil {
		b.Fatal(err)
	}
	rows := strings.TrimSuffix(strings.Repeat(`{"musicId":1,"musicDifficulty":"expert","playResult":"full_combo"},`, 20000), ",")
	document["userMusicResults"] = json.RawMessage("[" + rows + "]")
	full, err := json.Marshal(document)
	if err != nil {
		b.Fatal(err)
	}
	projected := map[string]json.RawMessage{}
	for _, key := range []string{"upload_time", "userGamedata", "userCards", "userDecks", "userProfile"} {
		projected[key] = document[key]
	}
	slim, err := json.Marshal(projected)
	if err != nil {
		b.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		suite []byte
	}{{"full", full}, {"command_fields", slim}} {
		b.Run(tc.name, func(b *testing.B) {
			factory := NewDefaultSnapshotFactory(nil, nil)
			b.ReportAllocs()
			for b.Loop() {
				if _, err := factory.Build(b.Context(), BuildInput{Region: renderregion.JP, SuiteJSON: tc.suite, MySekaiJSON: []byte(`{"upload_time":1710000000,"updatedResources":{"userMysekaiShops":[]}}`)}); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(len(tc.suite)), "suite-B/op")
		})
	}
}
