package handler

import (
	"context"
	"encoding/json"
	"fmt"

	"haruki-cloud/internal/pjsk/parser"
	renderapp "haruki-cloud/internal/pjsk/render/app"
	"haruki-cloud/internal/pjsk/render/card"
	"haruki-cloud/internal/pjsk/render/deck"
	"haruki-cloud/internal/pjsk/render/education"
	"haruki-cloud/internal/pjsk/render/event"
	"haruki-cloud/internal/pjsk/render/gacha"
	"haruki-cloud/internal/pjsk/render/music"
	"haruki-cloud/internal/pjsk/render/mysekai"
	"haruki-cloud/internal/pjsk/render/profile"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	"haruki-cloud/internal/pjsk/render/sk"
	"haruki-cloud/internal/pjsk/render/stamp"
	"haruki-cloud/utils/drawing"
)

// Execute routes a ResolvedCommand to the corresponding render controller,
// returning the rendered PNG bytes or an error.
// This is the main bridge between the parser output and the render system.
func Execute(_ context.Context, resolved *parser.ResolvedCommand, app *renderapp.App) ([]byte, error) {
	if resolved == nil {
		return nil, fmt.Errorf("bridge: nil resolved command")
	}
	if app == nil {
		return nil, fmt.Errorf("bridge: nil render app")
	}

	switch resolved.Module {
	case parser.ModuleCard:
		return executeCard(resolved, app)
	case parser.ModuleEvent:
		return executeEvent(resolved, app)
	case parser.ModuleMusic:
		return executeMusic(resolved, app)
	case parser.ModuleGacha:
		return executeGacha(resolved, app)
	case parser.ModuleDeck:
		return executeDeck(resolved, app)
	case parser.ModuleEducation:
		return executeEducation(resolved, app)
	case parser.ModuleSK:
		return executeSK(resolved, app)
	case parser.ModuleScore:
		return executeScore(resolved, app)
	case parser.ModuleProfile:
		return executeProfile(resolved, app)
	case parser.ModuleMysekai:
		return executeMysekai(resolved, app)
	case parser.ModuleStamp:
		return executeStamp(resolved, app)
	case parser.ModuleMisc:
		return executeMisc(resolved, app)
	default:
		return nil, fmt.Errorf("bridge: unsupported module %v", resolved.Module)
	}
}

func executeCard(r *parser.ResolvedCommand, app *renderapp.App) ([]byte, error) {
	switch r.Mode {
	case "card-detail":
		q := card.Query{Query: r.Query, Region: r.Region}
		mergeParams(r.Params, &q)
		return app.Cards.RenderCardDetail(q)
	case "card-list":
		q := card.ListRequest{Region: r.Region}
		mergeParams(r.Params, &q)
		return app.Cards.RenderCardList(q)
	case "card-box":
		queries := []card.Query{{Query: r.Query, Region: r.Region}}
		return app.Cards.RenderCardBox(queries)
	default:
		return nil, fmt.Errorf("bridge: unsupported card mode %q", r.Mode)
	}
}

func executeEvent(r *parser.ResolvedCommand, app *renderapp.App) ([]byte, error) {
	region := renderregion.Value(r.Region)
	switch r.Mode {
	case "event-detail":
		q := event.DetailQuery{Region: region}
		mergeParams(r.Params, &q)
		return app.Events.RenderEventDetail(q)
	case "event-list":
		q := event.ListQuery{Region: region}
		mergeParams(r.Params, &q)
		return app.Events.RenderEventList(q)
	case "event-record":
		req := drawing.EventRecordRequest{}
		mergeParams(r.Params, &req)
		return app.Events.RenderEventRecord(req)
	default:
		return nil, fmt.Errorf("bridge: unsupported event mode %q", r.Mode)
	}
}

