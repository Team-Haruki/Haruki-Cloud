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

	// Cold request: one full mysekai fetch carrying no known upload_time.
	if _, err := provider.Resolve(context.Background(), selector, false); err != nil {
		t.Fatalf("first Resolve() error = %v", err)
	}
	if got := len(client.mysekaiCalls); got != 1 {
		t.Fatalf("cold request: mysekai fetches = %d, want 1", got)
	}
	if got := client.mysekaiKnownTimes; len(got) != 1 || got[0] != 0 {
		t.Fatalf("cold request must not carry a known upload_time, got %v", got)
	}

	// Second, independent request is answered not-modified: no new full fetch.
	if _, err := provider.Resolve(context.Background(), selector, false); err != nil {
		t.Fatalf("second Resolve() error = %v", err)
	}
	if got := len(client.mysekaiCalls); got != 1 {
		t.Fatalf("warm request: mysekai fetches = %d, want 1 (served from cross-request cache)", got)
	}
	if client.mysekaiNotModified != 1 {
		t.Fatalf("warm request: not-modified answers = %d, want 1", client.mysekaiNotModified)
	}
}
