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
