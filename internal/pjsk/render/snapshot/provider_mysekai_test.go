package snapshot

import (
	"context"
	"testing"

	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/accountdata"
)

func TestToolboxMySekaiPayloadProviderAllowsMySekaiOnlyBinding(t *testing.T) {
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
		&fakePrivateDataClient{
			mysekaiJSON: []byte(`{"updatedResources":{"userMysekaiPhotos":[]}}`),
			uploadTime:  "1710000000",
		},
	)

	payload, err := provider.Resolve(context.Background(), Selector{
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
}
