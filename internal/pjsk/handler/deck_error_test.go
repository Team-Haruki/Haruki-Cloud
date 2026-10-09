package handler

import (
	"context"
	"haruki-cloud/internal/i18n"
	"testing"

	"haruki-cloud/internal/core/upstreamerr"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/accountdata"
	"haruki-cloud/internal/pjsk/notfound"
	"haruki-cloud/internal/pjsk/parser"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/deck"
	rendersnapshot "haruki-cloud/internal/pjsk/render/snapshot"
	"haruki-cloud/internal/testutil"
	"haruki-cloud/utils/usererror"
)

func TestNormalizeDeckUserFacingError(t *testing.T) {
	testCases := []struct {
		name  string
		input error
		code  usererror.Code
		id    string
	}{
		{"music not found", notfound.Music("虾ex"), usererror.CodeNotFound, "music.not_found_in_region"},
		{"snapshot required", rendersnapshot.ErrNotConfigured, usererror.CodeSetup, "binding.data.not_found"},
		{"deck service needs suite data", deck.ErrUserDataRequired, usererror.CodeSetup, "binding.data.not_found"},
		{"binding missing", accountdata.ErrNoBinding, usererror.CodeSetup, "binding.required"},
		{"upstream timeout", upstreamerr.Transport(upstreamerr.ServiceToolbox, "toolbox: request failed after retries", context.DeadlineExceeded), usererror.CodeTimeout, "common.timeout"},
		{"future event locked", &deckEventLockedError{EventID: 170}, usererror.CodeForbidden, "deck.event.locked"},
		{"deck masterdata event missing", deckRemoteError(404, "Event not found for eventId: 167"), usererror.CodeUnavailable, "upstream.deck.data_not_synced"},
		{"unclassified deck failure", deckRemoteError(500, "boom"), usererror.CodeUnavailable, "upstream.failed"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			testutil.RequireUserError(t, normalizeDeckUserFacingError(tc.input), tc.code, tc.id)
		})
	}
}

func deckRemoteError(status int, message string) error {
	return &deck.RemoteError{StatusCode: status, Message: message}
}

func TestExecuteDeckReturnsDisabledMessage(t *testing.T) {
	msg, err := executeDeck(&RequestContext{
		Ctx: context.Background(),
		Cmd: &CommandRequest{
			Module: parser.ModuleDeck,
			Mode:   "deck-event",
			Region: "jp",
		},
		App: &renderapp.App{
			Config: renderapp.Config{
				DeckRecommend: renderapp.DeckRecommendConfig{
					Disable:       true,
					DisableReason: "maintenance",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("executeDeck returned error: %v", err)
	}
	if len(msg) != 1 || msg[0].Type != onebot11.TypeText {
		t.Fatalf("unexpected message: %+v", msg)
	}
	data, ok := msg[0].Data.(onebot11.TextData)
	if !ok {
		t.Fatalf("unexpected text data: %+v", msg[0].Data)
	}
	want := i18n.T("deck.disabled_reason", i18n.Data{"Reason": "maintenance"})
	if data.Text != want {
		t.Fatalf("unexpected disabled message:\n%s", data.Text)
	}
}

func TestNormalizeDeckUserFacingErrorForEventMusicNotFound(t *testing.T) {
	err := normalizeDeckUserFacingErrorForCommand(notfound.Music("虾ex"), "jp", "deck-event")
	testutil.RequireUserError(t, err, usererror.CodeNotFound, "music.not_found_in_region")
}

func TestNormalizeDeckUserFacingErrorForFutureEventMasterdataMissing(t *testing.T) {
	err := normalizeDeckUserFacingErrorForCommand(deckRemoteError(404, "Event not found for eventId: 204"), "jp", "deck-event")
	testutil.RequireUserError(t, err, usererror.CodeUnavailable, "deck.event.data_not_synced")
}

type errString string

func (e errString) Error() string {
	return string(e)
}

func TestExecuteDeckReturnsStandardBindingReplayError(t *testing.T) {
	_, err := executeDeck(NewRequestContext(context.Background(), &CommandRequest{
		Module:            parser.ModuleDeck,
		Mode:              "deck-event",
		Region:            "jp",
		RequesterPlatform: "qq",
		RequesterUserID:   "42",
	}, &renderapp.App{
		Bindings: newHandlerTestBindingService(t),
	}))
	testutil.RequireUserError(t, err, usererror.CodeSetup, "binding.required")
}

func TestExecuteDeckReturnsStandardSuiteReplayError(t *testing.T) {
	ctx := context.Background()
	service := newHandlerTestBindingService(t)
	if _, err := service.Bind(ctx, "qq", "42", "12345678901234"); err != nil {
		t.Fatalf("bind: %v", err)
	}

	_, err := executeDeck(NewRequestContext(ctx, &CommandRequest{
		Module:            parser.ModuleDeck,
		Mode:              "deck-event",
		Region:            "jp",
		RequesterPlatform: "qq",
		RequesterUserID:   "42",
	}, &renderapp.App{
		Bindings: service,
	}))
	typed := testutil.RequireUserError(t, err, usererror.CodeSetup, "binding.data.not_found_account")
	if typed.Message.String() != privateDataNotFoundMessage("suite", &accountdata.ResolvedBinding{
		Server:     "jp",
		PJSKUserID: "12345678901234",
		Visibility: accountdata.UniformVisibility(false),
	}).String() {
		t.Fatalf("unexpected suite reply: %s", typed.Message)
	}
}
