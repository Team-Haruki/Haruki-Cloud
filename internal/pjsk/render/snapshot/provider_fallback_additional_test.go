package snapshot

import (
	"context"
	"errors"
	"strings"
	"testing"

	"haruki-cloud/internal/pjsk/accountdata"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
)

type fallbackTestSnapshot struct{ err error }

func (s *fallbackTestSnapshot) Require() error { return s.err }
func (*fallbackTestSnapshot) DetailedProfile(renderregion.Value) *drawing.DetailedProfileCardRequest {
	return nil
}
func (*fallbackTestSnapshot) ProfileCard(renderregion.Value) *drawing.ProfileCardRequest { return nil }
func (*fallbackTestSnapshot) MusicResults(string) map[int]string                         { return nil }
func (*fallbackTestSnapshot) GetMusicResult(int, string) string                          { return "" }
func (*fallbackTestSnapshot) ChallengeLive() *ChallengeLiveData                          { return nil }
func (*fallbackTestSnapshot) RawBytes() ([]byte, error)                                  { return nil, nil }
func (*fallbackTestSnapshot) RawValue(string) ([]byte, error)                            { return nil, nil }
func (*fallbackTestSnapshot) RawFilePath() string                                        { return "" }
func (*fallbackTestSnapshot) RawData() *RawUserData                                      { return nil }
func (*fallbackTestSnapshot) MusicMetaBytes() []byte                                     { return nil }
func (*fallbackTestSnapshot) MusicMetaPath() string                                      { return "" }

type fallbackSnapshotProviderFunc func(context.Context, Selector, ResolveOptions) (Snapshot, error)

func (f fallbackSnapshotProviderFunc) Resolve(ctx context.Context, selector Selector, opts ResolveOptions) (Snapshot, error) {
	return f(ctx, selector, opts)
}

func TestFallbackSnapshotProviderPolicy(t *testing.T) {
	ctx := context.Background()
	primaryErr := errors.New("primary failed")
	fallbackSnapshot := &fallbackTestSnapshot{}
	primaryCalls, fallbackCalls := 0, 0
	primary := fallbackSnapshotProviderFunc(func(context.Context, Selector, ResolveOptions) (Snapshot, error) {
		primaryCalls++
		return nil, primaryErr
	})
	fallback := fallbackSnapshotProviderFunc(func(context.Context, Selector, ResolveOptions) (Snapshot, error) {
		fallbackCalls++
		return fallbackSnapshot, nil
	})

	provider := NewFallbackSnapshotProvider(false, nil, primary, fallback)
	if _, err := provider.Resolve(ctx, Selector{}, ResolveOptions{}); !errors.Is(err, primaryErr) {
		t.Fatalf("production primary error = %v", err)
	}
	if primaryCalls != 1 || fallbackCalls != 0 {
		t.Fatalf("production calls = primary:%d fallback:%d", primaryCalls, fallbackCalls)
	}

	provider = NewFallbackSnapshotProvider(true, primary, fallback)
	got, err := provider.Resolve(ctx, Selector{}, ResolveOptions{})
	if err != nil || got != fallbackSnapshot || fallbackCalls != 1 {
		t.Fatalf("fallback result = %#v, %v, calls=%d", got, err, fallbackCalls)
	}

	nilResult := fallbackSnapshotProviderFunc(func(context.Context, Selector, ResolveOptions) (Snapshot, error) { return nil, nil })
	if _, err := NewFallbackSnapshotProvider(true, nilResult).Resolve(ctx, Selector{}, ResolveOptions{}); !errors.Is(err, ErrSnapshotUnavailable) {
		t.Fatalf("nil result error = %v", err)
	}
	if _, err := (*FallbackSnapshotProvider)(nil).Resolve(ctx, Selector{}, ResolveOptions{}); !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("nil provider error = %v", err)
	}
}

func TestStaticSnapshotProvider(t *testing.T) {
	ctx := context.Background()
	if _, err := (*StaticSnapshotProvider)(nil).Resolve(ctx, Selector{}, ResolveOptions{}); !errors.Is(err, ErrSnapshotUnavailable) {
		t.Fatalf("nil static provider error = %v", err)
	}
	if _, err := NewStaticSnapshotProvider(nil).Resolve(ctx, Selector{}, ResolveOptions{}); !errors.Is(err, ErrSnapshotUnavailable) {
		t.Fatalf("empty static provider error = %v", err)
	}
	wantErr := errors.New("invalid snapshot")
	if _, err := NewStaticSnapshotProvider(&fallbackTestSnapshot{err: wantErr}).Resolve(ctx, Selector{}, ResolveOptions{}); !errors.Is(err, wantErr) {
		t.Fatalf("snapshot requirement error = %v", err)
	}
	want := &fallbackTestSnapshot{}
	if got, err := NewStaticSnapshotProvider(want).Resolve(ctx, Selector{}, ResolveOptions{}); err != nil || got != want {
		t.Fatalf("static snapshot = %#v, %v", got, err)
	}
}

type mySekaiPayloadProviderFunc func(context.Context, Selector, bool) ([]byte, error)

func (f mySekaiPayloadProviderFunc) Resolve(ctx context.Context, selector Selector, prefer bool) ([]byte, error) {
	return f(ctx, selector, prefer)
}

