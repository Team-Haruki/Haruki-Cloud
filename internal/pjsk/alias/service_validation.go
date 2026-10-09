package alias

import (
	"context"
	"errors"
	"fmt"
	"strings"

	pjskdb "haruki-cloud/database/pjsk"
	aliasdb "haruki-cloud/database/pjsk/alias"
	"haruki-cloud/database/pjsk/aliasadmin"
	"haruki-cloud/database/pjsk/pendingalias"
	"haruki-cloud/database/sekai/gamecharacter"
	sekaimusic "haruki-cloud/database/sekai/music"
	"haruki-cloud/internal/i18n"
	"haruki-cloud/utils/usererror"
)

func (s *Service) ensureAliasAvailable(ctx context.Context, aliasType string, approved *pjskdb.AliasClient, pending *pjskdb.PendingAliasClient, aliasText string) error {
	if err := s.ensureEntityNameAvailable(ctx, aliasType, aliasText, false); err != nil {
		return err
	}
	exists, err := approvedAliasExists(ctx, approved, aliasType, aliasText)
	if err != nil {
		return err
	}
	if exists {
		return usererror.Invalid(i18n.M("alias.already_approved", i18n.Data{"Kind": aliasKind(aliasType), "UserAlias": i18n.EchoQuery(aliasText)}))
	}
	pendingExists, err := pendingAliasExists(ctx, pending, aliasType, aliasText)
	if err != nil {
		return err
	}
	if pendingExists {
		return usererror.Invalid(i18n.M("alias.already_pending", i18n.Data{"Kind": aliasKind(aliasType), "UserAlias": i18n.EchoQuery(aliasText)}))
	}
	return nil
}

// conflictsNameError is the reply when aliasText equals a song or character
// name. review is true on the approval path, whose replies only alias review
// admins receive: they always see the text; a submitter sees it only with
// parameter echo.
func conflictsNameError(aliasType, aliasText string, review bool) error {
	if review {
		return usererror.Invalid(i18n.M("alias.review.conflicts_name", i18n.Data{"Kind": aliasKind(aliasType), "Alias": aliasText, "NameKind": aliasNameKind(aliasType)}))
	}
	return usererror.Invalid(i18n.M("alias.conflicts_name", i18n.Data{"Kind": aliasKind(aliasType), "UserAlias": i18n.EchoQuery(aliasText), "NameKind": aliasNameKind(aliasType)}))
}

func (s *Service) ensureEntityNameAvailable(ctx context.Context, aliasType, aliasText string, review bool) error {
	switch aliasType {
	case PjskAliasTypeMusic:
		conflicts, err := s.sekai.Music.Query().
			Where(sekaimusic.TitleEqualFold(aliasText)).
			Count(ctx)
		if err != nil {
			return err
		}
		if conflicts > 0 {
			return conflictsNameError(aliasType, aliasText, review)
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
				return conflictsNameError(aliasType, aliasText, review)
			}
		}
		return nil
	default:
		return usererror.Wrap(usererror.CodeInternal, i18n.M("alias.type_unsupported"), fmt.Errorf("unsupported alias type %q", aliasType))
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
		return nil, "", errAliasUnavailable()
	}
	platform = strings.TrimSpace(platform)
	platformUserID = strings.TrimSpace(platformUserID)
	if platform == "" || platformUserID == "" {
		return nil, "", errors.New("alias review identity is missing")
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
			return nil, "", usererror.Forbidden(i18n.M("alias.not_admin"))
		}
		return nil, "", err
	}
	return row, buildActorLabel(platform, platformUserID), nil
}
