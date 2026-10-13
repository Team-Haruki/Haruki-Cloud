package snapshot

import (
	"context"
	"slices"
	"testing"
)

const (
	cloudProfileSuiteJSON = `{"upload_time":1710000000,"userGamedata":{"userId":123456789},"userProfile":{"word":"cloud"}}`
	wholeSuiteJSON        = `{"upload_time":1710000000,"userGamedata":{"userId":123456789},"userProfile":{"word":"whole"}}`
)

// setSuiteProfile switches the fake to answer whole-Suite reads in profile,
// with the body that profile renders.
func setSuiteProfile(client *fakePrivateDataClient, profile string) {
	client.suiteProfile = profile
	client.suiteJSON = []byte(wholeSuiteJSON)
	if profile != "" {
		client.suiteJSON = []byte(cloudProfileSuiteJSON)
	}
}

func resolveSuiteWord(t *testing.T, provider *ToolboxSnapshotProvider, ctx context.Context) string {
	t.Helper()
	snap, err := provider.Resolve(ctx, validToolboxSelector(), ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	return snap.RawData().UserProfile.Word
}

// Bodies of different profiles carry the same upload_time, so a profile must
// be part of every cache key: otherwise a 304 for one shape would confirm a
// cached body of the other.
func TestSuiteProfileSeparatesRawBuiltAndRequestCaches(t *testing.T) {
	client := &fakePrivateDataClient{uploadTime: "1710000000"}
	setSuiteProfile(client, "cloud")
	provider := newRevalidationProvider(client)

	if got := resolveSuiteWord(t, provider, WithRequestCache(t.Context())); got != "cloud" {
		t.Fatalf("cold cloud read served %q", got)
	}
	if got := resolveSuiteWord(t, provider, WithRequestCache(t.Context())); got != "cloud" {
		t.Fatalf("warm cloud read served %q", got)
	}
	if !slices.Equal(client.suiteKnownTimes, []int64{0, 1710000000}) {
		t.Fatalf("cloud known times = %v, want a cold read then a conditional one", client.suiteKnownTimes)
	}

	// Raw and built entries exist only for the cloud profile: the whole
	// document is fetched cold, never revalidated against the cloud body.
	setSuiteProfile(client, "")
	if got := resolveSuiteWord(t, provider, WithRequestCache(t.Context())); got != "whole" {
		t.Fatalf("whole-document read served %q from another profile", got)
	}
	if !slices.Equal(client.suiteKnownTimes, []int64{0, 1710000000, 0}) {
		t.Fatalf("known times = %v, want a cold whole-document read", client.suiteKnownTimes)
	}

	// With the raw cache gone, each profile revalidates only its own built
	// snapshot.
	provider.WithPrivateDataCache(NewPrivateDataCache())
	for _, profile := range []string{"cloud", ""} {
		setSuiteProfile(client, profile)
		want := "whole"
		if profile != "" {
			want = "cloud"
		}
		if got := resolveSuiteWord(t, provider, WithRequestCache(t.Context())); got != want {
			t.Fatalf("profile %q after raw eviction served %q, want %q", profile, got, want)
		}
	}
	if client.suiteNotModified != 3 || len(client.suiteCalls) != 2 {
		t.Fatalf("not-modified=%d full=%d, want built revalidation for both profiles", client.suiteNotModified, len(client.suiteCalls))
	}

	// One command's request cache keeps the shapes apart too.
	ctx := WithRequestCache(t.Context())
	provider = newRevalidationProvider(client)
	setSuiteProfile(client, "cloud")
	first := resolveSuiteWord(t, provider, ctx)
	setSuiteProfile(client, "")
	second := resolveSuiteWord(t, provider, ctx)
	if first != "cloud" || second != "whole" {
		t.Fatalf("one request cache served %q then %q", first, second)
	}
}

func TestSuiteProfileAppliesOnlyToWholeSuiteReads(t *testing.T) {
	client := &fakePrivateDataClient{
		mysekaiJSON: []byte(revalidationMySekaiJSON),
		uploadTime:  "1710000000",
	}
	setSuiteProfile(client, "cloud")
	provider := newRevalidationProvider(client)

	if _, err := provider.Resolve(WithRequestCache(t.Context()), validToolboxSelector(), ResolveOptions{SuiteFields: []string{"userProfile"}, NeedMySekai: true}); err != nil {
		t.Fatalf("projected Resolve() error = %v", err)
	}
	if len(client.suiteProfiles) != 0 {
		t.Fatalf("a projected read went through the profiled whole-Suite read: %v", client.suiteProfiles)
	}
	if _, err := provider.Resolve(WithRequestCache(t.Context()), validToolboxSelector(), ResolveOptions{}); err != nil {
		t.Fatalf("whole Resolve() error = %v", err)
	}
	if !slices.Equal(client.suiteProfiles, []string{"cloud"}) {
		t.Fatalf("whole-Suite read profiles = %v", client.suiteProfiles)
	}

	var keys []PrivateDataKey
	provider.privateCache.mu.Lock()
	for key := range provider.privateCache.items {
		keys = append(keys, key)
	}
	provider.privateCache.mu.Unlock()
	for _, key := range keys {
		wantProfile := ""
		if key.DataType == "suite" && key.Projection == "" {
			wantProfile = "cloud"
		}
		if key.Profile != wantProfile {
			t.Fatalf("raw cache key %+v, want profile %q", key, wantProfile)
		}
	}
	if len(keys) != 3 {
		t.Fatalf("raw cache keys = %+v, want projected Suite, profiled Suite and MySekai", keys)
	}

	provider.builtCache.mu.Lock()
	defer provider.builtCache.mu.Unlock()
	for key := range provider.builtCache.items {
		if wantProfile := map[bool]string{true: "", false: "cloud"}[key.SuiteProjection != ""]; key.SuiteProfile != wantProfile {
			t.Fatalf("built cache key %+v, want profile %q", key, wantProfile)
		}
	}
}
