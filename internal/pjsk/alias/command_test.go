package alias

import (
	"context"
	"strings"
	"testing"

	json "haruki-cloud/internal/jsonutil"
)

func aliasCommandJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal command params: %v", err)
	}
	return raw
}

func executeAliasCommand(t *testing.T, ctx context.Context, service *Service, mode string, params any, want string) string {
	t.Helper()
	result, err := ExecuteCommand(ctx, service, mode, aliasCommandJSON(t, params))
	if err != nil {
		t.Fatalf("ExecuteCommand(%s): %v", mode, err)
	}
	text := string(result)
	if !strings.Contains(text, want) {
		t.Fatalf("ExecuteCommand(%s) = %q, want substring %q", mode, text, want)
	}
	return text
}

func TestExecuteCommandReviewLifecycle(t *testing.T) {
	ctx := context.Background()
	deps := newAliasTestDeps(t)
	deps.addMusic(t, ctx, 7001, "测试歌曲")
	deps.addApprovedAlias(t, ctx, PjskAliasTypeMusic, 7001, "旧别名")
	deps.addAdmin(t, ctx, "qq", "admin", "Command Admin")

	executeAliasCommand(t, ctx, deps.service, ModeQuery, QueryCommandParams{
		AliasType: PjskAliasTypeMusic,
		Target:    "7001",
	}, "旧别名")
	executeAliasCommand(t, ctx, deps.service, ModeAdd, AddCommandParams{
		AliasType:      PjskAliasTypeMusic,
		Platform:       " qq ",
		PlatformUserID: " submitter ",
		Target:         " 7001 ",
		Aliases:        []string{"待通过", "待拒绝", "批量拒绝一", "批量拒绝二"},
	}, "已提交 4 条")

	pending, err := deps.service.ListPending(ctx, "qq", "admin")
	if err != nil || len(pending) != 4 {
		t.Fatalf("pending records = %#v, %v", pending, err)
	}
	executeAliasCommand(t, ctx, deps.service, ModePendingList, ReviewListCommandParams{
		Platform: "qq", PlatformUserID: "admin",
	}, "当前共有 4 条")
	executeAliasCommand(t, ctx, deps.service, ModeSubmitter, SubmitterCommandParams{
		Platform: "qq", PlatformUserID: "admin", ReviewID: pending[0].ReviewID,
	}, "submitter")
	executeAliasCommand(t, ctx, deps.service, ModeApprove, ApproveCommandParams{
		Platform: "qq", PlatformUserID: "admin", ReviewIDs: []int64{pending[0].ReviewID},
	}, "已通过 1 条")
	executeAliasCommand(t, ctx, deps.service, ModeReject, RejectCommandParams{
		Platform: "qq", PlatformUserID: "admin", ReviewID: pending[1].ReviewID, Reason: " 重复 ",
	}, "重复")
	executeAliasCommand(t, ctx, deps.service, ModeBatchReject, BatchRejectCommandParams{
		Platform: "qq", PlatformUserID: "admin", ReviewIDs: []int64{pending[2].ReviewID, pending[3].ReviewID},
	}, "已批量拒绝 2 条")
	executeAliasCommand(t, ctx, deps.service, ModeBanSubmitter, BanSubmitterCommandParams{
		Platform:             "qq",
		PlatformUserID:       "admin",
		TargetPlatform:       "qq",
		TargetPlatformUserID: "submitter",
	}, "已禁止用户 qq:submitter")

	executeAliasCommand(t, ctx, deps.service, ModeDelete, DeleteCommandParams{
		AliasType:      PjskAliasTypeMusic,
		Platform:       "qq",
		PlatformUserID: "admin",
		Target:         "7001",
		Aliases:        []string{"旧别名", "待通过"},
	}, "已删除 2 条")
	executeAliasCommand(t, ctx, deps.service, ModeQuery, QueryCommandParams{
		AliasType: PjskAliasTypeMusic,
		Target:    "7001",
	}, "已审核别名: 无")
	executeAliasCommand(t, ctx, deps.service, ModePendingList, ReviewListCommandParams{
		Platform: "qq", PlatformUserID: "admin",
	}, "当前没有待审核别名")
}