func TestFallbackMySekaiPayloadProvider(t *testing.T) {
	ctx := context.Background()
	wantErr := errors.New("first failed")
	first := mySekaiPayloadProviderFunc(func(context.Context, Selector, bool) ([]byte, error) { return nil, wantErr })
	empty := mySekaiPayloadProviderFunc(func(context.Context, Selector, bool) ([]byte, error) { return nil, nil })
	success := mySekaiPayloadProviderFunc(func(context.Context, Selector, bool) ([]byte, error) { return []byte("payload"), nil })

	provider := NewFallbackMySekaiPayloadProvider(nil, first, empty, success)
	payload, err := provider.Resolve(ctx, Selector{}, true)
	if err != nil || string(payload) != "payload" {
		t.Fatalf("fallback payload = %q, %v", payload, err)
	}
	if _, err := NewFallbackMySekaiPayloadProvider(first, empty).Resolve(ctx, Selector{}, false); !errors.Is(err, wantErr) {
		t.Fatalf("last fallback error = %v", err)
	}
	if _, err := NewFallbackMySekaiPayloadProvider(empty).Resolve(ctx, Selector{}, false); !errors.Is(err, ErrSnapshotUnavailable) {
		t.Fatalf("empty fallback error = %v", err)
	}
	if _, err := (*FallbackMySekaiPayloadProvider)(nil).Resolve(ctx, Selector{}, false); !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("nil fallback error = %v", err)
	}
}

func TestToolboxMySekaiPayloadValidation(t *testing.T) {
	ctx := context.Background()
	client := &fakePrivateDataClient{mysekaiJSON: []byte(`{"ok":true}`), uploadTime: "1"}
	bindings := &fakeBindingLookup{bindings: map[string]*accountdata.ResolvedBinding{
		"jp": {PJSKUserID: "123", Server: "jp", MySekaiVisible: true},
	}}
	if (*ToolboxMySekaiPayloadProvider)(nil).WithPrivateDataCache(NewPrivateDataCache()) != nil {
		t.Fatal("nil provider cache attachment is non-nil")
	}
	for _, provider := range []*ToolboxMySekaiPayloadProvider{
		nil,
		NewToolboxMySekaiPayloadProvider(nil, client),
		NewToolboxMySekaiPayloadProvider(bindings, nil),
	} {
		if _, err := provider.Resolve(ctx, Selector{IMPlatform: "qq", IMUserID: "1"}, false); !errors.Is(err, ErrProviderUnavailable) {
			t.Fatalf("unavailable provider error = %v", err)
		}
	}
	provider := NewToolboxMySekaiPayloadProvider(bindings, client)
	if _, err := provider.Resolve(ctx, Selector{}, false); err == nil || !strings.Contains(err.Error(), "selector is incomplete") {
		t.Fatalf("incomplete selector error = %v", err)
	}

	bindings.bindings["jp"] = &accountdata.ResolvedBinding{PJSKUserID: "not-a-number", Server: "jp", MySekaiVisible: true}
	if _, err := provider.Resolve(ctx, Selector{IMPlatform: "qq", IMUserID: "1", Region: renderregion.JP}, false); err == nil || !strings.Contains(err.Error(), "invalid bound") {
		t.Fatalf("invalid UID error = %v", err)
	}
	bindings.bindings["jp"] = &accountdata.ResolvedBinding{PJSKUserID: "123", Server: "jp", MySekaiVisible: true}
	client.mysekaiJSON = nil
	if _, err := provider.Resolve(ctx, Selector{IMPlatform: "qq", IMUserID: "1", Region: renderregion.JP}, false); !errors.Is(err, ErrSnapshotUnavailable) {
		t.Fatalf("empty payload error = %v", err)
	}
}

func TestResolveExplicitMySekaiPayloadBinding(t *testing.T) {
	ctx := context.Background()
	bindings := &fakeBindingLookup{listItems: []accountdata.BindingListItem{
		{BindingID: 1, UserID: "123", Server: "jp", MySekaiVisible: true},
		{BindingID: 2, UserID: "123", Server: "tw", MySekaiVisible: true},
		{BindingID: 3, UserID: "456", Server: "jp", MySekaiVisible: false},
	}}
	if _, err := resolveMySekaiPayloadBinding(ctx, nil, "qq", "1", renderregion.JP, "", false); !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("nil binding lookup error = %v", err)
	}
	if _, err := resolveExplicitMySekaiPayloadBinding(ctx, bindings, "qq", "1", renderregion.Unknown, "123"); err == nil || !strings.Contains(err.Error(), "multiple bindings") {
		t.Fatalf("ambiguous explicit binding error = %v", err)
	}
	resolved, err := resolveExplicitMySekaiPayloadBinding(ctx, bindings, "qq", "1", renderregion.JP, "123")
	if err != nil || resolved.BindingID != 1 {
		t.Fatalf("explicit binding = %#v, %v", resolved, err)
	}
	if _, err := resolveExplicitMySekaiPayloadBinding(ctx, bindings, "qq", "1", renderregion.JP, "missing"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing explicit binding error = %v", err)
	}
	if _, err := resolveExplicitMySekaiPayloadBinding(ctx, bindings, "qq", "1", renderregion.JP, "456"); err == nil || !strings.Contains(err.Error(), "does not expose") {
		t.Fatalf("hidden explicit binding error = %v", err)
	}
	if mySekaiPayloadBindingAllowed(nil) || mySekaiPayloadBindingAllowed(&accountdata.ResolvedBinding{}) || !mySekaiPayloadBindingAllowed(resolved) {
		t.Fatal("mysekai binding visibility classification failed")
	}
}
