package alias

import (
	"context"
	"encoding/json"
	"fmt"
	sonic "github.com/bytedance/sonic"
	"strings"
)

func ExecuteCommand(ctx context.Context, service *Service, mode string, raw json.RawMessage) ([]byte, error) {
	switch mode {
	case ModeDelete:
		params, err := decodeDeleteParams(raw)
		if err != nil {
			return nil, err
		}
		records, err := service.Delete(ctx, params.AliasType, params.Platform, params.PlatformUserID, params.Target, params.Aliases)
		if err != nil {
			return nil, err
		}
		lines := []string{fmt.Sprintf("已删除 %d 条%s已审核别名：", len(records), aliasTypeLabel(params.AliasType))}
		for _, record := range records {
			lines = append(lines, formatApprovedAliasRecord(record))
		}
		return []byte(strings.Join(lines, "\n")), nil
	case ModeAdd:
		params, err := decodeAddParams(raw)
		if err != nil {
			return nil, err
		}
		records, err := service.Submit(ctx, params.AliasType, params.Platform, params.PlatformUserID, params.Target, params.Aliases)
		if err != nil {
			return nil, err
		}
		lines := []string{fmt.Sprintf("已提交 %d 条%s别名审核申请，审核ID如下：", len(records), aliasTypeLabel(params.AliasType))}
		for _, record := range records {
			lines = append(lines, formatAliasRecord(record))
		}
		return []byte(strings.Join(lines, "\n")), nil
	case ModeQuery:
		params, err := decodeQueryParams(raw)
		if err != nil {
			return nil, err
		}
		result, err := service.Query(ctx, params.AliasType, params.Target)
		if err != nil {
			return nil, err
		}
		lines := []string{
			fmt.Sprintf("%s: %d", aliasTypeIDLabel(params.AliasType), result.Entity.ID),
			fmt.Sprintf("%s: %s", aliasTypeNameLabel(params.AliasType), result.Entity.Name),
		}
		if len(result.Aliases) == 0 {
			lines = append(lines, "已审核别名: 无")
		} else {
			lines = append(lines, fmt.Sprintf("已审核别名（%d 条）:", len(result.Aliases)))
			lines = append(lines, result.Aliases...)
		}
		return []byte(strings.Join(lines, "\n")), nil
	case ModePendingList:
		params, err := decodeReviewListParams(raw)
		if err != nil {
			return nil, err
		}
		records, err := service.ListPending(ctx, params.Platform, params.PlatformUserID)
		if err != nil {
			return nil, err
		}
		if len(records) == 0 {
			return []byte("当前没有待审核别名"), nil
		}
		lines := []string{fmt.Sprintf("当前共有 %d 条待审核别名：", len(records))}
		for _, record := range records {
			lines = append(lines, formatAliasRecord(record))
		}
		return []byte(strings.Join(lines, "\n")), nil
	case ModeSubmitter:
		params, err := decodeSubmitterParams(raw)
		if err != nil {
			return nil, err
		}
		record, err := service.GetSubmitter(ctx, params.Platform, params.PlatformUserID, params.ReviewID)
		if err != nil {
			return nil, err
		}
		return []byte(fmt.Sprintf("别名提交者：\n%s\n提交者: %s", formatAliasRecord(*record), record.SubmittedBy)), nil
	case ModeBanSubmitter:
		params, err := decodeBanSubmitterParams(raw)
		if err != nil {
			return nil, err
		}
		record, err := service.BanSubmitter(
			ctx,
			params.Platform,
			params.PlatformUserID,
			params.TargetPlatform,
			params.TargetPlatformUserID,
		)
		if err != nil {
			return nil, err
		}
		return []byte(fmt.Sprintf("已禁止用户 %s:%s 提交别名", record.Platform, record.PlatformUserID)), nil
	case ModeApprove:
		params, err := decodeApproveParams(raw)
		if err != nil {
			return nil, err
		}
		records, err := service.Approve(ctx, params.Platform, params.PlatformUserID, params.ReviewIDs)
		if err != nil {
			return nil, err
		}
		lines := []string{fmt.Sprintf("已通过 %d 条别名审核：", len(records))}
		for _, record := range records {
			lines = append(lines, formatAliasRecord(record))
		}
		return []byte(strings.Join(lines, "\n")), nil
	case ModeReject:
		params, err := decodeRejectParams(raw)
		if err != nil {
			return nil, err
		}
		record, err := service.Reject(ctx, params.Platform, params.PlatformUserID, params.ReviewID, params.Reason)
		if err != nil {
			return nil, err
		}
		lines := []string{"已拒绝别名审核：", formatRejectedAliasRecord(*record, params.Reason)}
		return []byte(strings.Join(lines, "\n")), nil
	case ModeBatchReject:
		params, err := decodeBatchRejectParams(raw)
		if err != nil {
			return nil, err
		}
		const reason = "批量拒绝"
		records, err := service.RejectMany(ctx, params.Platform, params.PlatformUserID, params.ReviewIDs, reason)
		if err != nil {
			return nil, err
		}
		lines := []string{fmt.Sprintf("已批量拒绝 %d 条别名审核：", len(records))}
		for _, record := range records {
			lines = append(lines, formatRejectedAliasRecord(record, reason))
		}
		return []byte(strings.Join(lines, "\n")), nil
	default:
		return nil, fmt.Errorf("bridge: unsupported alias mode %q", mode)
	}
}

