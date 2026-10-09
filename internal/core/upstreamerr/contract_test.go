package upstreamerr

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"syscall"
	"testing"

	"haruki-cloud/internal/i18n"
	"haruki-cloud/utils/usererror"
)

// described is a minimal Described error, as the clients build them.
type described struct {
	service Service
	status  int
	message string
}

func (d *described) Error() string            { return fmt.Sprintf("%s %d %s", d.service, d.status, d.message) }
func (d *described) UpstreamService() Service { return d.service }
func (d *described) UpstreamStatus() int      { return d.status }
func (d *described) UpstreamMessage() string  { return d.message }

// TestUpstreamContract lists every upstream message Cloud depends on, as the
// upstream services write it, and the kind it must classify as. Changing a
// message in the Toolbox, SekaiAPI, the tracker, deck-service or Drawing
// without updating contract.go fails here.
func TestUpstreamContract(t *testing.T) {
	cases := []struct {
		service Service
		status  int
		message string
		want    Kind
	}{
		// Toolbox ("message" of the JSON error body).
		{ServiceToolbox, http.StatusForbidden, "forbidden: invalid platform or platform_user_id for this user", KindAccessDenied},
		{ServiceToolbox, http.StatusForbidden, "forbidden: account owner is banned", KindOwnerBanned},
		{ServiceToolbox, http.StatusNotFound, "account binding not found", KindAccountNotBound},
		{ServiceToolbox, http.StatusNotFound, "game data not found", KindDataNotUploaded},
		{ServiceToolbox, http.StatusServiceUnavailable, "toolbox service unavailable", KindUnavailable},
		{ServiceToolbox, http.StatusUnauthorized, "missing token", KindAuth},
		{ServiceToolbox, http.StatusUnauthorized, "invalid token", KindAuth},
		// SekaiAPI ("message" of the JSON error body).
		{ServiceGameData, http.StatusUnauthorized, "missing token", KindAuth},
		{ServiceGameData, http.StatusUnauthorized, "invalid token", KindAuth},
		{ServiceGameData, http.StatusForbidden, "token is not authorized for this server", KindAuth},
		{ServiceGameData, http.StatusBadRequest, "invalid api type", KindRejected},
		{ServiceGameData, http.StatusInternalServerError, "internal server error", KindBadResponse},
		{ServiceGameData, http.StatusBadGateway, "upstream unavailable", KindUnavailable},
		// Tracker ("message" of the JSON error body).
		{ServiceRanking, http.StatusTooManyRequests, "rate limited by tracker", KindRateLimited},
		{ServiceRanking, http.StatusNotFound, "no heartbeat found for event", KindNoRankingData},
		{ServiceRanking, http.StatusBadRequest, "invalid server", KindRegionUnsupported},
		{ServiceRanking, http.StatusNotFound, "ranking record not found", KindRankingNotFound},
		{ServiceRanking, http.StatusBadRequest, "event not found", KindNotFound},
		// deck-service ("error" of the JSON error body, or a batch item error).
		{ServiceDeck, http.StatusBadRequest, "fixed_characters and fixed_cards cannot be used together", KindIncompatible},
		{ServiceDeck, http.StatusNotFound, "Event not found for eventId: 999", KindDataNotSynced},
		{ServiceDeck, http.StatusNotFound, "master data not found for region jp", KindDataNotSynced},
		{ServiceDeck, http.StatusNotFound, "music metas not found", KindDataNotSynced},
		{ServiceDeck, http.StatusNotFound, "music meta not found for music 1", KindDataNotSynced},
		{ServiceDeck, http.StatusBadRequest, "userdata_hash is required", KindCacheExpired},
		{ServiceDeck, http.StatusNotFound, "user data not found for userdata_hash abc", KindCacheExpired},
		{ServiceDeck, http.StatusUnsupportedMediaType, "Unsupported Media Type", KindIncompatible},
		{ServiceDeck, http.StatusBadRequest, "unsupported content type application/x", KindIncompatible},
		{ServiceDeck, http.StatusBadRequest, "invalid content type", KindIncompatible},
		{ServiceDeck, http.StatusBadRequest, "invalid recommend payload: missing region", KindRejected},
		{ServiceDeck, http.StatusBadRequest, "recommend requires batch_options", KindRejected},
		{ServiceDeck, 0, "no user data bytes available", KindUserDataInvalid},
		{ServiceDeck, http.StatusBadRequest, "user data is required", KindUserDataInvalid},
		// Drawing ("detail" of a 4xx body; any part of the body for thin data).
		{ServiceRender, http.StatusBadRequest, "Data insufficient for trace", KindDataInsufficient},
		{ServiceRender, http.StatusBadRequest, "insufficient data", KindDataInsufficient},
		{ServiceRender, http.StatusBadRequest, "not enough data points", KindDataInsufficient},
		{ServiceRender, http.StatusBadRequest, "轨迹数据不足", KindDataInsufficient},
		{ServiceRender, http.StatusBadRequest, "single positional indexer is out-of-bounds", KindDataInsufficient},
		{ServiceRender, http.StatusBadRequest, "index 3 is out-of-bounds", KindDataInsufficient},
		{ServiceRender, http.StatusBadRequest, "list index out of bounds", KindDataInsufficient},
		{ServiceRender, http.StatusBadRequest, "list index out of range", KindDataInsufficient},
		{ServiceRender, http.StatusBadRequest, "content size is too large", KindContentTooLarge},
		{ServiceRender, http.StatusBadRequest, "canvas size is too large", KindContentTooLarge},
		{ServiceRender, http.StatusNotFound, "target file not found: a.png", KindAssetMissing},
		{ServiceRender, http.StatusBadRequest, "file not found", KindAssetMissing},
		{ServiceRender, http.StatusBadRequest, "[Errno 2] No such file or directory", KindAssetMissing},
		{ServiceRender, http.StatusBadRequest, "asset path is empty", KindAssetMissing},
		{ServiceRender, http.StatusBadRequest, "图片文件不存在", KindAssetMissing},
		{ServiceRender, http.StatusBadRequest, "cannot identify image file", KindAssetBroken},
		{ServiceRender, http.StatusBadRequest, "failed to read image", KindAssetBroken},
		{ServiceRender, http.StatusBadRequest, "failed to open image", KindAssetBroken},
		{ServiceRender, http.StatusBadRequest, "download of asset x failed", KindAssetDownload},
	}
	for _, tc := range cases {
		class, ok := Classify(&described{service: tc.service, status: tc.status, message: tc.message})
		if !ok || class.Kind != tc.want || class.Service != tc.service || class.Status != tc.status {
			t.Errorf("%s %d %q: Classify = %+v (ok %v), want kind %s", tc.service, tc.status, tc.message, class, ok, tc.want)
		}
	}
}

