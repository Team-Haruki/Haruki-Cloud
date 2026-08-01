package alias

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	pjskdb "haruki-cloud/database/pjsk"
	"haruki-cloud/database/pjsk/pendingalias"
	"haruki-cloud/internal/onebot11"
)

func (s *Service) ListPending(ctx context.Context, platform, platformUserID string) ([]PjskAliasRecord, error) {
	if !s.IsReady() {
		return nil, fmt.Errorf("别名服务未就绪，请稍后再试")
	}
	if _, _, err := s.requireAdmin(ctx, platform, platformUserID); err != nil {
		return nil, err
	}
	rows, err := s.pjsk.PendingAlias.Query().
		Where(pendingalias.AliasTypeIn(supportedAliasTypes...)).
		Order(pendingalias.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return s.buildAliasRecordsFromPending(ctx, rows)
}

func (s *Service) Approve(ctx context.Context, platform, platformUserID string, reviewIDs []int64) ([]PjskAliasRecord, error) {
	if !s.IsReady() {
		return nil, onebot11.NewReplayError("别名服务未就绪，请稍后再试")
	}
	if err := s.requireWritable(); err != nil {
		return nil, err
	}
	if _, _, err := s.requireAdmin(ctx, platform, platformUserID); err != nil {
		return nil, err
	}
	uniqueIDs, err := normalizeReviewIDs(reviewIDs)
	if err != nil {
		return nil, err
	}

	tx, err := s.pjsk.Tx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	rows, err := tx.PendingAlias.Query().
		Where(
			pendingalias.AliasTypeIn(supportedAliasTypes...),
			pendingalias.IDIn(uniqueIDs...),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}

	byID := make(map[int64]*pjskdb.PendingAlias, len(rows))
	for _, row := range rows {
		byID[row.ID] = row
	}
	if len(byID) != len(uniqueIDs) {
		missing := make([]string, 0)
		for _, reviewID := range uniqueIDs {
			if _, ok := byID[reviewID]; !ok {
				missing = append(missing, strconv.FormatInt(reviewID, 10))
			}
		}
		return nil, onebot11.NewReplayError("未找到待审核别名ID: %s", strings.Join(missing, " "))
	}

	reserved := make(map[string]int64, len(uniqueIDs))
	for _, reviewID := range uniqueIDs {
		row := byID[reviewID]
		if err := s.ensureEntityNameAvailable(ctx, row.AliasType, row.Alias); err != nil {
			return nil, err
		}
		exists, err := approvedAliasExists(ctx, tx.Alias, row.AliasType, row.Alias)
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, onebot11.NewReplayError("%s别名 %q 已经存在于已审核列表中", aliasTypeLabel(row.AliasType), row.Alias)
		}
		key := row.AliasType + "\x00" + normalizeCompareText(row.Alias)
		if prevID, ok := reserved[key]; ok {
			return nil, onebot11.NewReplayError("待通过的审核ID %d 与 %d 使用了重复%s别名 %q", prevID, reviewID, aliasTypeLabel(row.AliasType), row.Alias)
		}
		reserved[key] = reviewID
	}

	for _, reviewID := range uniqueIDs {
		row := byID[reviewID]
		if _, err := tx.Alias.Create().
			SetAliasType(row.AliasType).
			SetAliasTypeID(row.AliasTypeID).
			SetAlias(row.Alias).
			Save(ctx); err != nil {
			if pjskdb.IsConstraintError(err) {
				return nil, onebot11.NewReplayError("%s别名 %q 已经存在于已审核列表中", aliasTypeLabel(row.AliasType), row.Alias)
			}
			return nil, err
		}
		if err := tx.PendingAlias.DeleteOneID(row.ID).Exec(ctx); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	tx = nil
	return s.buildAliasRecordsFromPending(ctx, orderedPendingAliases(uniqueIDs, byID))
}

func (s *Service) Reject(ctx context.Context, platform, platformUserID string, reviewID int64, reason string) (*PjskAliasRecord, error) {
	records, err := s.RejectMany(ctx, platform, platformUserID, []int64{reviewID}, reason)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("未找到待审核别名ID: %d", reviewID)
	}
	return &records[0], nil
}

// RejectMany rejects all requested pending aliases in one transaction.
func (s *Service) RejectMany(ctx context.Context, platform, platformUserID string, reviewIDs []int64, reason string) ([]PjskAliasRecord, error) {
	if !s.IsReady() {
		return nil, onebot11.NewReplayError("别名服务未就绪，请稍后再试")
	}
	if err := s.requireWritable(); err != nil {
		return nil, err
	}
	admin, reviewer, err := s.requireAdmin(ctx, platform, platformUserID)
	if err != nil {
		return nil, err
	}
	uniqueIDs, err := normalizeReviewIDs(reviewIDs)
	if err != nil {
		return nil, err
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, onebot11.NewReplayError("请输入拒绝原因")
	}
	if strings.TrimSpace(admin.Name) != "" {
		reviewer = strings.TrimSpace(admin.Name)
	}

	tx, err := s.pjsk.Tx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	rows, err := tx.PendingAlias.Query().
		Where(
			pendingalias.AliasTypeIn(supportedAliasTypes...),
			pendingalias.IDIn(uniqueIDs...),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}

	byID := make(map[int64]*pjskdb.PendingAlias, len(rows))
	for _, row := range rows {
		byID[row.ID] = row
	}
	if len(byID) != len(uniqueIDs) {
		missing := make([]string, 0)
		for _, reviewID := range uniqueIDs {
			if _, ok := byID[reviewID]; !ok {
				missing = append(missing, strconv.FormatInt(reviewID, 10))
			}
		}
		return nil, onebot11.NewReplayError("未找到待审核别名ID: %s", strings.Join(missing, " "))
	}

	reviewedAt := time.Now()
	for _, reviewID := range uniqueIDs {
		row := byID[reviewID]
		if _, err := tx.RejectedAlias.Create().
			SetAliasType(row.AliasType).
			SetAliasTypeID(row.AliasTypeID).
			SetAlias(row.Alias).
			SetReviewedBy(reviewer).
			SetReason(reason).
			SetReviewedAt(reviewedAt).
			Save(ctx); err != nil {
			return nil, err
		}
		if err := tx.PendingAlias.DeleteOneID(reviewID).Exec(ctx); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	tx = nil

	records, err := s.buildAliasRecordsFromPending(ctx, orderedPendingAliases(uniqueIDs, byID))
	if err != nil {
		return nil, err
	}
	return records, nil
}
