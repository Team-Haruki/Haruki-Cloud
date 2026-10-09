package usererror

import (
	"errors"
	"fmt"
	"testing"

	"haruki-cloud/internal/i18n"
)

func TestIsInputJudgesTypedCodesOnly(t *testing.T) {
	err := Unrecognized()
	wrapped := fmt.Errorf("failed to search card: %w", err)
	if !IsInput(err) || !IsInput(wrapped) || CodeOf(wrapped) != CodeUsage {
		t.Fatalf("IsInput(Unrecognized) = %v, wrapped %v", IsInput(err), IsInput(wrapped))
	}
	if IsInput(nil) || IsInput(errors.New("database unavailable")) || IsInput(Unavailable(i18n.FeatureRender, nil)) {
		t.Fatal("nil, untyped or failure errors classified as input")
	}
}

func TestSetupMisuseAndUnrecognized(t *testing.T) {
	setup := Setup(i18n.M("binding.required"))
	if setup.Code != CodeSetup || !IsExpected(setup) || IsInput(setup) {
		t.Fatalf("Setup = %+v", setup)
	}
	misuse := Misuse(i18n.M("common.no_args"))
	if misuse.Code != CodeUsage || misuse.Message.ID != "common.no_args" || !IsInput(misuse) {
		t.Fatalf("Misuse = %+v", misuse)
	}
	if got := Unrecognized(); got.Code != CodeUsage || got.Message.ID != "common.unrecognized_args" {
		t.Fatalf("Unrecognized = %+v", got)
	}
}