// TestUpstreamStatusContract pins the status codes Cloud interprets when the
// upstream message is empty or unknown.
func TestUpstreamStatusContract(t *testing.T) {
	cases := []struct {
		service Service
		status  int
		want    Kind
	}{
		{ServiceToolbox, http.StatusUnauthorized, KindAuth},
		{ServiceToolbox, http.StatusForbidden, KindAccessDenied},
		{ServiceToolbox, http.StatusNotFound, KindDataNotUploaded},
		{ServiceToolbox, http.StatusServiceUnavailable, KindUnavailable},
		{ServiceGameData, http.StatusForbidden, KindAuth},
		{ServiceGameData, http.StatusNotFound, KindPlayerNotFound},
		{ServiceGameData, http.StatusServiceUnavailable, KindMaintenance},
		{ServiceRanking, http.StatusNotFound, KindRankingNotFound},
		{ServiceRanking, http.StatusTooManyRequests, KindRateLimited},
		{ServiceRanking, http.StatusServiceUnavailable, KindMaintenance},
		{ServiceRender, http.StatusNotFound, KindIncompatible},
		{ServiceRender, http.StatusInternalServerError, KindBadResponse},
		{ServiceRender, http.StatusBadGateway, KindUnavailable},
		{ServiceDeck, http.StatusNotFound, KindRejected},
		{ServiceDeck, http.StatusUnsupportedMediaType, KindIncompatible},
		{ServiceDeck, http.StatusGatewayTimeout, KindUnavailable},
		{ServiceDeck, http.StatusBadRequest, KindRejected},
	}
	for _, tc := range cases {
		class, ok := Classify(&described{service: tc.service, status: tc.status})
		if !ok || class.Kind != tc.want {
			t.Errorf("%s %d: Classify = %+v, want %s", tc.service, tc.status, class, tc.want)
		}
	}
}

