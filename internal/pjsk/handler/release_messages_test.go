package handler

import (
	"errors"
	"testing"

	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/accountdata"
	renderevent "haruki-cloud/internal/pjsk/render/event"
	"haruki-cloud/internal/pjsk/render/releasecheck"
)

func TestNormalizeCardUserFacingErrorReturnsUnreleasedReplayError(t *testing.T) {
	err := normalizeCardUserFacingError(releasecheck.New(releasecheck.KindCard, "", 1001))
	assertReplayErrorText(t, err, "该卡牌还未上线")
}

func TestNormalizeCardUserFacingErrorExtractsSekaiCardNotFound(t *testing.T) {
	err := normalizeCardUserFacingError(errString("failed to search card: query card 662: sekai: card not found"))
	assertReplayErrorText(t, err, "找不到特定的卡牌: 662")
}

func TestNormalizeCardUserFacingErrorExtractsFilterCardNotFound(t *testing.T) {
	err := normalizeCardUserFacingError(errString("failed to search card list: no cards found for filter: miku secret"))
	assertReplayErrorText(t, err, "找不到特定的卡牌: miku secret")
}

func TestNormalizeCardUserFacingErrorTranslatesCardServiceErrors(t *testing.T) {
	err := normalizeCardUserFacingError(errString("no card data source for region jp"))
	assertReplayErrorText(t, err, "卡牌服务未就绪，请稍后再试")
}

func TestNormalizeMusicUserFacingErrorReturnsUnreleasedReplayError(t *testing.T) {
	err := normalizeMusicUserFacingError(releasecheck.New(releasecheck.KindMusic, "Future Song", 0))
	assertReplayErrorText(t, err, "该歌曲还未上线")
}

func TestNormalizeMusicUserFacingErrorTranslatesServiceErrors(t *testing.T) {
	err := normalizeMusicUserFacingError(errString("music controller is not configured"))
	assertReplayErrorText(t, err, "歌曲服务未就绪，请稍后再试")
}

func TestNormalizeMusicUserFacingErrorReturnsRegionSpecificNotFoundReplayError(t *testing.T) {
	err := normalizeMusicUserFacingErrorForLookup(errString("music not found: Tell Your World"), "cn", "")
	assertReplayErrorText(t, err, "CN服找不到特定的歌: Tell Your World\n如果需要查其他服务器歌曲请加区服前缀")
}

func TestNormalizeMusicUserFacingErrorExtractsWrappedNotFoundQuery(t *testing.T) {
	err := normalizeMusicUserFacingErrorForLookup(errString("failed to search music by title or alias: music not found: 虾ex"), "jp", "fallback")
	assertReplayErrorText(t, err, "JP服找不到特定的歌: 虾ex\n如果需要查其他服务器歌曲请加区服前缀")
}

func TestNormalizeMusicUserFacingErrorExtractsSekaiMusicNotFound(t *testing.T) {
	err := normalizeMusicUserFacingErrorForLookup(errString("failed to search music: query music 662: sekai: music not found"), "cn", "")
	assertReplayErrorText(t, err, "CN服找不到特定的歌: 662\n如果需要查其他服务器歌曲请加区服前缀")
}

func TestNormalizeMusicUserFacingErrorTreatsBanIndexAsLookupMiss(t *testing.T) {
	err := normalizeMusicUserFacingErrorForLookup(errString("failed to search music: ban event index out of range: 6"), "jp", "miku6")
	assertReplayErrorText(t, err, "JP服找不到特定的歌: miku6\n如果需要查其他服务器歌曲请加区服前缀")
}

func TestNormalizeMusicUserFacingErrorTreatsMissingBanEventsAsLookupMiss(t *testing.T) {
	err := normalizeMusicUserFacingErrorForLookup(errString("failed to search music: no ban events found for character 20"), "cn", "mzk5")
	assertReplayErrorText(t, err, "CN服找不到特定的歌: mzk5\n如果需要查其他服务器歌曲请加区服前缀")
}

func TestNormalizeEventUserFacingErrorReturnsUnreleasedReplayError(t *testing.T) {
	err := normalizeEventUserFacingError(releasecheck.New(releasecheck.KindEvent, "", 2001))
	assertReplayErrorText(t, err, "该活动还未上线")
}

func TestNormalizeEventUserFacingErrorReturnsNoOngoingReplayError(t *testing.T) {
	err := normalizeEventUserFacingError(renderevent.ErrNoOngoingEvent)
	assertReplayErrorText(t, err, "当前无正在举办的活动")
}

func TestNormalizeEventUserFacingErrorTranslatesRequiredID(t *testing.T) {
	err := normalizeEventUserFacingError(errString("event id is required"))
	assertReplayErrorText(t, err, "请提供要查询的活动")
}

func TestNormalizeEventUserFacingErrorTranslatesMissingMasterdata(t *testing.T) {
	err := normalizeEventUserFacingErrorForRegion(errString("query event 203: ent: event not found"), "tw")
	assertReplayErrorText(t, err, "当前TW服未找到该活动数据，如需查询其他服务器活动请加对应区服前缀")
}

func TestNormalizeEventUserFacingErrorTranslatesEmptyRegionEventData(t *testing.T) {
	err := normalizeEventUserFacingErrorForRegion(errString("no events found for region cn"), "cn")
	assertReplayErrorText(t, err, "当前CN服未找到该活动数据，如需查询其他服务器活动请加对应区服前缀")
}

func TestNormalizeGachaUserFacingErrorReturnsUnreleasedReplayError(t *testing.T) {
	err := normalizeGachaUserFacingError(releasecheck.New(releasecheck.KindGacha, "", 3001))
	assertReplayErrorText(t, err, "该卡池还未上线")
}

func TestNormalizeGachaUserFacingErrorTranslatesNotFound(t *testing.T) {
	err := normalizeGachaUserFacingError(errString("gacha not found for event: 42"))
	assertReplayErrorText(t, err, "找不到特定的卡池")
}

func TestNormalizeReleaseMessagesPreserveNonReleaseErrors(t *testing.T) {
	testCases := []struct {
		name      string
		normalize func(error) error
		input     error
	}{
		{
			name:      "card no binding",
			normalize: normalizeCardUserFacingError,
			input:     accountdata.ErrNoBinding,
		},
		{
			name:      "card costume query failure",
			normalize: normalizeCardUserFacingError,
			input:     errString("query card costumes for card 123: database unavailable"),
		},
		{
			name:      "music replay error",
			normalize: normalizeMusicUserFacingError,
			input:     onebot11.NewReplayError("keep me"),
		},
		{
			name:      "event plain error",
			normalize: normalizeEventUserFacingError,
			input:     errString("plain error"),
		},
		{
			name:      "gacha no binding",
			normalize: normalizeGachaUserFacingError,
			input:     accountdata.ErrNoBinding,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.normalize(tc.input)
			if !errors.Is(got, tc.input) {
				t.Fatalf("expected error to be preserved, got %v", got)
			}
		})
	}
}
