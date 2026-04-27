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

func TestNormalizeMusicUserFacingErrorReturnsUnreleasedReplayError(t *testing.T) {
	err := normalizeMusicUserFacingError(releasecheck.New(releasecheck.KindMusic, "Future Song", 0))
	assertReplayErrorText(t, err, "该歌曲还未上线")
}

func TestNormalizeMusicUserFacingErrorReturnsRegionSpecificNotFoundReplayError(t *testing.T) {
	err := normalizeMusicUserFacingErrorForLookup(errString("music not found: Tell Your World"), "cn", "")
	assertReplayErrorText(t, err, "CN服找不到特定的歌: Tell Your World\n如果需要查其他区服的歌曲请加区服前缀，如需要查日服的请加jp区服前缀")
}

func TestNormalizeMusicUserFacingErrorExtractsWrappedNotFoundQuery(t *testing.T) {
	err := normalizeMusicUserFacingErrorForLookup(errString("failed to search music by title or alias: music not found: 虾ex"), "jp", "fallback")
	assertReplayErrorText(t, err, "JP服找不到特定的歌: 虾ex\n如果需要查其他区服的歌曲请加区服前缀，如需要查日服的请加jp区服前缀")
}

func TestNormalizeMusicUserFacingErrorExtractsSekaiMusicNotFound(t *testing.T) {
	err := normalizeMusicUserFacingErrorForLookup(errString("failed to search music: query music 662: sekai: music not found"), "cn", "")
	assertReplayErrorText(t, err, "CN服找不到特定的歌: 662\n如果需要查其他区服的歌曲请加区服前缀，如需要查日服的请加jp区服前缀")
}

func TestNormalizeMusicUserFacingErrorTreatsBanIndexAsLookupMiss(t *testing.T) {
	err := normalizeMusicUserFacingErrorForLookup(errString("failed to search music: ban event index out of range: 6"), "jp", "miku6")
	assertReplayErrorText(t, err, "JP服找不到特定的歌: miku6\n如果需要查其他区服的歌曲请加区服前缀，如需要查日服的请加jp区服前缀")
}

func TestNormalizeEventUserFacingErrorReturnsUnreleasedReplayError(t *testing.T) {
	err := normalizeEventUserFacingError(releasecheck.New(releasecheck.KindEvent, "", 2001))
	assertReplayErrorText(t, err, "该活动还未上线")
}

func TestNormalizeEventUserFacingErrorReturnsNoOngoingReplayError(t *testing.T) {
	err := normalizeEventUserFacingError(renderevent.ErrNoOngoingEvent)
	assertReplayErrorText(t, err, "当前无正在举办的活动")
}

func TestNormalizeGachaUserFacingErrorReturnsUnreleasedReplayError(t *testing.T) {
	err := normalizeGachaUserFacingError(releasecheck.New(releasecheck.KindGacha, "", 3001))
	assertReplayErrorText(t, err, "该卡池还未上线")
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
