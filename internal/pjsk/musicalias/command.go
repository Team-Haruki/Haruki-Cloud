package musicalias

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	ModeDelete      = "music-alias-delete"
	ModeAdd         = "music-alias-add"
	ModeQuery       = "music-alias-query"
	ModePendingList = "music-alias-pending-list"
	ModeApprove     = "music-alias-approve"
	ModeReject      = "music-alias-reject"
)

type AddCommandParams struct {
	Platform       string   `json:"platform"`
	PlatformUserID string   `json:"platform_user_id"`
	Target         string   `json:"target"`
	Aliases        []string `json:"aliases"`
}

type DeleteCommandParams struct {
	Platform       string   `json:"platform"`
	PlatformUserID string   `json:"platform_user_id"`
	Target         string   `json:"target"`
	Aliases        []string `json:"aliases"`
}

type QueryCommandParams struct {
	Target string `json:"target"`
}

type ReviewListCommandParams struct {
	Platform       string `json:"platform"`
	PlatformUserID string `json:"platform_user_id"`
}

type ApproveCommandParams struct {
	Platform       string  `json:"platform"`
	PlatformUserID string  `json:"platform_user_id"`
	ReviewIDs      []int64 `json:"review_ids"`
}

type RejectCommandParams struct {
	Platform       string `json:"platform"`
	PlatformUserID string `json:"platform_user_id"`
	ReviewID       int64  `json:"review_id"`
	Reason         string `json:"reason"`
}

func ExecuteCommand(ctx context.Context, service *Service, mode string, raw json.RawMessage) ([]byte, error) {
	switch mode {
	case ModeDelete:
		params, err := decodeDeleteParams(raw)
		if err != nil {
			return nil, err
		}
		records, err := service.Delete(ctx, params.Platform, params.PlatformUserID, params.Target, params.Aliases)
		if err != nil {
			return nil, err
		}
		lines := []string{fmt.Sprintf("已删除 %d 条歌曲已审核别名：", len(records))}
		for _, record := range records {
			lines = append(lines, formatApprovedAliasRecord(record))
		}
		return []byte(strings.Join(lines, "\n")), nil
	case ModeAdd:
		params, err := decodeAddParams(raw)
		if err != nil {
			return nil, err
		}
		records, err := service.Submit(ctx, params.Platform, params.PlatformUserID, params.Target, params.Aliases)
		if err != nil {
			return nil, err
		}
		lines := []string{fmt.Sprintf("已提交 %d 条歌曲别名审核申请，审核ID如下：", len(records))}
		for _, record := range records {
			lines = append(lines, formatAliasRecord(record))
		}
		return []byte(strings.Join(lines, "\n")), nil
	case ModeQuery:
		params, err := decodeQueryParams(raw)
		if err != nil {
			return nil, err
		}
		result, err := service.Query(ctx, params.Target)
		if err != nil {
			return nil, err
		}
		lines := []string{
			fmt.Sprintf("歌曲ID: %d", result.Music.ID),
			fmt.Sprintf("曲名: %s", result.Music.Title),
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
			return []byte("当前没有待审核的歌曲别名"), nil
		}
		lines := []string{fmt.Sprintf("当前共有 %d 条待审核歌曲别名：", len(records))}
		for _, record := range records {
			lines = append(lines, formatAliasRecord(record))
		}
		return []byte(strings.Join(lines, "\n")), nil
	case ModeApprove:
		params, err := decodeApproveParams(raw)
		if err != nil {
			return nil, err
		}
		records, err := service.Approve(ctx, params.Platform, params.PlatformUserID, params.ReviewIDs)
		if err != nil {
			return nil, err
		}
		lines := []string{fmt.Sprintf("已通过 %d 条歌曲别名审核：", len(records))}
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
		lines := []string{"已拒绝歌曲别名审核：", formatRejectedAliasRecord(*record, params.Reason)}
		return []byte(strings.Join(lines, "\n")), nil
	default:
		return nil, fmt.Errorf("bridge: unsupported music alias mode %q", mode)
	}
}

