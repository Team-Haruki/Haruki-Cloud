package deck

import (
	"context"
	"errors"
	"strings"
	"testing"

	"haruki-cloud/internal/observability/commandtrace"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/snapshot"
)

type stageTraceSnapshot struct {
	snapshot.Snapshot
	bytes    []byte
	bytesErr error
	noRaw    bool
}

func (s *stageTraceSnapshot) RawBytes() ([]byte, error) {
	if s.bytesErr != nil || s.bytes != nil {
		return s.bytes, s.bytesErr
	}
	return s.Snapshot.RawBytes()
}

func (s *stageTraceSnapshot) RawData() *snapshot.RawUserData {
	if s.noRaw {
		return nil
	}
	return s.Snapshot.RawData()
}

func TestPrepareRecommendUserDataStageTrace(t *testing.T) {
	copyErr := errors.New("snapshot bytes unavailable")
	for _, tc := range []struct {
		name        string
		query       AutoQuery
		bytes       []byte
		bytesErr    error
		noRaw       bool
		wantError   string
		wantPrepare int
		wantCards   int
	}{
		{name: "unchanged", wantCards: 5},
		{name: "filtered", query: AutoQuery{ExcludedCards: []int{1001}}, wantPrepare: 1, wantCards: 4},
		{name: "copy error", bytesErr: copyErr, wantError: copyErr.Error()},
		{name: "missing typed data", noRaw: true, wantError: "raw user snapshot is unavailable"},
		{name: "prepare error", query: AutoQuery{ExcludedCards: []int{1001}}, bytes: []byte("{"), wantPrepare: 1, wantError: "decode original user snapshot"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			controller := newTestDeckController(t, RecommendConfig{})
			ctx, trace := commandtrace.WithTrace(context.Background())
			controller = controller.WithContext(ctx)
			original := controller.snapshot.RawData()
			controller.snapshot = &stageTraceSnapshot{
				Snapshot: controller.snapshot, bytes: tc.bytes, bytesErr: tc.bytesErr, noRaw: tc.noRaw,
			}
			raw, encoded, err := controller.prepareRecommendUserData(renderregion.JP, "no_event", tc.query, map[string]any{})
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("prepare error = %v, want %q", err, tc.wantError)
				}
				if tc.bytesErr != nil && !errors.Is(err, tc.bytesErr) {
					t.Fatalf("copy error lost identity: %v", err)
				}
			} else if err != nil || raw == nil || len(raw.UserCards) != tc.wantCards || len(encoded) == 0 {
				t.Fatalf("prepared result = %+v, %d bytes, %v", raw, len(encoded), err)
			}
			if len(original.UserCards) != 5 {
				t.Fatalf("preparation changed original cards: %d", len(original.UserCards))
			}
			for name, want := range map[string]int{"deck.userdata_copy": 1, "deck.userdata_prepare": tc.wantPrepare} {
				operation, _ := traceOperation(trace.Snapshot(), name)
				if operation.Count != want {
					t.Errorf("%s count = %d, want %d", name, operation.Count, want)
				}
			}
		})
	}
}

func TestResolveAutoRecommendResourcesRecordsRequestMetadataLookup(t *testing.T) {
	for _, present := range []bool{true, false} {
		t.Run(map[bool]string{true: "available", false: "missing"}[present], func(t *testing.T) {
			meta := &testMusicMetaSource{}
			if present {
				meta.data = []byte(`[{"music_id":1}]`)
			}
			controller := newTestDeckControllerWithMeta(t, RecommendConfig{}, meta)
			controller.engine = &remoteEngineProvider{recommenders: map[string]PjskDeckRecommender{"jp": &refactorCoverageRecommender{}}}
			ctx, trace := commandtrace.WithTrace(context.Background())
			resources, err := controller.resolveAutoRecommendResources(ctx, AutoQuery{Region: "jp", RecommendType: "no_event"})
			if present && (err != nil || string(resources.musicMeta) != string(meta.data)) {
				t.Fatalf("metadata = %q, %v", resources.musicMeta, err)
			}
			if !present && err == nil {
				t.Fatal("expected missing metadata error")
			}
			operation, _ := traceOperation(trace.Snapshot(), "deck.music_meta_lookup")
			if operation.Count != 1 {
				t.Fatalf("metadata lookup count = %d, want 1", operation.Count)
			}
		})
	}
}
