package alias

import (
	"context"
	"strconv"
	"strings"
	"time"

	pjskdb "haruki-cloud/database/pjsk"
	"haruki-cloud/database/pjsk/pendingalias"
	"haruki-cloud/internal/i18n"
	"haruki-cloud/utils/usererror"
)

func (s *Service) ListPending(ctx context.Context, platform, platformUserID string) ([]PjskAliasRecord, error) {
	if !s.IsReady() {
		return nil, errAliasUnavailable()
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
		return nil, errAliasUnavailable()
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

	byID, err := loadPendingAliasesForReview(ctx, tx, uniqueIDs)
	if err != nil {
		return nil, err
	}
	if err := s.validatePendingAliasesForApproval(ctx, tx, uniqueIDs, byID); err != nil {
		return nil, err
	}
	if err := approvePendingAliases(ctx, tx, uniqueIDs, byID); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	tx = nil
	approved := orderedPendingAliases(uniqueIDs, byID)
	changes := make([]AliasChange, 0, len(approved))
	for _, row := range approved {
		changes = append(changes, AliasChange{AliasType: row.AliasType, AliasTypeID: row.AliasTypeID, Alias: row.Alias})
	}
	s.notifyChanged(ctx, changes)
	return s.buildAliasRecordsFromPending(ctx, approved)
}

func loadPendingAliasesForReview(ctx context.Context, tx *pjskdb.Tx, reviewIDs []int64) (map[int64]*pjskdb.PendingAlias, error) {
	rows, err := tx.PendingAlias.Query().
		Where(
			pendingalias.AliasTypeIn(supportedAliasTypes...),
			pendingalias.IDIn(reviewIDs...),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]*pjskdb.PendingAlias, len(rows))
	for _, row := range rows {
		byID[row.ID] = row
	}
	missing := missingReviewIDs(reviewIDs, byID)
	if len(missing) != 0 {
		return nil, usererror.New(usererror.CodeNotFound, i18n.M("alias.review_not_found", i18n.Data{"UserIDs": i18n.UserText(strings.Join(missing, "、"))}))
	}
	return byID, nil
}

func missingReviewIDs(reviewIDs []int64, byID map[int64]*pjskdb.PendingAlias) []string {
	missing := make([]string, 0)
	for _, reviewID := range reviewIDs {
		if _, ok := byID[reviewID]; !ok {
			missing = append(missing, strconv.FormatInt(reviewID, 10))
		}
	}
	return missing
}

func (s *Service) validatePendingAliasesForApproval(ctx context.Context, tx *pjskdb.Tx, reviewIDs []int64, byID map[int64]*pjskdb.PendingAlias) error {
	reserved := make(map[string]int64, len(reviewIDs))
	for _, reviewID := range reviewIDs {
		row := byID[reviewID]
		if err := s.ensureEntityNameAvailable(ctx, row.AliasType, row.Alias); err != nil {
			return err
		}
		exists, err := approvedAliasExists(ctx, tx.Alias, row.AliasType, row.Alias)
		if err != nil {
			return err
		}
		if exists {
			return usererror.Invalid(i18n.M("alias.already_approved", i18n.Data{"Kind": aliasKind(row.AliasType), "UserAlias": i18n.EchoQuery(row.Alias)}))
		}
		key := row.AliasType + "\x00" + normalizeCompareText(row.Alias)
		if prevID, ok := reserved[key]; ok {
			return usererror.Invalid(i18n.M("alias.duplicate_in_batch", i18n.Data{"UserFirst": i18n.UserNumber(prevID), "UserSecond": i18n.UserNumber(reviewID), "Kind": aliasKind(row.AliasType), "UserAlias": i18n.EchoQuery(row.Alias)}))
		}
		reserved[key] = reviewID
	}
	return nil
}

func approvePendingAliases(ctx context.Context, tx *pjskdb.Tx, reviewIDs []int64, byID map[int64]*pjskdb.PendingAlias) error {
	for _, reviewID := range reviewIDs {
		row := byID[reviewID]
		_, err := tx.Alias.Create().
			SetAliasType(row.AliasType).
			SetAliasTypeID(row.AliasTypeID).
			SetAlias(row.Alias).
			Save(ctx)
		if pjskdb.IsConstraintError(err) {
			return usererror.Invalid(i18n.M("alias.already_approved", i18n.Data{"Kind": aliasKind(row.AliasType), "UserAlias": i18n.EchoQuery(row.Alias)}))
		}
		if err != nil {
			return err
		}
		if err := tx.PendingAlias.DeleteOneID(row.ID).Exec(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) Reject(ctx context.Context, platform, platformUserID string, reviewID int64, reason string) (*PjskAliasRecord, error) {
	records, err := s.RejectMany(ctx, platform, platformUserID, []int64{reviewID}, reason)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, usererror.New(usererror.CodeNotFound, i18n.M("alias.review_not_found", i18n.Data{"UserIDs": i18n.UserNumber(reviewID)}))
	}
	return &records[0], nil
}

// RejectMany rejects all requested pending aliases in one transaction.
func (s *Service) RejectMany(ctx context.Context, platform, platformUserID string, reviewIDs []int64, reason string) ([]PjskAliasRecord, error) {
	if !s.IsReady() {
		return nil, errAliasUnavailable()
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
		return nil, usererror.Misuse(i18n.M("alias.reject_reason_required"))
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

	byID, err := loadPendingAliasesForReview(ctx, tx, uniqueIDs)
	if err != nil {
		return nil, err
	}
	if err := rejectPendingAliases(ctx, tx, uniqueIDs, byID, reviewer, reason); err != nil {
		return nil, err
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

func rejectPendingAliases(ctx context.Context, tx *pjskdb.Tx, reviewIDs []int64, byID map[int64]*pjskdb.PendingAlias, reviewer, reason string) error {
	reviewedAt := time.Now()
	for _, reviewID := range reviewIDs {
		row := byID[reviewID]
		if _, err := tx.RejectedAlias.Create().
			SetAliasType(row.AliasType).
			SetAliasTypeID(row.AliasTypeID).
			SetAlias(row.Alias).
			SetReviewedBy(reviewer).
			SetReason(reason).
			SetReviewedAt(reviewedAt).
			Save(ctx); err != nil {
			return err
		}
		if err := tx.PendingAlias.DeleteOneID(reviewID).Exec(ctx); err != nil {
			return err
		}
	}
	return nil
}
