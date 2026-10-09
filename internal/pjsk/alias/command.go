package alias

import (
	"context"
	"fmt"
	"strings"

	"haruki-cloud/internal/i18n"
	json "haruki-cloud/internal/jsonutil"
	"haruki-cloud/utils/usererror"
)

// ExecuteCommand runs an alias command and returns its reply. The reply is a
// catalog message, not rendered text: replies that repeat alias text nobody
// has reviewed yet (submissions, the pending list, submitter lookups,
// rejections) show it only when the bot client enabled parameter echo, and
// the delivering layer renders the message for its own client.
func ExecuteCommand(ctx context.Context, service *Service, mode string, raw json.RawMessage) (i18n.Message, error) {
	switch mode {
	case ModeDelete:
		return executeDeleteCommand(ctx, service, raw)
	case ModeAdd:
		return executeAddCommand(ctx, service, raw)
	case ModeQuery:
		return executeQueryCommand(ctx, service, raw)
	case ModePendingList:
		return executePendingListCommand(ctx, service, raw)
	case ModeSubmitter:
		return executeSubmitterCommand(ctx, service, raw)
	case ModeBanSubmitter:
		return executeBanSubmitterCommand(ctx, service, raw)
	case ModeApprove:
		return executeApproveCommand(ctx, service, raw)
	case ModeReject:
		return executeRejectCommand(ctx, service, raw)
	case ModeBatchReject:
		return executeBatchRejectCommand(ctx, service, raw)
	default:
		return i18n.Message{}, fmt.Errorf("bridge: unsupported alias mode %q", mode)
	}
}

func executeDeleteCommand(ctx context.Context, service *Service, raw json.RawMessage) (i18n.Message, error) {
	params, err := decodeDeleteParams(raw)
	if err != nil {
		return i18n.Message{}, err
	}
	records, err := service.Delete(ctx, params.AliasType, params.Platform, params.PlatformUserID, params.Target, params.Aliases)
	if err != nil {
		return i18n.Message{}, err
	}
	lines := make([]i18n.Message, 0, len(records))
	for _, record := range records {
		lines = append(lines, approvedAliasRecordMessage(record))
	}
	return i18n.M("alias.delete.done", i18n.Data{"Count": len(records), "Kind": aliasKind(params.AliasType), "Records": lines}), nil
}

func executeAddCommand(ctx context.Context, service *Service, raw json.RawMessage) (i18n.Message, error) {
	params, err := decodeAddParams(raw)
	if err != nil {
		return i18n.Message{}, err
	}
	records, err := service.Submit(ctx, params.AliasType, params.Platform, params.PlatformUserID, params.Target, params.Aliases)
	if err != nil {
		return i18n.Message{}, err
	}
	return i18n.M("alias.add.done", i18n.Data{"Count": len(records), "Kind": aliasKind(params.AliasType), "Records": aliasRecordMessages(records)}), nil
}

func executeQueryCommand(ctx context.Context, service *Service, raw json.RawMessage) (i18n.Message, error) {
	params, err := decodeQueryParams(raw)
	if err != nil {
		return i18n.Message{}, err
	}
	result, err := service.Query(ctx, params.AliasType, params.Target)
	if err != nil {
		return i18n.Message{}, err
	}
	entity := i18n.Data{"ID": result.Entity.ID, "Name": result.Entity.Name}
	header := i18n.M("alias.query.music", entity)
	if params.AliasType == PjskAliasTypeCharacter {
		header = i18n.M("alias.query.character", entity)
	}
	list := i18n.M("alias.query.none")
	if len(result.Aliases) > 0 {
		list = i18n.M("alias.query.list", i18n.Data{"Count": len(result.Aliases), "Aliases": strings.Join(result.Aliases, "\n")})
	}
	return i18n.M("alias.query.result", i18n.Data{"Header": header, "List": list}), nil
}

