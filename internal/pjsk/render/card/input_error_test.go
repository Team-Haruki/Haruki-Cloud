package card

import (
	"fmt"
	"testing"

	"haruki-cloud/internal/testutil"
	"haruki-cloud/utils/usererror"
)

func TestUnparsableCardQueriesAreUserInput(t *testing.T) {
	_, err := NewParser(nil).Parse("这不是卡牌指令")
	testutil.RequireUserError(t, err, usererror.CodeUsage, "common.unrecognized_args")
	if wrapped := fmt.Errorf("failed to search card box: %w", err); !usererror.IsInput(wrapped) {
		t.Fatal("wrapping lost the user-input mark")
	}
	_, err = NewParser(nil).ParseStrictFilter("???")
	if err == nil || !usererror.IsInput(err) {
		t.Fatalf("ParseStrictFilter error = %v", err)
	}
	_, err = (&Builder{}).BuildCardBoxRequest(nil, "", nil, false, false, false, false, "")
	testutil.RequireUserError(t, err, usererror.CodeNotFound, "card.not_found_unspecified")
}
