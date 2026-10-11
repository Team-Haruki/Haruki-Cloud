package handler

import (
	"context"
	"strings"
	"testing"

	"haruki-cloud/internal/i18n"
	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/internal/pjsk/render/card"
	"haruki-cloud/internal/pjsk/render/common"
	rendermusic "haruki-cloud/internal/pjsk/render/music"
	"haruki-cloud/utils/usererror"
)

// Card handlers defer filter parsing until rendering. Merely dispatching an
// example therefore cannot detect invalid filters or missing required ones.
func TestHelpCardExamplesReachFilterParser(t *testing.T) {
	EnsureCommandHandlersRegistered()
	for _, key := range []string{"card_detail", "card_list", "card_box", "card_image"} {
		for _, example := range helpDocExamples(helpDocMarkdown(t, key)) {
			t.Run(example, func(t *testing.T) {
				request, err := dispatchForTest(context.Background(), helpExampleEvent(example))
				if err != nil {
					t.Fatal(err)
				}
				if err := validateHelpCardQuery(request); err != nil {
					t.Fatalf("%s: downstream card parser rejects example: %s", key, usererror.LogText(err))
				}
			})
		}
	}
}

func validateHelpCardQuery(request *CommandRequest) error {
	query := card.Query{Query: request.Query}
	if len(request.Params) > 0 {
		if err := json.Unmarshal(request.Params, &query); err != nil {
			return err
		}
	}
	if request.Mode == cardBoxCommand && strings.TrimSpace(query.Query) == "" {
		return nil
	}
	parser := card.NewParser(common.DefaultNicknames)
	if query.StrictFilterOnly {
		_, err := parser.ParseStrictFilter(query.Query)
		return err
	}
	_, err := parser.Parse(query.Query)
	return err
}

func TestHelpCardValidationRejectsPreviouslyMisleadingExamples(t *testing.T) {
	EnsureCommandHandlersRegistered()
	for _, example := range []string{"/cards", "/卡面 miku 特训前"} {
		request, err := dispatchForTest(context.Background(), helpExampleEvent(example))
		if err != nil {
			t.Fatal(err)
		}
		if err := validateHelpCardQuery(request); err == nil {
			t.Errorf("accepted invalid downstream example %q", example)
		}
	}
}

func TestHelpPlannerMultipleSongExampleKeepsSeparateSongs(t *testing.T) {
	EnsureCommandHandlersRegistered()
	found := false
	for _, example := range helpDocExamples(helpDocMarkdown(t, "event_planner")) {
		request, err := dispatchForTest(context.Background(), helpExampleEvent(example))
		if err != nil {
			t.Fatal(err)
		}
		var params eventPlannerCommandParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			t.Fatal(err)
		}
		if !params.CurrentPointSet {
			continue
		}
		found = true
		songs := eventPlannerSongsForRequest(params, buildEventPlannerBaseDeckQuery("jp", params.Deck))
		if params.TargetPoint != 10000000 || params.CurrentPoint != 1200000 || len(songs) != 2 {
			t.Fatalf("example %q: target=%d current=%d songs=%+v", example, params.TargetPoint, params.CurrentPoint, songs)
		}
		if songs[0].Difficulty != "expert" || songs[1].Difficulty != "hard" {
			t.Fatalf("song difficulties: %+v", songs)
		}
	}
	if !found {
		t.Fatal("missing multi-song example with current PT")
	}
}

func TestHelpMusicBoardExamplePreservesCalculationInputs(t *testing.T) {
	EnsureCommandHandlersRegistered()
	found := false
	for _, example := range helpDocExamples(helpDocMarkdown(t, "score_music-board")) {
		if !strings.Contains(example, "实效230") {
			continue
		}
		found = true
		request, err := dispatchForTest(context.Background(), helpExampleEvent(example))
		if err != nil {
			t.Fatal(err)
		}
		var params rendermusic.BoardQuery
		if err := json.Unmarshal(request.Params, &params); err != nil {
			t.Fatal(err)
		}
		if params.LiveType != "multi" || params.Target != "pt/time" || params.Power != 300000 || params.DeckBonus != 200 || params.PlayInterval != 30 || len(params.Skills) != 5 || params.Skills[0] != 2.3 || len(params.SpecQueries) != 0 {
			t.Fatalf("example %q lost calculation inputs: %+v", example, params)
		}
	}
	if !found {
		t.Fatal("missing explicit calculation example")
	}
}

func TestHelpCommonFlagsAndExternalMonitorRegion(t *testing.T) {
	EnsureCommandHandlersRegistered()
	general, err := commandHelpMarkdown(&CommandRequest{CommandPath: "music"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(general, i18n.T("usage.help.common_flags")) || !strings.Contains(general, i18n.T("usage.help.region_argument")) {
		t.Fatal("missing common arguments")
	}
	monitor, err := commandHelpMarkdown(&CommandRequest{CommandPath: BirthdayMonitorHelpPath})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(monitor, i18n.T("usage.help.common_flags")) || strings.Contains(monitor, i18n.T("usage.help.region_argument")) {
		t.Fatal("external monitor advertised unsupported generic flags")
	}
	if !strings.Contains(monitor, "`jp` `tw` `kr` `en`") || !strings.Contains(monitor, i18n.T("usage.help.region_no_cn_mysekai")) {
		t.Fatal("external monitor is missing supported regions")
	}
}
