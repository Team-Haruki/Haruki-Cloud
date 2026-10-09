package alias

import (
	"context"
	"strconv"
	"strings"
	"time"

	pjskdb "haruki-cloud/database/pjsk"
	"haruki-cloud/database/pjsk/aliassubmissionban"
	"haruki-cloud/database/pjsk/pendingalias"
	"haruki-cloud/internal/i18n"
	"haruki-cloud/utils/usererror"
)

func (s *Service) GetSubmitter(ctx context.Context, platform, platformUserID string, reviewID int64) (*PjskAliasRecord, error) {
	if !s.IsReady() {
		return nil, errAliasUnavailable()
	}
	if _, _, err := s.requireAdmin(ctx, platform, platformUserID); err != nil {
		return nil, err
	}
	if reviewID <= 0 {
		return nil, usererror.Invalid(i18n.M("alias.review_id_positive"))
	}

	row, err := s.pjsk.PendingAlias.Query().
		Where(
			pendingalias.AliasTypeIn(supportedAliasTypes...),
			pendingalias.IDEQ(reviewID),
		).
		Only(ctx)
	if err != nil {
		if pjskdb.IsNotFound(err) {
			return nil, usererror.New(usererror.CodeNotFound, i18n.M("alias.review_not_found", i18n.Data{"IDs": strconv.FormatInt(reviewID, 10)}))
		}
		return nil, err
	}
	records, err := s.buildAliasRecordsFromPending(ctx, []*pjskdb.PendingAlias{row})
	if err != nil {
		return nil, err
	}
	if len(records) != 1 {
		return nil, usererror.New(usererror.CodeNotFound, i18n.M("alias.review_not_found", i18n.Data{"IDs": strconv.FormatInt(reviewID, 10)}))
	}
	return &records[0], nil
}

func (s *Service) BanSubmitter(ctx context.Context, platform, platformUserID, targetPlatform, targetPlatformUserID string) (*SubmissionBanRecord, error) {
	if !s.IsReady() {
		return nil, errAliasUnavailable()
	}
	if err := s.requireWritable(); err != nil {
		return nil, err
	}
	admin, bannedBy, err := s.requireAdmin(ctx, platform, platformUserID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(admin.Name) != "" {
		bannedBy = strings.TrimSpace(admin.Name)
	}
	targetPlatform = strings.TrimSpace(targetPlatform)
	targetPlatformUserID = strings.TrimSpace(targetPlatformUserID)
	if targetPlatform == "" || targetPlatformUserID == "" {
		return nil, usererror.Misuse(i18n.M("alias.ban_target_required"))
	}

	row, err := s.pjsk.AliasSubmissionBan.Query().
		Where(
			aliassubmissionban.PlatformEQ(targetPlatform),
			aliassubmissionban.PlatformUserIDEQ(targetPlatformUserID),
		).
		Only(ctx)
	if err == nil {
		row, err = row.Update().
			SetBannedBy(bannedBy).
			SetBannedAt(time.Now()).
			Save(ctx)
	} else if pjskdb.IsNotFound(err) {
		row, err = s.pjsk.AliasSubmissionBan.Create().
			SetPlatform(targetPlatform).
			SetPlatformUserID(targetPlatformUserID).
			SetBannedBy(bannedBy).
			SetBannedAt(time.Now()).
			Save(ctx)
	}
	if err != nil {
		return nil, err
	}
	return &SubmissionBanRecord{
		Platform:       row.Platform,
		PlatformUserID: row.PlatformUserID,
		BannedBy:       row.BannedBy,
	}, nil
}
