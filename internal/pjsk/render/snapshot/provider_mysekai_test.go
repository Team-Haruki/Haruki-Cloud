package snapshot

import (
	"context"
	"testing"

	"haruki-cloud/internal/pjsk/accountdata"
	renderregion "haruki-cloud/internal/pjsk/region"
)

func TestToolboxMySekaiPayloadProviderAllowsMySekaiOnlyBinding(t *testing.T) {
	client := &fakePrivateDataClient{
		mysekaiJSON: []byte(`{"updatedResources":{"userMysekaiPhotos":[]}}`),
		uploadTime:  "1710000000",
	}
	provider := NewToolboxMySekaiPayloadProvider(
		&fakeBindingLookup{
			bindings: map[string]*accountdata.ResolvedBinding{
				"jp": {
					PJSKUserID:     "123456789",
					Server:         "jp",
					SuiteVisible:   false,
					MySekaiVisible: true,
				},
			},
		},
		client,
	)

	type contextKey struct{}
	ctx := context.WithValue(context.Background(), contextKey{}, "request")
	payload, err := provider.Resolve(ctx, Selector{
		IMPlatform: "qq",
		IMUserID:   "10001",
		Region:     renderregion.JP,
	}, false)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if string(payload) != `{"updatedResources":{"userMysekaiPhotos":[]}}` {
		t.Fatalf("unexpected payload: %s", string(payload))
	}
	if client.mysekaiCtx != ctx {
		t.Fatal("mysekai payload request did not receive the caller context")
	}
}

func TestToolboxMySekaiPayloadProviderSharesPayloadAcrossRequests(t *testing.T) {
	client := &fakePrivateDataClient{
		mysekaiJSON: []byte(`{"upload_time":1710000000,"updatedResources":{}}`),
		uploadTime:  "1710000000",
	}
	provider := NewToolboxMySekaiPayloadProvider(
		&fakeBindingLookup{
			bindings: map[string]*accountdata.ResolvedBinding{
				"jp": {PJSKUserID: "123456789", Server: "jp", MySekaiVisible: true},
			},
		},
		client,
	).WithPrivateDataCache(NewPrivateDataCache())

	selector := Selector{IMPlatform: "qq", IMUserID: "10001", Region: renderregion.JP}

	// Cold request: one full mysekai fetch, no upload_time probe.
	if _, err := provider.Resolve(context.Background(), selector, false); err != nil {
		t.Fatalf("first Resolve() error = %v", err)
	}
	if got := len(client.mysekaiCalls); got != 1 {
		t.Fatalf("cold request: mysekai fetches = %d, want 1", got)
	}
	if client.mysekaiUploadTimeCalls != 0 {
		t.Fatalf("cold request must not probe upload_time, got %d", client.mysekaiUploadTimeCalls)
	}

	// Second, independent request: probe hits the shared cache, no new full fetch.
	if _, err := provider.Resolve(context.Background(), selector, false); err != nil {
		t.Fatalf("second Resolve() error = %v", err)
	}
	if got := len(client.mysekaiCalls); got != 1 {
		t.Fatalf("warm request: mysekai fetches = %d, want 1 (served from cross-request cache)", got)
	}
	if client.mysekaiUploadTimeCalls != 1 {
		t.Fatalf("warm request: upload_time probes = %d, want 1", client.mysekaiUploadTimeCalls)
	}
}