func decodeAddParams(raw json.RawMessage) (AddCommandParams, error) {
	var params AddCommandParams
	if len(raw) == 0 {
		return params, fmt.Errorf("bridge: missing music alias add params")
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return params, fmt.Errorf("bridge: unmarshal music alias add params: %w", err)
	}
	params.Platform = strings.TrimSpace(params.Platform)
	params.PlatformUserID = strings.TrimSpace(params.PlatformUserID)
	params.Target = strings.TrimSpace(params.Target)
	if params.Target == "" {
		return params, fmt.Errorf("请输入歌曲ID、曲名或已审核别名")
	}
	return params, nil
}

func decodeDeleteParams(raw json.RawMessage) (DeleteCommandParams, error) {
	var params DeleteCommandParams
	if len(raw) == 0 {
		return params, fmt.Errorf("bridge: missing music alias delete params")
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return params, fmt.Errorf("bridge: unmarshal music alias delete params: %w", err)
	}
	params.Platform = strings.TrimSpace(params.Platform)
	params.PlatformUserID = strings.TrimSpace(params.PlatformUserID)
	params.Target = strings.TrimSpace(params.Target)
	if params.Platform == "" || params.PlatformUserID == "" {
		return params, fmt.Errorf("bridge: missing music alias delete identity context")
	}
	if params.Target == "" {
		return params, fmt.Errorf("请输入歌曲ID、曲名或已审核别名")
	}
	return params, nil
}

func decodeQueryParams(raw json.RawMessage) (QueryCommandParams, error) {
	var params QueryCommandParams
	if len(raw) == 0 {
		return params, fmt.Errorf("bridge: missing music alias query params")
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return params, fmt.Errorf("bridge: unmarshal music alias query params: %w", err)
	}
	params.Target = strings.TrimSpace(params.Target)
	if params.Target == "" {
		return params, fmt.Errorf("请输入歌曲ID、曲名或已审核别名")
	}
	return params, nil
}

func decodeReviewListParams(raw json.RawMessage) (ReviewListCommandParams, error) {
	var params ReviewListCommandParams
	if len(raw) == 0 {
		return params, fmt.Errorf("bridge: missing music alias review list params")
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return params, fmt.Errorf("bridge: unmarshal music alias review list params: %w", err)
	}
	params.Platform = strings.TrimSpace(params.Platform)
	params.PlatformUserID = strings.TrimSpace(params.PlatformUserID)
	if params.Platform == "" || params.PlatformUserID == "" {
		return params, fmt.Errorf("bridge: missing music alias review identity context")
	}
	return params, nil
}

func decodeApproveParams(raw json.RawMessage) (ApproveCommandParams, error) {
	var params ApproveCommandParams
	if len(raw) == 0 {
		return params, fmt.Errorf("bridge: missing music alias approve params")
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return params, fmt.Errorf("bridge: unmarshal music alias approve params: %w", err)
	}
	params.Platform = strings.TrimSpace(params.Platform)
	params.PlatformUserID = strings.TrimSpace(params.PlatformUserID)
	if params.Platform == "" || params.PlatformUserID == "" {
		return params, fmt.Errorf("bridge: missing music alias approve identity context")
	}
	return params, nil
}

func decodeRejectParams(raw json.RawMessage) (RejectCommandParams, error) {
	var params RejectCommandParams
	if len(raw) == 0 {
		return params, fmt.Errorf("bridge: missing music alias reject params")
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return params, fmt.Errorf("bridge: unmarshal music alias reject params: %w", err)
	}
	params.Platform = strings.TrimSpace(params.Platform)
	params.PlatformUserID = strings.TrimSpace(params.PlatformUserID)
	params.Reason = strings.TrimSpace(params.Reason)
	if params.Platform == "" || params.PlatformUserID == "" {
		return params, fmt.Errorf("bridge: missing music alias reject identity context")
	}
	return params, nil
}