func executePendingListCommand(ctx context.Context, service *Service, raw json.RawMessage) (i18n.Message, error) {
	params, err := decodeReviewListParams(raw)
	if err != nil {
		return i18n.Message{}, err
	}
	records, err := service.ListPending(ctx, params.Platform, params.PlatformUserID)
	if err != nil {
		return i18n.Message{}, err
	}
	if len(records) == 0 {
		return i18n.M("alias.pending.none"), nil
	}
	return i18n.M("alias.pending.list", i18n.Data{"Count": len(records), "Records": aliasRecordMessages(records)}), nil
}

func executeSubmitterCommand(ctx context.Context, service *Service, raw json.RawMessage) (i18n.Message, error) {
	params, err := decodeSubmitterParams(raw)
	if err != nil {
		return i18n.Message{}, err
	}
	record, err := service.GetSubmitter(ctx, params.Platform, params.PlatformUserID, params.ReviewID)
	if err != nil {
		return i18n.Message{}, err
	}
	return i18n.M("alias.submitter.result", i18n.Data{"Record": aliasRecordMessage(*record), "Submitter": record.SubmittedBy}), nil
}

func executeBanSubmitterCommand(ctx context.Context, service *Service, raw json.RawMessage) (i18n.Message, error) {
	params, err := decodeBanSubmitterParams(raw)
	if err != nil {
		return i18n.Message{}, err
	}
	record, err := service.BanSubmitter(
		ctx,
		params.Platform,
		params.PlatformUserID,
		params.TargetPlatform,
		params.TargetPlatformUserID,
	)
	if err != nil {
		return i18n.Message{}, err
	}
	return i18n.M("alias.ban.done", i18n.Data{"User": record.Platform + ":" + record.PlatformUserID}), nil
}

func executeApproveCommand(ctx context.Context, service *Service, raw json.RawMessage) (i18n.Message, error) {
	params, err := decodeApproveParams(raw)
	if err != nil {
		return i18n.Message{}, err
	}
	records, err := service.Approve(ctx, params.Platform, params.PlatformUserID, params.ReviewIDs)
	if err != nil {
		return i18n.Message{}, err
	}
	return i18n.M("alias.approve.done", i18n.Data{"Count": len(records), "Records": passedAliasRecordMessages(records)}), nil
}

func executeRejectCommand(ctx context.Context, service *Service, raw json.RawMessage) (i18n.Message, error) {
	params, err := decodeRejectParams(raw)
	if err != nil {
		return i18n.Message{}, err
	}
	record, err := service.Reject(ctx, params.Platform, params.PlatformUserID, params.ReviewID, params.Reason)
	if err != nil {
		return i18n.Message{}, err
	}
	return i18n.M("alias.reject.done", i18n.Data{"Record": rejectedAliasRecordMessage(*record, params.Reason)}), nil
}

func executeBatchRejectCommand(ctx context.Context, service *Service, raw json.RawMessage) (i18n.Message, error) {
	params, err := decodeBatchRejectParams(raw)
	if err != nil {
		return i18n.Message{}, err
	}
	reason := i18n.T("alias.batch_reject.reason")
	records, err := service.RejectMany(ctx, params.Platform, params.PlatformUserID, params.ReviewIDs, reason)
	if err != nil {
		return i18n.Message{}, err
	}
	lines := make([]i18n.Message, 0, len(records))
	for _, record := range records {
		lines = append(lines, rejectedAliasRecordMessage(record, reason))
	}
	return i18n.M("alias.batch_reject.done", i18n.Data{"Count": len(records), "Records": lines}), nil
}

func decodeDeleteParams(raw json.RawMessage) (DeleteCommandParams, error) {
	var params DeleteCommandParams
	if len(raw) == 0 {
		return params, fmt.Errorf("bridge: missing alias delete params")
	}
	if err := json.Unmarshal(raw, &params); err != nil {
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
		return params, aliasTargetRequired(params.AliasType)
	}
	return params, nil
}