func decodeDeleteParams(raw json.RawMessage) (DeleteCommandParams, error) {
	var params DeleteCommandParams
	if len(raw) == 0 {
		return params, fmt.Errorf("bridge: missing alias delete params")
	}
	if err := sonic.Unmarshal(raw, &params); err != nil {
		return params, fmt.Errorf("bridge: unmarshal alias delete params: %w", err)
	}
	params.Platform = strings.TrimSpace(params.Platform)
	params.PlatformUserID = strings.TrimSpace(params.PlatformUserID)
	params.Target = strings.TrimSpace(params.Target)
	if _, err := normalizeAliasType(params.AliasType); err != nil {
		return params, err
	}
	if params.Platform == "" || params.PlatformUserID == "" {
		return params, fmt.Errorf("bridge: missing alias delete identity context")
	}
	if params.Target == "" {
		return params, fmt.Errorf("请输入%s", entityTokenPrompt(params.AliasType))
	}
	return params, nil
}

func decodeAddParams(raw json.RawMessage) (AddCommandParams, error) {
	var params AddCommandParams
	if len(raw) == 0 {
		return params, fmt.Errorf("bridge: missing alias add params")
	}
	if err := sonic.Unmarshal(raw, &params); err != nil {
		return params, fmt.Errorf("bridge: unmarshal alias add params: %w", err)
	}
	params.Platform = strings.TrimSpace(params.Platform)
	params.PlatformUserID = strings.TrimSpace(params.PlatformUserID)
	params.Target = strings.TrimSpace(params.Target)
	if _, err := normalizeAliasType(params.AliasType); err != nil {
		return params, err
	}
	if params.Target == "" {
		return params, fmt.Errorf("请输入%s", entityTokenPrompt(params.AliasType))
	}
	return params, nil
}

func decodeQueryParams(raw json.RawMessage) (QueryCommandParams, error) {
	var params QueryCommandParams
	if len(raw) == 0 {
		return params, fmt.Errorf("bridge: missing alias query params")
	}
	if err := sonic.Unmarshal(raw, &params); err != nil {
		return params, fmt.Errorf("bridge: unmarshal alias query params: %w", err)
	}
	params.Target = strings.TrimSpace(params.Target)
	if _, err := normalizeAliasType(params.AliasType); err != nil {
		return params, err
	}
	if params.Target == "" {
		return params, fmt.Errorf("请输入%s", entityTokenPrompt(params.AliasType))
	}
	return params, nil
}

func decodeReviewListParams(raw json.RawMessage) (ReviewListCommandParams, error) {
	var params ReviewListCommandParams
	if len(raw) == 0 {
		return params, fmt.Errorf("bridge: missing alias review list params")
	}
	if err := sonic.Unmarshal(raw, &params); err != nil {
		return params, fmt.Errorf("bridge: unmarshal alias review list params: %w", err)
	}
	params.Platform = strings.TrimSpace(params.Platform)
	params.PlatformUserID = strings.TrimSpace(params.PlatformUserID)
	if params.Platform == "" || params.PlatformUserID == "" {
		return params, fmt.Errorf("bridge: missing alias review identity context")
	}
	return params, nil
}