type timeoutError struct{}

func (timeoutError) Error() string   { return "i/o timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

func TestTransportKindUsesErrorTypes(t *testing.T) {
	refused := &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}
	cases := []struct {
		err  error
		want Kind
	}{
		{context.DeadlineExceeded, KindTimeout},
		{fmt.Errorf("post: %w", timeoutError{}), KindTimeout},
		{refused, KindUnavailable},
		{&net.DNSError{Err: "no such host", Name: "x"}, KindUnavailable},
		{errors.New("connection reset"), KindUnavailable},
	}
	for _, tc := range cases {
		err := Transport(ServiceRender, "drawing request failed", tc.err)
		class, ok := Classify(err)
		if !ok || class.Kind != tc.want || class.Service != ServiceRender {
			t.Errorf("Transport(%v): Classify = %+v, want %s", tc.err, class, tc.want)
		}
		if !errors.Is(err, tc.err) {
			t.Errorf("Transport(%v) must unwrap to the original error", tc.err)
		}
	}
	if Transport(ServiceRender, "x", nil) != nil {
		t.Fatal("Transport(nil) must be nil")
	}
	if !IsNetworkFailure(refused) || !IsNetworkFailure(context.DeadlineExceeded) || IsNetworkFailure(errors.New("plain")) {
		t.Fatal("IsNetworkFailure must judge by error type")
	}
}

func TestClassifyPrefersKindedErrorsAndIgnoresOthers(t *testing.T) {
	sentinel := NewSentinel(ServiceRender, KindDataInsufficient, 0, "drawing response data is insufficient")
	wrapped := fmt.Errorf("render: %w", sentinel)
	if class, ok := Classify(wrapped); !ok || class.Kind != KindDataInsufficient || !Is(wrapped, KindDataInsufficient) || !IsService(wrapped, ServiceRender) {
		t.Fatalf("Classify(wrapped sentinel) = %+v, %v", class, ok)
	}
	tagged := Tag(ServiceDeck, KindEmptyResult, "", errors.New("deck-service returned empty response"))
	if tagged.Error() != "deck-service returned empty response" {
		t.Fatalf("Tag text = %q", tagged.Error())
	}
	if _, ok := Classify(errors.New("plain")); ok {
		t.Fatal("a plain error is not an upstream failure")
	}
	if _, ok := Classify(nil); ok {
		t.Fatal("nil is not an upstream failure")
	}
}

