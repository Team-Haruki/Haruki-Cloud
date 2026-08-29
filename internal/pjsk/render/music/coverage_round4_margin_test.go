package music

import (
	"errors"
	"testing"

	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
)

func TestMusicDetailMetaResidualPureBranches(t *testing.T) {
	var nilController *Controller
	nilController.enrichMusicDetailRequest(nil, renderregion.JP, nil, nil, nil, "")
	if got := nilController.resolveMusicDetailBPM(renderregion.JP, 1, "master"); got != nil {
		t.Fatalf("nil-controller BPM = %v, want nil", got)
	}

	controller := NewController(newRound4SearchSource(), nil, nil, nil, nil)
	if got := controller.resolveMusicDetailBPM(renderregion.JP, 0, "master"); got != nil {
		t.Fatalf("zero-ID BPM = %v, want nil", got)
	}
	if got := formatMusicDetailLength(-5); got != "0.0秒（0分0.0秒）" {
		t.Fatalf("negative length = %q", got)
	}
	if matrix, total := controller.resolveMusicDetailLeaderboard(renderregion.JP, nil, nil, 1); matrix != nil || total != 0 {
		t.Fatalf("invalid leaderboard = %#v, %d", matrix, total)
	}

	rows := []musicBoardRow{
		{MusicID: 2, Rank: 0},
		{MusicID: 2, Rank: 1},
	}
	info, total := findMusicDetailLeaderboardInfo(rows, 1, "solo", "score")
	if info != nil || total != 1 {
		t.Fatalf("missing leaderboard entry = %#v, %d", info, total)
	}
	if got := formatMusicDetailLeaderboardValue(musicBoardRow{}, "solo", "unsupported"); got != "-" {
		t.Fatalf("unsupported leaderboard value = %q", got)
	}
	if got := cloneMusicDetailLabels(nil, nil); got != nil {
		t.Fatalf("empty labels = %#v, want nil", got)
	}
}

func TestMusicAliasMergeResidualBranches(t *testing.T) {
	var nilController *Controller
	nilController.appendApprovedMusicAliases(nil, 1)

	controller := NewController(newRound4SearchSource(), nil, nil, nil, nil)
	req := &drawing.MusicDetailRequest{}
	controller.appendApprovedMusicAliases(nil, 1)
	controller.appendApprovedMusicAliases(req, 0)
	controller.appendApprovedMusicAliases(req, 1)

	controller.SetAliasResolver(&lookupTestAliasResolver{err: errors.New("aliases unavailable")})
	controller.appendApprovedMusicAliases(req, 1)
	if req.Alias != nil {
		t.Fatalf("failed alias lookup changed request: %#v", req.Alias)
	}

	controller.SetAliasResolver(&lookupTestAliasResolver{approved: map[int][]string{
		1: {" Alias ", "alias", "  ", "Second"},
	}})
	req.Alias = []string{"Existing", " existing "}
	controller.appendApprovedMusicAliases(req, 1)
	if len(req.Alias) != 3 || req.Alias[0] != "Existing" || req.Alias[1] != "Alias" || req.Alias[2] != "Second" {
		t.Fatalf("merged aliases = %#v", req.Alias)
	}
	if got := mergeMusicAliasTexts([]string{"  "}, " "); got != nil {
		t.Fatalf("blank aliases = %#v, want nil", got)
	}
}

func TestMusicBuilderLocalizationResidualBranches(t *testing.T) {
	if got := selectLocalizedTitle("base", "kr", []string{"中文"}); got != "" {
		t.Fatalf("non-Korean KR title = %q", got)
	}
	if got := selectLocalizedTitle("base", "en", []string{"中文"}); got != "" {
		t.Fatalf("non-Latin EN title = %q", got)
	}
	if got := normalizeVocalCaption("  Unknown   Caption  ", "", "", renderregion.JP); got != "Unknown   Caption" {
		t.Fatalf("unknown caption = %q", got)
	}
}
