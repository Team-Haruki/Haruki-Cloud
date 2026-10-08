package snapshot

import (
	"context"
	"testing"

	renderregion "haruki-cloud/internal/pjsk/region"
)

const ingestMySekaiJSON = `{"upload_time":1710000000,"updatedResources":{}}`

func TestPrivateDataCacheFetchReturnsCallerOwnedBytesOnMiss(t *testing.T) {
	cache := NewPrivateDataCache()
	source := []byte(`{"upload_time":9,"x":1}`)
	data, _, err := cache.Fetch(suiteKey(), func(int64) ([]byte, bool, error) { return source, false, nil })
	if err != nil {
		t.Fatal(err)
	}
	data[0] = 'x'
	cached, hit, err := cache.Fetch(suiteKey(), func(int64) ([]byte, bool, error) { return nil, true, nil })
	if err != nil || !hit || string(cached) != `{"upload_time":9,"x":1}` {
		t.Fatalf("caller mutation reached the cache: %q, %t, %v", cached, hit, err)
	}
}

func TestBuildKeepsImmutableSuiteJSONWithoutCopy(t *testing.T) {
	suite := []byte(minimalSuiteJSON)
	for _, immutable := range []bool{false, true} {
		snap, err := NewDefaultSnapshotFactory(nil, nil).Build(context.Background(), BuildInput{Region: renderregion.JP, SuiteJSON: suite, SuiteJSONImmutable: immutable})
		if err != nil {
			t.Fatal(err)
		}
		shared := &snap.(*Service).rawJSON[0] == &suite[0]
		if shared != immutable {
			t.Fatalf("immutable=%t shared=%t", immutable, shared)
		}
	}
	merged, err := NewDefaultSnapshotFactory(nil, nil).Build(context.Background(), BuildInput{Region: renderregion.JP, SuiteJSON: suite, MySekaiJSON: []byte(ingestMySekaiJSON)})
	if err != nil {
		t.Fatal(err)
	}
	if raw := merged.(*Service).rawJSON; len(raw) == 0 || &raw[0] == &suite[0] {
		t.Fatal("merged documents must keep their own buffer")
	}
}
