package card

import (
	"fmt"
	"testing"

	"haruki-cloud/utils/usererror"
)

func TestUnparsableCardQueriesAreUserInput(t *testing.T) {
	_, err := NewParser(nil).Parse("这不是卡牌指令")
	if err == nil || err.Error() != "无法解析的指令: 这不是卡牌指令" || !usererror.IsInput(err) {
		t.Fatalf("Parse error = %v (input %v)", err, usererror.IsInput(err))
	}
	if wrapped := fmt.Errorf("failed to search card box: %w", err); !usererror.IsInput(wrapped) {
		t.Fatal("wrapping lost the user-input mark")
	}
	_, err = NewParser(nil).ParseStrictFilter("???")
	if err == nil || !usererror.IsInput(err) {
		t.Fatalf("ParseStrictFilter error = %v", err)
	}
	if _, err := (&Builder{}).BuildCardBoxRequest(nil, "", nil, false, false, false, false, ""); err == nil || err.Error() != "cards are required" || !usererror.IsInput(err) {
		t.Fatalf("empty card box error = %v", err)
	}
}
