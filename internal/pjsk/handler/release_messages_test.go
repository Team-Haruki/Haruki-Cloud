package handler

import (
	"errors"
	"testing"

	"haruki-cloud/internal/onebot11"
	"haruki-cloud/internal/pjsk/accountdata"
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

func TestNormalizeEventUserFacingErrorReturnsUnreleasedReplayError(t *testing.T) {
	err := normalizeEventUserFacingError(releasecheck.New(releasecheck.KindEvent, "", 2001))
	assertReplayErrorText(t, err, "该活动还未上线")
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
