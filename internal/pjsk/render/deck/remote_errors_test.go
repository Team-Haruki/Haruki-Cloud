package deck

import (
	"errors"
	"testing"

	"haruki-cloud/internal/core/upstreamerr"
)

// TestDeckServiceErrorsClassifyByContract checks the deck-service answers the
// replies depend on (see upstreamerr/contract.go) and that the error text
// the retry logic reads is unchanged.
func TestDeckServiceErrorsClassifyByContract(t *testing.T) {
	cases := []struct {
		status int
		body   string
		text   string
		kind   upstreamerr.Kind
	}{
		{400, `{"error":"fixed_characters and fixed_cards cannot be used together"}`, "fixed_characters and fixed_cards cannot be used together", upstreamerr.KindIncompatible},
		{404, `{"error":"master data not found for region jp"}`, "master data not found for region jp", upstreamerr.KindDataNotSynced},
		{404, `{"error":"user data not found for userdata_hash abc"}`, "user data not found for userdata_hash abc", upstreamerr.KindCacheExpired},
		{415, `Unsupported Media Type`, "deck-service returned HTTP 415: Unsupported Media Type", upstreamerr.KindIncompatible},
		{500, ``, "deck-service returned HTTP 500", upstreamerr.KindBadResponse},
		{503, `busy`, "deck-service returned HTTP 503: busy", upstreamerr.KindUnavailable},
	}
	for _, tc := range cases {
		err := parseRemoteHTTPError(tc.status, []byte(tc.body))
		if err.Error() != tc.text {
			t.Errorf("parseRemoteHTTPError(%d, %s) text = %q, want %q", tc.status, tc.body, err.Error(), tc.text)
		}
		class, ok := upstreamerr.Classify(err)
		if !ok || class.Service != upstreamerr.ServiceDeck || class.Kind != tc.kind {
			t.Errorf("parseRemoteHTTPError(%d, %s): Classify = %+v (ok %v), want %s", tc.status, tc.body, class, ok, tc.kind)
		}
	}
	item := &RemoteError{Message: "music metas not found"}
	if !upstreamerr.Is(item, upstreamerr.KindDataNotSynced) || item.Error() != "music metas not found" {
		t.Fatalf("batch item error = %v", item)
	}
	if !upstreamerr.Is(errDeckNotConfigured, upstreamerr.KindNotConfigured) || errors.Is(ErrUserDataRequired, errDeckNotConfigured) {
		t.Fatal("deck sentinels misclassified")
	}
}
