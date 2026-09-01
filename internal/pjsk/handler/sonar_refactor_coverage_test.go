package handler

import (
	"context"
	"errors"
	"strings"
	"testing"

	"haruki-cloud/internal/pjsk/accountdata"
	aliases "haruki-cloud/internal/pjsk/alias"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/deck"
	rendermusic "haruki-cloud/internal/pjsk/render/music"
)

func TestDeckRecommendTypeCoversEveryMode(t *testing.T) {
	tests := map[string]string{
		deckEventCommand:     "event",
		deckChallengeCommand: "challenge",
		deckNoEventCommand:   "no_event",
		deckBonusCommand:     "bonus",
	}
	for mode, want := range tests {
		got, ok := deckRecommendType(mode)
		if !ok || got != want {
			t.Fatalf("deckRecommendType(%q) = %q, %v", mode, got, ok)
		}
	}
	if got, ok := deckRecommendType("unknown"); ok || got != "" {
		t.Fatalf("unknown mode = %q, %v", got, ok)
	}
	if text := buildDeckDoneText(deck.AutoQuery{RecommendType: "event"}); !strings.Contains(text, "Haruki工具箱") {
		t.Fatalf("event completion text = %q", text)
	}
}

func TestAliasImageRefactorInputGuards(t *testing.T) {
	app := &renderapp.App{
		Aliases: &aliases.Service{},
		Music:   &rendermusic.Controller{},
	}
	for _, rc := range []*RequestContext{
		{App: app},
		{App: app, Cmd: &CommandRequest{Mode: "not-query"}},
		{App: app, Cmd: &CommandRequest{Mode: aliases.ModeQuery, Params: []byte("{")}},
		{App: app, Cmd: &CommandRequest{Mode: aliases.ModeQuery, Params: []byte(`{"target":" "}`)}},
	} {
		if _, ok, _ := tryRenderAliasQueryAsImage(rc); ok {
			t.Fatal("guarded alias query unexpectedly rendered")
		}
	}
}

func TestBindingResolutionStateRecordsAllOutcomes(t *testing.T) {
	usable := &accountdata.ResolvedBinding{SuiteVisible: true, MySekaiVisible: true}
	state := bindingResolutionState{options: bindingResolutionOptions{RequireSuite: true, RequireMySekai: true}}
	if !state.record(1, usable, nil) {
		t.Fatal("usable binding was rejected")
	}
	if state.record(2, nil, nil) || !state.sawNoBinding {
		t.Fatal("nil binding was not recorded as missing")
	}
	state = bindingResolutionState{options: bindingResolutionOptions{RequireSuite: true}}
	invalid := &accountdata.ResolvedBinding{}
	if state.record(3, invalid, nil) || state.invalidBinding != invalid || state.invalidHID != 3 {
		t.Fatal("invalid private-data binding was not retained")
	}
	state.record(0, nil, accountdata.ErrNoBinding)
	if !state.sawNoBinding {
		t.Fatal("ErrNoBinding was not retained")
	}
	unexpected := errors.New("binding backend failed")
	state.record(0, nil, unexpected)
	if !errors.Is(state.unexpectedErr, unexpected) {
		t.Fatal("unexpected error was not retained")
	}
	if (&bindingResolutionState{}).valid(nil) {
		t.Fatal("nil binding is valid")
	}
	if (&bindingResolutionState{options: bindingResolutionOptions{RequireMySekai: true}}).valid(&accountdata.ResolvedBinding{}) {
		t.Fatal("binding without MySekai visibility is valid")
	}
}

func TestBindingResolutionStateReturnsEveryOutcome(t *testing.T) {
	ctx := context.Background()
	unexpected := errors.New("unexpected")
	if _, _, err := (&bindingResolutionState{unexpectedErr: unexpected}).result(ctx, "jp", false); !errors.Is(err, unexpected) {
		t.Fatalf("unexpected result error = %v", err)
	}
	binding := &accountdata.ResolvedBinding{PJSKUserID: "123"}
	hid, got, err := (&bindingResolutionState{invalidHID: 7, invalidBinding: binding}).result(ctx, "jp", true)
	if err != nil || hid != 7 || got != binding {
		t.Fatalf("invalid binding result = %d, %#v, %v", hid, got, err)
	}
	if _, _, err := (&bindingResolutionState{sawNoBinding: true}).result(ctx, "jp", false); !errors.Is(err, accountdata.ErrNoBinding) {
		t.Fatalf("missing binding error = %v", err)
	}
	if hid, got, err := (&bindingResolutionState{}).result(ctx, "jp", false); hid != 0 || got != nil || err != nil {
		t.Fatalf("empty result = %d, %#v, %v", hid, got, err)
	}
}
