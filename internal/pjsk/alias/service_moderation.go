package alias

import (
	"context"
	"fmt"
	"strings"
	"time"

	pjskdb "haruki-cloud/database/pjsk"
	"haruki-cloud/database/pjsk/aliassubmissionban"
	"haruki-cloud/database/pjsk/pendingalias"
	"haruki-cloud/internal/onebot11"
)

func (s *Service) GetSubmitter(ctx context.Context, platform, platformUserID string, reviewID int64) (*PjskAliasRecord, error) {
	if !s.IsReady() {
		return nil, onebot11.NewReplayError("别名服务未就绪，请稍后再试")
	}
	if _, _, err := s.requireAdmin(ctx, platform, platformUserID); err != nil {
		return nil, err
	}
	if reviewID <= 0 {
		return nil, onebot11.NewReplayError("请输入正确的待审核ID")
	}

	row, err := s.pjsk.PendingAlias.Query().
		Where(
			pendingalias.AliasTypeIn(supportedAliasTypes...),
			pendingalias.IDEQ(reviewID),
		).
		Only(ctx)
	if err != nil {
		if pjskdb.IsNotFound(err) {
			return nil, onebot11.NewReplayError("未找到待审核别名ID: %d", reviewID)
		}
		return nil, err
	}
	records, err := s.buildAliasRecordsFromPending(ctx, []*pjskdb.PendingAlias{row})
	if err != nil {
		return nil, err
	}
	if len(records) != 1 {
		return nil, fmt.Errorf("未找到待审核别名ID: %d", reviewID)
	}
	return &records[0], nil
}

func (s *Service) BanSubmitter(ctx context.Context, platform, platformUserID, targetPlatform, targetPlatformUserID string) (*SubmissionBanRecord, error) {
	if !s.IsReady() {
		return nil, onebot11.NewReplayError("别名服务未就绪，请稍后再试")
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
		return nil, onebot11.NewReplayError("请输入要禁用的用户ID")
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
