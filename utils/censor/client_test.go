package censor

import "testing"

func TestBaiduTextCensorResultError(t *testing.T) {
	result := map[string]any{
		"error_code": float64(17),
		"error_msg":  "Open api daily request limit reached",
	}

	if err := baiduTextCensorResultError(result); err == nil {
		t.Fatal("baiduTextCensorResultError() = nil, want error")
	}
}

func TestBaiduTextCensorResultErrorAllowsNormalResponse(t *testing.T) {
	result := map[string]any{
		"conclusion": string(ResultCompliant),
	}

	if err := baiduTextCensorResultError(result); err != nil {
		t.Fatalf("baiduTextCensorResultError() = %v, want nil", err)
	}
}