func decodeSubmitterParams(raw json.RawMessage) (SubmitterCommandParams, error) {
	var params SubmitterCommandParams
	if len(raw) == 0 {
		return params, fmt.Errorf("bridge: missing alias submitter params")
	}
	if err := sonic.Unmarshal(raw, &params); err != nil {
		return params, fmt.Errorf("bridge: unmarshal alias submitter params: %w", err)
	}
	params.Platform = strings.TrimSpace(params.Platform)
	params.PlatformUserID = strings.TrimSpace(params.PlatformUserID)
	if params.Platform == "" || params.PlatformUserID == "" {
		return params, fmt.Errorf("bridge: missing alias submitter identity context")
	}
	if params.ReviewID <= 0 {
		return params, fmt.Errorf("请输入正确的待审核ID")
	}
	return params, nil
}

func decodeBanSubmitterParams(raw json.RawMessage) (BanSubmitterCommandParams, error) {
	var params BanSubmitterCommandParams
	if len(raw) == 0 {
		return params, fmt.Errorf("bridge: missing alias ban submitter params")
	}
	if err := sonic.Unmarshal(raw, &params); err != nil {
		return params, fmt.Errorf("bridge: unmarshal alias ban submitter params: %w", err)
	}
	params.Platform = strings.TrimSpace(params.Platform)
	params.PlatformUserID = strings.TrimSpace(params.PlatformUserID)
	params.TargetPlatform = strings.TrimSpace(params.TargetPlatform)
	params.TargetPlatformUserID = strings.TrimSpace(params.TargetPlatformUserID)
	if params.Platform == "" || params.PlatformUserID == "" {
		return params, fmt.Errorf("bridge: missing alias ban submitter identity context")
	}
	if params.TargetPlatform == "" || params.TargetPlatformUserID == "" {
		return params, fmt.Errorf("请输入要禁用的用户ID")
	}
	return params, nil
}

func decodeApproveParams(raw json.RawMessage) (ApproveCommandParams, error) {
	var params ApproveCommandParams
	if len(raw) == 0 {
		return params, fmt.Errorf("bridge: missing alias approve params")
	}
	if err := sonic.Unmarshal(raw, &params); err != nil {
		return params, fmt.Errorf("bridge: unmarshal alias approve params: %w", err)
	}
	params.Platform = strings.TrimSpace(params.Platform)
	params.PlatformUserID = strings.TrimSpace(params.PlatformUserID)
	if params.Platform == "" || params.PlatformUserID == "" {
		return params, fmt.Errorf("bridge: missing alias approve identity context")
	}
	return params, nil
}

func decodeRejectParams(raw json.RawMessage) (RejectCommandParams, error) {
	var params RejectCommandParams
	if len(raw) == 0 {
		return params, fmt.Errorf("bridge: missing alias reject params")
	}
	if err := sonic.Unmarshal(raw, &params); err != nil {
		return params, fmt.Errorf("bridge: unmarshal alias reject params: %w", err)
	}
	params.Platform = strings.TrimSpace(params.Platform)
	params.PlatformUserID = strings.TrimSpace(params.PlatformUserID)
	params.Reason = strings.TrimSpace(params.Reason)
	if params.Platform == "" || params.PlatformUserID == "" {
		return params, fmt.Errorf("bridge: missing alias reject identity context")
	}
	return params, nil
}

func decodeBatchRejectParams(raw json.RawMessage) (BatchRejectCommandParams, error) {
	var params BatchRejectCommandParams
	if len(raw) == 0 {
		return params, fmt.Errorf("bridge: missing alias batch reject params")
	}
	if err := sonic.Unmarshal(raw, &params); err != nil {
		return params, fmt.Errorf("bridge: unmarshal alias batch reject params: %w", err)
	}
	params.Platform = strings.TrimSpace(params.Platform)
	params.PlatformUserID = strings.TrimSpace(params.PlatformUserID)
	if params.Platform == "" || params.PlatformUserID == "" {
		return params, fmt.Errorf("bridge: missing alias batch reject identity context")
	}
	if len(params.ReviewIDs) == 0 {
		return params, fmt.Errorf("请至少输入一个待审核ID")
	}
	return params, nil
}