func decodeAddParams(raw json.RawMessage) (AddCommandParams, error) {
	var params AddCommandParams
	if len(raw) == 0 {
		return params, fmt.Errorf("bridge: missing alias add params")
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return params, fmt.Errorf("bridge: unmarshal alias add params: %w", err)
	}
	params.Platform = strings.TrimSpace(params.Platform)
	params.PlatformUserID = strings.TrimSpace(params.PlatformUserID)
	params.Target = strings.TrimSpace(params.Target)
	if _, err := normalizeAliasType(params.AliasType); err != nil {
		return params, err
	}
	if params.Target == "" {
		return params, aliasTargetRequired(params.AliasType)
	}
	return params, nil
}

func decodeQueryParams(raw json.RawMessage) (QueryCommandParams, error) {
	var params QueryCommandParams
	if len(raw) == 0 {
		return params, fmt.Errorf("bridge: missing alias query params")
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return params, fmt.Errorf("bridge: unmarshal alias query params: %w", err)
	}
	params.Target = strings.TrimSpace(params.Target)
	if _, err := normalizeAliasType(params.AliasType); err != nil {
		return params, err
	}
	if params.Target == "" {
		return params, aliasTargetRequired(params.AliasType)
	}
	return params, nil
}

func decodeReviewListParams(raw json.RawMessage) (ReviewListCommandParams, error) {
	var params ReviewListCommandParams
	if len(raw) == 0 {
		return params, fmt.Errorf("bridge: missing alias review list params")
	}
	if err := json.Unmarshal(raw, &params); err != nil {
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
	if err := json.Unmarshal(raw, &params); err != nil {
		return params, fmt.Errorf("bridge: unmarshal alias submitter params: %w", err)
	}
	params.Platform = strings.TrimSpace(params.Platform)
	params.PlatformUserID = strings.TrimSpace(params.PlatformUserID)
	if params.Platform == "" || params.PlatformUserID == "" {
		return params, fmt.Errorf("bridge: missing alias submitter identity context")
	}
	if params.ReviewID <= 0 {
		return params, usererror.Invalid(i18n.M("alias.review_id_positive"))
	}
	return params, nil
}

func decodeBanSubmitterParams(raw json.RawMessage) (BanSubmitterCommandParams, error) {
	var params BanSubmitterCommandParams
	if len(raw) == 0 {
		return params, fmt.Errorf("bridge: missing alias ban submitter params")
	}
	if err := json.Unmarshal(raw, &params); err != nil {
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
		return params, usererror.Misuse(i18n.M("alias.ban_target_required"))
	}
	return params, nil
}

func decodeApproveParams(raw json.RawMessage) (ApproveCommandParams, error) {
	var params ApproveCommandParams
	if len(raw) == 0 {
		return params, fmt.Errorf("bridge: missing alias approve params")
	}
	if err := json.Unmarshal(raw, &params); err != nil {
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
	if err := json.Unmarshal(raw, &params); err != nil {
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
	if err := json.Unmarshal(raw, &params); err != nil {
		return params, fmt.Errorf("bridge: unmarshal alias batch reject params: %w", err)
	}
	params.Platform = strings.TrimSpace(params.Platform)
	params.PlatformUserID = strings.TrimSpace(params.PlatformUserID)
	if params.Platform == "" || params.PlatformUserID == "" {
		return params, fmt.Errorf("bridge: missing alias batch reject identity context")
	}
	if len(params.ReviewIDs) == 0 {
		return params, usererror.Misuse(i18n.M("alias.review_ids_required"))
	}
	return params, nil
}

// aliasTargetRequired is the reply when an alias command names no song or
// character.
func aliasTargetRequired(aliasType string) error {
	if aliasType == PjskAliasTypeCharacter {
		return usererror.Misuse(i18n.M("alias.target_required.character"))
	}
	return usererror.Misuse(i18n.M("alias.target_required.music"))
}