func executeMusic(r *parser.ResolvedCommand, app *renderapp.App) ([]byte, error) {
	switch r.Mode {
	case "music-detail":
		q := music.Query{Query: r.Query, Region: r.Region}
		mergeParams(r.Params, &q)
		return app.Music.RenderMusicDetail(q)
	case "music-list":
		q := music.ListQuery{Region: r.Region}
		mergeParams(r.Params, &q)
		return app.Music.RenderMusicList(q)
	case "music-chart":
		q := music.ChartQuery{Query: r.Query, Region: r.Region}
		mergeParams(r.Params, &q)
		return app.Music.RenderMusicChart(q)
	case "music-progress":
		q := music.ProgressQuery{Region: r.Region}
		mergeParams(r.Params, &q)
		return app.Music.RenderMusicProgress(q)
	case "music-rewards":
		q := music.RewardsBasicQuery{Region: r.Region}
		mergeParams(r.Params, &q)
		return app.Music.RenderMusicRewardsBasic(q)
	default:
		return nil, fmt.Errorf("bridge: unsupported music mode %q", r.Mode)
	}
}

func executeGacha(r *parser.ResolvedCommand, app *renderapp.App) ([]byte, error) {
	region := renderregion.Value(r.Region)
	switch r.Mode {
	case "gacha":
		q := gacha.ListQuery{Region: region}
		mergeParams(r.Params, &q)
		return app.Gachas.RenderGachaList(q)
	default:
		return nil, fmt.Errorf("bridge: unsupported gacha mode %q", r.Mode)
	}
}

func executeDeck(r *parser.ResolvedCommand, app *renderapp.App) ([]byte, error) {
	recommendType := ""
	switch r.Mode {
	case "deck-event":
		recommendType = "event"
	case "deck-challenge":
		recommendType = "challenge"
	case "deck-no-event":
		recommendType = "no_event"
	case "deck-bonus":
		recommendType = "bonus"
	case "deck-mysekai":
		recommendType = "mysekai"
	default:
		return nil, fmt.Errorf("bridge: unsupported deck mode %q", r.Mode)
	}
	q := deck.AutoQuery{Region: r.Region, RecommendType: recommendType}
	mergeParams(r.Params, &q)
	return app.Decks.RenderAutoRecommend(q)
}

func executeEducation(r *parser.ResolvedCommand, app *renderapp.App) ([]byte, error) {
	region := renderregion.Value(r.Region)
	switch r.Mode {
	case "education-challenge":
		q := education.ChallengeLiveQuery{Region: region}
		mergeParams(r.Params, &q)
		return app.Edu.RenderChallengeLiveDetails(q)
	case "education-power":
		req := drawing.PowerBonusDetailRequest{}
		mergeParams(r.Params, &req)
		return app.Edu.RenderPowerBonusDetail(req)
	case "education-area":
		req := drawing.AreaItemUpgradeMaterialsRequest{}
		mergeParams(r.Params, &req)
		return app.Edu.RenderAreaItemUpgradeMaterials(req)
	case "education-bonds":
		req := drawing.BondsRequest{}
		mergeParams(r.Params, &req)
		return app.Edu.RenderBonds(req)
	case "education-leader":
		req := drawing.LeaderCountRequest{}
		mergeParams(r.Params, &req)
		return app.Edu.RenderLeaderCount(req)
	default:
		return nil, fmt.Errorf("bridge: unsupported education mode %q", r.Mode)
	}
}

func executeSK(r *parser.ResolvedCommand, app *renderapp.App) ([]byte, error) {
	switch r.Mode {
	case "sk-line":
		req := sk.LineRequest{}
		mergeParams(r.Params, &req)
		return app.SK.RenderLine(req)
	case "sk-query":
		req := drawing.SKRequest{}
		mergeParams(r.Params, &req)
		return app.SK.RenderQuery(req)
	case "sk-check-room":
		req := drawing.CFRequest{}
		mergeParams(r.Params, &req)
		return app.SK.RenderCheckRoom(req)
	case "sk-speed":
		req := drawing.SpeedRequest{}
		mergeParams(r.Params, &req)
		return app.SK.RenderSpeed(req)
	case "sk-player-trace":
		req := drawing.PlayerTraceRequest{}
		mergeParams(r.Params, &req)
		return app.SK.RenderPlayerTrace(req)
	case "sk-rank-trace":
		req := drawing.RankTraceRequest{}
		mergeParams(r.Params, &req)
		return app.SK.RenderRankTrace(req)
	case "sk-winrate":
		req := drawing.WinRateRequest{}
		mergeParams(r.Params, &req)
		return app.SK.RenderWinRate(req)
	default:
		return nil, fmt.Errorf("bridge: unsupported sk mode %q", r.Mode)
	}
}

