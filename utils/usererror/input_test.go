package usererror

import (
	"errors"
	"fmt"
	"testing"
)

func TestInputKeepsTheMessageAndSurvivesWrapping(t *testing.T) {
	err := Inputf("无法解析的指令: %s", "xyz")
	if err.Error() != "无法解析的指令: xyz" || !IsInput(err) {
		t.Fatalf("Inputf = %q, IsInput = %v", err, IsInput(err))
	}
	wrapped := fmt.Errorf("failed to search card: %w", err)
	if wrapped.Error() != "failed to search card: 无法解析的指令: xyz" || !IsInput(wrapped) {
		t.Fatalf("wrapped = %q, IsInput = %v", wrapped, IsInput(wrapped))
	}
	base := errors.New("cards are required")
	marked := Input(base)
	if marked.Error() != base.Error() || !errors.Is(marked, base) || !IsInput(marked) {
		t.Fatalf("Input(base) = %q", marked)
	}
	if Input(marked) != marked {
		t.Fatal("Input re-wraps an already marked error")
	}
	if Input(nil) != nil || IsInput(nil) || IsInput(errors.New("database unavailable")) {
		t.Fatal("nil or unmarked errors classified as input")
	}
}
