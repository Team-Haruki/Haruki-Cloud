package alias

import (
	"context"
	"fmt"
	"strings"

	pjskdb "haruki-cloud/database/pjsk"
	aliasdb "haruki-cloud/database/pjsk/alias"
	"haruki-cloud/database/pjsk/aliasadmin"
	"haruki-cloud/database/pjsk/pendingalias"
	"haruki-cloud/database/sekai/gamecharacter"
	sekaimusic "haruki-cloud/database/sekai/music"
	"haruki-cloud/internal/pjsk/onebot11"
)

func (s *Service) ensureAliasAvailable(ctx context.Context, aliasType string, approved *pjskdb.AliasClient, pending *pjskdb.PendingAliasClient, aliasText string) error {
	if err := s.ensureEntityNameAvailable(ctx, aliasType, aliasText); err != nil {
		return err
	}
	exists, err := approvedAliasExists(ctx, approved, aliasType, aliasText)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("%s别名 %q 已经存在于已审核列表中", aliasTypeLabel(aliasType), aliasText)
	}
	pendingExists, err := pendingAliasExists(ctx, pending, aliasType, aliasText)
	if err != nil {
		return err
	}
	if pendingExists {
		return fmt.Errorf("%s别名 %q 已经在待审核列表中", aliasTypeLabel(aliasType), aliasText)
	}
	return nil
}

func (s *Service) ensureEntityNameAvailable(ctx context.Context, aliasType, aliasText string) error {
	switch aliasType {
	case PjskAliasTypeMusic:
		conflicts, err := s.sekai.Music.Query().
			Where(sekaimusic.TitleEqualFold(aliasText)).
			Count(ctx)
		if err != nil {
			return err
		}
		if conflicts > 0 {
			return fmt.Errorf("%s别名 %q 与已有%s重复", aliasTypeLabel(aliasType), aliasText, aliasTypeNameLabel(aliasType))
		}
		return nil
	case PjskAliasTypeCharacter:
		rows, err := s.sekai.Gamecharacter.Query().
			Where(gamecharacter.GameIDGT(0)).
			All(ctx)
		if err != nil {
			return err
		}
		target := normalizeCompareText(aliasText)
		for _, row := range rows {
			if characterMatchesName(row, target) {
				return fmt.Errorf("%s别名 %q 与已有%s重复", aliasTypeLabel(aliasType), aliasText, aliasTypeNameLabel(aliasType))
			}
		}
		return nil
	default:
		return fmt.Errorf("不支持的别名类型: %s", aliasType)
	}
}

func approvedAliasExists(ctx context.Context, client *pjskdb.AliasClient, aliasType, aliasText string) (bool, error) {
	return client.Query().
		Where(
			aliasdb.AliasTypeEQ(aliasType),
			aliasdb.AliasEqualFold(aliasText),
		).
		Exist(ctx)
}

func pendingAliasExists(ctx context.Context, client *pjskdb.PendingAliasClient, aliasType, aliasText string) (bool, error) {
	return client.Query().
		Where(
			pendingalias.AliasTypeEQ(aliasType),
			pendingalias.AliasEqualFold(aliasText),
		).
		Exist(ctx)
}

func (s *Service) requireAdmin(ctx context.Context, platform, platformUserID string) (*pjskdb.AliasAdmin, string, error) {
	if s == nil || s.identity == nil {
		return nil, "", onebot11.NewReplayError("别名审核服务未就绪，请稍后再试")
	}
	platform = strings.TrimSpace(platform)
	platformUserID = strings.TrimSpace(platformUserID)
	if platform == "" || platformUserID == "" {
		return nil, "", fmt.Errorf("缺少审核身份信息")
	}
	harukiUserID, err := s.identity.ResolveOrCreate(ctx, platform, platformUserID)
	if err != nil {
		return nil, "", err
	}
	row, err := s.pjsk.AliasAdmin.Query().
		Where(aliasadmin.HarukiUserIDEQ(harukiUserID)).
		Only(ctx)
	if err != nil {
		if pjskdb.IsNotFound(err) {
			return nil, "", onebot11.NewReplayError("你不是别名审核管理员")
		}
		return nil, "", err
	}
	return row, buildActorLabel(platform, platformUserID), nil
}