func executeScore(r *parser.ResolvedCommand, app *renderapp.App) ([]byte, error) {
	switch r.Mode {
	case "score-control":
		req := drawing.ScoreControlRequest{}
		mergeParams(r.Params, &req)
		return app.Score.RenderScoreControl(req)
	case "score-custom-room":
		req := drawing.CustomRoomScoreRequest{}
		mergeParams(r.Params, &req)
		return app.Score.RenderCustomRoomScore(req)
	case "score-music-meta":
		var req []drawing.MusicMetaRequest
		if r.Params != nil {
			if err := json.Unmarshal(r.Params, &req); err != nil {
				return nil, fmt.Errorf("bridge: unmarshal music-meta params: %w", err)
			}
		}
		return app.Score.RenderMusicMeta(req)
	case "score-music-board":
		req := drawing.MusicBoardRequest{}
		mergeParams(r.Params, &req)
		return app.Score.RenderMusicBoard(req)
	default:
		return nil, fmt.Errorf("bridge: unsupported score mode %q", r.Mode)
	}
}

func executeProfile(r *parser.ResolvedCommand, app *renderapp.App) ([]byte, error) {
	switch r.Mode {
	case "profile":
		q := profile.Query{Region: r.Region}
		mergeParams(r.Params, &q)
		return app.Profiles.RenderProfile(q)
	default:
		return nil, fmt.Errorf("bridge: unsupported profile mode %q", r.Mode)
	}
}

func executeMysekai(r *parser.ResolvedCommand, app *renderapp.App) ([]byte, error) {
	switch r.Mode {
	case "mysekai-resource":
		q := mysekai.ResourceQuery{Region: r.Region}
		mergeParams(r.Params, &q)
		return app.MySekai.RenderResource(q)
	case "mysekai-fixture-list":
		q := mysekai.FixtureListQuery{Region: r.Region}
		mergeParams(r.Params, &q)
		return app.MySekai.RenderFixtureList(q)
	case "mysekai-fixture-detail":
		q := mysekai.FixtureDetailQuery{Region: r.Region, Query: r.Query}
		mergeParams(r.Params, &q)
		return app.MySekai.RenderFixtureDetail(q)
	case "mysekai-door-upgrade":
		q := mysekai.DoorUpgradeQuery{Region: r.Region, Query: r.Query}
		mergeParams(r.Params, &q)
		return app.MySekai.RenderDoorUpgrade(q)
	case "mysekai-music-record":
		q := mysekai.MusicRecordQuery{Region: r.Region}
		mergeParams(r.Params, &q)
		return app.MySekai.RenderMusicRecord(q)
	case "mysekai-talk-list":
		q := mysekai.TalkListQuery{Region: r.Region, Query: r.Query}
		mergeParams(r.Params, &q)
		return app.MySekai.RenderTalkList(q)
	default:
		return nil, fmt.Errorf("bridge: unsupported mysekai mode %q", r.Mode)
	}
}

func executeStamp(r *parser.ResolvedCommand, app *renderapp.App) ([]byte, error) {
	region := renderregion.Value(r.Region)
	switch r.Mode {
	case "stamp-list":
		q := stamp.ListQuery{Region: region}
		mergeParams(r.Params, &q)
		return app.Stamps.RenderStampList(q)
	default:
		return nil, fmt.Errorf("bridge: unsupported stamp mode %q", r.Mode)
	}
}

func executeMisc(r *parser.ResolvedCommand, app *renderapp.App) ([]byte, error) {
	switch r.Mode {
	case "misc-birthday":
		req := drawing.CharaBirthdayRequest{}
		mergeParams(r.Params, &req)
		return app.Misc.RenderCharaBirthday(req)
	default:
		return nil, fmt.Errorf("bridge: unsupported misc mode %q", r.Mode)
	}
}

// mergeParams unmarshals the JSON params from ResolvedCommand into the target struct,
// allowing handler-set fields to override defaults. Fields not present in params
// remain at their zero/pre-set values.
func mergeParams(params json.RawMessage, target interface{}) {
	if len(params) == 0 {
		return
	}
	_ = json.Unmarshal(params, target)
}
