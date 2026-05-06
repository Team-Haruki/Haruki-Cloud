package handler

import (
	"context"
	"errors"
	"testing"

	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/accountdata"
	"haruki-cloud/internal/pjsk/parser"
	renderapp "haruki-cloud/internal/pjsk/render/app"
)

func TestNormalizeDeckUserFacingError(t *testing.T) {
	testCases := []struct {
		name    string
		input   error
		wantErr string
	}{
		{
			name:    "music not found",
			input:   errString("failed to search music by title or alias: music not found: 虾ex"),
			wantErr: "JP服找不到特定的歌: 虾ex\n如果需要查其他服务器歌曲请加区服前缀",
		},
		{
			name:    "snapshot required",
			input:   errString("local user snapshot is not configured"),
			wantErr: ErrMsgSuiteDataNotFound,
		},
		{
			name:    "binding missing",
			input:   accountdata.ErrNoBinding,
			wantErr: ErrMsgBindingNotFound,
		},
		{
			name:    "upstream timeout",
			input:   errString("toolbox: request failed after retries: context deadline exceeded"),
			wantErr: "获取组卡所需数据超时，请稍后重试",
		},
		{
			name:    "future event locked",
			input:   &deckEventLockedError{EventID: 170},
			wantErr: "该活动组卡将于卡池开放后解禁",
		},
		{
			name:    "deck masterdata event missing",
			input:   errString("Event not found for eventId: 167"),
			wantErr: "组卡服务找不到该活动的 masterdata，请更新 masterdata 后重试",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := normalizeDeckUserFacingError(tc.input)
			var replyErr onebot11.ReplayError
			ok := errors.As(err, &replyErr)
			if !ok {
				t.Fatalf("expected ReplayError, got %T (%v)", err, err)
			}
			if string(replyErr) != tc.wantErr {
				t.Fatalf("unexpected replay error: %q", replyErr)
			}
		})
	}
}

func TestNormalizeDeckUserFacingErrorForEventMusicNotFound(t *testing.T) {
	err := normalizeDeckUserFacingErrorForCommand(errString("failed to search music by title or alias: music not found: 虾ex"), "jp", "deck-event")
	assertReplayErrorText(t, err, "当前区服没有该歌曲")
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
	assertReplayErrorText(t, err, ErrMsgBindingNotFound)
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
	assertReplayErrorText(t, err, buildPrivateDataNotFoundMessage("suite", &accountdata.ResolvedBinding{
		Server:     "jp",
		PJSKUserID: "12345678901234",
		Visible:    false,
	}))
}

func assertReplayErrorText(t *testing.T, err error, want string) {
	t.Helper()
	var replyErr onebot11.ReplayError
	if !errors.As(err, &replyErr) {
		t.Fatalf("expected ReplayError, got %T (%v)", err, err)
	}
	if string(replyErr) != want {
		t.Fatalf("unexpected replay error: %q", replyErr)
	}
}