// TestUserErrorMapping pins the code and catalog message of every kind, so a
// user-facing reply never depends on upstream wording.
func TestUserErrorMapping(t *testing.T) {
	cases := []struct {
		service Service
		kind    Kind
		code    usererror.Code
		id      string
	}{
		{ServiceDeck, KindNotConfigured, usererror.CodeMisconfigured, "common.misconfigured"},
		{ServiceGameData, KindAuth, usererror.CodeMisconfigured, "common.misconfigured"},
		{ServiceRender, KindIncompatible, usererror.CodeMisconfigured, "common.misconfigured"},
		{ServiceRanking, KindUnavailable, usererror.CodeUnavailable, "common.unavailable"},
		{ServiceToolbox, KindTimeout, usererror.CodeTimeout, "common.timeout"},
		{ServiceRanking, KindRateLimited, usererror.CodeUnavailable, "upstream.rate_limited"},
		{ServiceGameData, KindMaintenance, usererror.CodeUnavailable, "upstream.maintenance"},
		{ServiceRender, KindBadResponse, usererror.CodeUnavailable, "upstream.failed"},
		{ServiceDeck, KindRejected, usererror.CodeUnavailable, "upstream.failed"},
		{ServiceToolbox, KindAccountNotBound, usererror.CodeSetup, "upstream.toolbox.account_not_bound"},
		{ServiceToolbox, KindDataNotUploaded, usererror.CodeSetup, "upstream.toolbox.data_not_uploaded"},
		{ServiceToolbox, KindAccessDenied, usererror.CodeSetup, "upstream.toolbox.access_denied"},
		{ServiceToolbox, KindOwnerBanned, usererror.CodeForbidden, "upstream.toolbox.owner_banned"},
		{ServiceGameData, KindPlayerNotFound, usererror.CodeNotFound, "upstream.game_data.player_not_found"},
		{ServiceRanking, KindRankingNotFound, usererror.CodeNotFound, "upstream.ranking.not_found"},
		{ServiceRanking, KindNotFound, usererror.CodeNotFound, "upstream.ranking.no_match"},
		{ServiceRender, KindNotFound, usererror.CodeUnavailable, "upstream.failed"},
		{ServiceRanking, KindNoRankingData, usererror.CodeUnavailable, "upstream.ranking.no_data"},
		{ServiceRanking, KindRegionUnsupported, usererror.CodeUnavailable, "upstream.ranking.region_unsupported"},
		{ServiceRender, KindDataInsufficient, usererror.CodeNotFound, "upstream.render.data_insufficient"},
		{ServiceRender, KindContentTooLarge, usererror.CodeInput, "upstream.render.too_large"},
		{ServiceRender, KindAssetMissing, usererror.CodeUnavailable, "upstream.render.asset_missing"},
		{ServiceRender, KindAssetBroken, usererror.CodeUnavailable, "upstream.render.asset_broken"},
		{ServiceRender, KindAssetDownload, usererror.CodeUnavailable, "upstream.render.asset_download"},
		{ServiceDeck, KindDataNotSynced, usererror.CodeUnavailable, "upstream.deck.data_not_synced"},
		{ServiceDeck, KindCacheExpired, usererror.CodeUnavailable, "upstream.deck.cache_expired"},
		{ServiceDeck, KindUserDataInvalid, usererror.CodeSetup, "upstream.deck.user_data_invalid"},
		{ServiceDeck, KindEmptyResult, usererror.CodeUnavailable, "upstream.deck.empty_result"},
		{ServiceDeck, KindUnknown, usererror.CodeUnavailable, "common.unavailable"},
	}
	for _, tc := range cases {
		cause := Tag(tc.service, tc.kind, "cause", nil)
		typed := UserError(cause)
		if typed == nil || typed.Code != tc.code || typed.Message.ID != tc.id {
			t.Errorf("%s/%s: UserError = %+v, want %s %s", tc.service, tc.kind, typed, tc.code, tc.id)
			continue
		}
		if !i18n.Has(typed.Message.ID) || typed.Error() == "" {
			t.Errorf("%s/%s: message %s is not in the catalog", tc.service, tc.kind, typed.Message.ID)
		}
		if typed.Cause != cause {
			t.Errorf("%s/%s: the upstream error must stay the logged cause", tc.service, tc.kind)
		}
	}
	if UserError(errors.New("plain")) != nil {
		t.Fatal("a plain error has no upstream user error")
	}
	own := usererror.ReadOnly()
	if UserError(fmt.Errorf("wrap: %w", own)) != own {
		t.Fatal("a typed user error passes through")
	}
}

func TestFeatureNames(t *testing.T) {
	for service, want := range map[Service]i18n.Message{
		ServiceToolbox:  i18n.FeatureToolbox,
		ServiceGameData: i18n.FeatureGameData,
		ServiceRanking:  i18n.FeatureRanking,
		ServiceDeck:     i18n.FeatureDeck,
		ServiceRender:   i18n.FeatureRender,
		"":              i18n.FeatureGameData,
	} {
		if Feature(service).ID != want.ID {
			t.Errorf("Feature(%q) = %s, want %s", service, Feature(service).ID, want.ID)
		}
	}
}