func TestAliasCommandDecodersRejectInvalidParams(t *testing.T) {
	if _, err := ExecuteCommand(context.Background(), nil, "unknown", nil); err == nil {
		t.Fatal("unsupported mode unexpectedly succeeded")
	}

	tests := []struct {
		name string
		run  func(json.RawMessage) error
	}{
		{name: "delete missing", run: func(raw json.RawMessage) error { _, err := decodeDeleteParams(raw); return err }},
		{name: "delete malformed", run: func(json.RawMessage) error { _, err := decodeDeleteParams([]byte("{")); return err }},
		{name: "delete type", run: func(json.RawMessage) error {
			_, err := decodeDeleteParams([]byte(`{"alias_type":"bad","platform":"qq","platform_user_id":"1","target":"1"}`))
			return err
		}},
		{name: "delete identity", run: func(json.RawMessage) error {
			_, err := decodeDeleteParams([]byte(`{"alias_type":"music","target":"1"}`))
			return err
		}},
		{name: "delete target", run: func(json.RawMessage) error {
			_, err := decodeDeleteParams([]byte(`{"alias_type":"music","platform":"qq","platform_user_id":"1"}`))
			return err
		}},
		{name: "add missing", run: func(raw json.RawMessage) error { _, err := decodeAddParams(raw); return err }},
		{name: "add malformed", run: func(json.RawMessage) error { _, err := decodeAddParams([]byte("{")); return err }},
		{name: "add type", run: func(json.RawMessage) error {
			_, err := decodeAddParams([]byte(`{"alias_type":"bad","target":"1"}`))
			return err
		}},
		{name: "add target", run: func(json.RawMessage) error { _, err := decodeAddParams([]byte(`{"alias_type":"music"}`)); return err }},
		{name: "query missing", run: func(raw json.RawMessage) error { _, err := decodeQueryParams(raw); return err }},
		{name: "query malformed", run: func(json.RawMessage) error { _, err := decodeQueryParams([]byte("{")); return err }},
		{name: "query type", run: func(json.RawMessage) error {
			_, err := decodeQueryParams([]byte(`{"alias_type":"bad","target":"1"}`))
			return err
		}},
		{name: "query target", run: func(json.RawMessage) error { _, err := decodeQueryParams([]byte(`{"alias_type":"music"}`)); return err }},
		{name: "list missing", run: func(raw json.RawMessage) error { _, err := decodeReviewListParams(raw); return err }},
		{name: "list malformed", run: func(json.RawMessage) error { _, err := decodeReviewListParams([]byte("{")); return err }},
		{name: "list identity", run: func(json.RawMessage) error { _, err := decodeReviewListParams([]byte(`{}`)); return err }},
		{name: "submitter missing", run: func(raw json.RawMessage) error { _, err := decodeSubmitterParams(raw); return err }},
		{name: "submitter malformed", run: func(json.RawMessage) error { _, err := decodeSubmitterParams([]byte("{")); return err }},
		{name: "submitter identity", run: func(json.RawMessage) error { _, err := decodeSubmitterParams([]byte(`{"review_id":1}`)); return err }},
		{name: "submitter id", run: func(json.RawMessage) error {
			_, err := decodeSubmitterParams([]byte(`{"platform":"qq","platform_user_id":"1"}`))
			return err
		}},
		{name: "ban missing", run: func(raw json.RawMessage) error { _, err := decodeBanSubmitterParams(raw); return err }},
		{name: "ban malformed", run: func(json.RawMessage) error { _, err := decodeBanSubmitterParams([]byte("{")); return err }},
		{name: "ban identity", run: func(json.RawMessage) error {
			_, err := decodeBanSubmitterParams([]byte(`{"target_platform":"qq","target_platform_user_id":"1"}`))
			return err
		}},
		{name: "ban target", run: func(json.RawMessage) error {
			_, err := decodeBanSubmitterParams([]byte(`{"platform":"qq","platform_user_id":"1"}`))
			return err
		}},
		{name: "approve missing", run: func(raw json.RawMessage) error { _, err := decodeApproveParams(raw); return err }},
		{name: "approve malformed", run: func(json.RawMessage) error { _, err := decodeApproveParams([]byte("{")); return err }},
		{name: "approve identity", run: func(json.RawMessage) error { _, err := decodeApproveParams([]byte(`{}`)); return err }},
		{name: "reject missing", run: func(raw json.RawMessage) error { _, err := decodeRejectParams(raw); return err }},
		{name: "reject malformed", run: func(json.RawMessage) error { _, err := decodeRejectParams([]byte("{")); return err }},
		{name: "reject identity", run: func(json.RawMessage) error { _, err := decodeRejectParams([]byte(`{}`)); return err }},
		{name: "batch missing", run: func(raw json.RawMessage) error { _, err := decodeBatchRejectParams(raw); return err }},
		{name: "batch malformed", run: func(json.RawMessage) error { _, err := decodeBatchRejectParams([]byte("{")); return err }},
		{name: "batch identity", run: func(json.RawMessage) error {
			_, err := decodeBatchRejectParams([]byte(`{"review_ids":[1]}`))
			return err
		}},
		{name: "batch ids", run: func(json.RawMessage) error {
			_, err := decodeBatchRejectParams([]byte(`{"platform":"qq","platform_user_id":"1"}`))
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.run(nil); err == nil {
				t.Fatal("invalid params unexpectedly succeeded")
			}
		})
	}
}
