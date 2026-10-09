package pjsk

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"haruki-cloud/internal/i18n"
	"haruki-cloud/internal/onebot11"
	"haruki-cloud/utils/usererror"
)

func TestCommandErrorEnvelopeShowsTypedUserErrorWithoutCause(t *testing.T) {
	cause := errors.New("toolbox api error: status 502 at http://127.0.0.1:8080/api/private/x")
	for _, err := range []error{
		usererror.Unavailable(i18n.FeatureToolbox, cause),
		fmt.Errorf("bind: %w", usererror.Unavailable(i18n.FeatureToolbox, cause)),
	} {
		envelope := commandErrorEnvelope(err, "profile/bind", "/绑定", false)
		segments, ok := envelope.Data.([]onebot11.Segment)
		if !ok || len(segments) != 1 {
			t.Fatalf("envelope data = %#v", envelope.Data)
		}
		text := segments[0].Data.(onebot11.TextData).Text
		if text != i18n.Unavailable(i18n.FeatureToolbox).String() || strings.Contains(text, "502") || strings.Contains(text, "toolbox") {
			t.Fatalf("typed error reply = %q", text)
		}
	}
}

func TestTypedUserErrorsClassifyByCode(t *testing.T) {
	expected := []error{
		usererror.BadParam("x", i18n.M("moderation.qq_invalid")),
		usererror.Forbidden(i18n.M("moderation.not_admin")),
		usererror.ReadOnly(),
	}
	for _, err := range expected {
		if !isExpectedCommandError(err) {
			t.Fatalf("%v should be an expected rejection", err)
		}
	}
	for _, err := range []error{
		usererror.Internal(errors.New("boom")),
		usererror.Misconfigured(nil),
		usererror.Timeout(i18n.FeatureRanking, nil),
	} {
		if isExpectedCommandError(err) {
			t.Fatalf("%v should be logged as a failure", err)
		}
	}
}

func TestTypedUserErrorTextFallsBackOnSensitiveURL(t *testing.T) {
	err := usererror.Invalid(i18n.Verbatim("see http://127.0.0.1:6666/x"))
	if got := typedUserErrorText(err); got != genericClientErrorText {
		t.Fatalf("typedUserErrorText() = %q", got)
	}
	if got := typedUserErrorText(usererror.Invalid(i18n.Verbatim(" "))); got != genericClientErrorText {
		t.Fatalf("empty typed text = %q", got)
	}
}
