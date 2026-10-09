package alias

import (
	"context"
	"errors"
	"strings"
	"time"

	pjskdb "haruki-cloud/database/pjsk"
	aliasdb "haruki-cloud/database/pjsk/alias"
	"haruki-cloud/database/pjsk/aliassubmissionban"
	"haruki-cloud/internal/i18n"
	"haruki-cloud/utils/usererror"
)

func (s *Service) Submit(ctx context.Context, aliasType, platform, platformUserID, target string, aliasesToSubmit []string) ([]PjskAliasRecord, error) {
	if !s.IsReady() {
		return nil, errAliasUnavailable()
	}
	if err := s.requireWritable(); err != nil {
		return nil, err
	}
	submitter, err := s.validateSubmitter(ctx, platform, platformUserID)
	if err != nil {
		return nil, err
	}
	aliasType, err = normalizeAliasType(aliasType)
	if err != nil {
		return nil, err
	}
	entityRef, err := s.resolveEntityByToken(ctx, aliasType, target)
	if err != nil {
		return nil, err
	}
	cleanedAliases, err := normalizeSubmittedAliases(aliasesToSubmit)
	if err != nil {
		return nil, err
	}
	if err := s.ensureAliasesAvailable(ctx, aliasType, cleanedAliases); err != nil {
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

	records, err := createPendingAliases(ctx, tx, aliasType, entityRef, cleanedAliases, submitter)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	tx = nil
	return records, nil
}

func (s *Service) validateSubmitter(ctx context.Context, platform, platformUserID string) (string, error) {
	platform = strings.TrimSpace(platform)
	platformUserID = strings.TrimSpace(platformUserID)
	if platform == "" || platformUserID == "" {
		return "", errors.New("alias submitter identity is missing")
	}
	banned, err := s.pjsk.AliasSubmissionBan.Query().
		Where(
			aliassubmissionban.PlatformEQ(platform),
			aliassubmissionban.PlatformUserIDEQ(platformUserID),
		).
		Exist(ctx)
	if err != nil {
		return "", err
	}
	if banned {
		return "", usererror.Forbidden(i18n.M("alias.submitter_banned"))
	}
	return buildActorLabel(platform, platformUserID), nil
}

func (s *Service) ensureAliasesAvailable(ctx context.Context, aliasType string, aliases []string) error {
	for _, aliasText := range aliases {
		if err := s.ensureAliasAvailable(ctx, aliasType, s.pjsk.Alias, s.pjsk.PendingAlias, aliasText); err != nil {
			return err
		}
	}
	return nil
}

func createPendingAliases(ctx context.Context, tx *pjskdb.Tx, aliasType string, entity EntityRef, aliases []string, submitter string) ([]PjskAliasRecord, error) {
	records := make([]PjskAliasRecord, 0, len(aliases))
	now := time.Now()
	for _, aliasText := range aliases {
		row, err := tx.PendingAlias.Create().
			SetAliasType(aliasType).
			SetAliasTypeID(entity.ID).
			SetAlias(aliasText).
			SetSubmittedBy(submitter).
			SetSubmittedAt(now).
			Save(ctx)
		if pjskdb.IsConstraintError(err) {
			return nil, usererror.Invalid(i18n.M("alias.already_pending", i18n.Data{"Kind": aliasKind(aliasType), "Alias": aliasText}))
		}
		if err != nil {
			return nil, err
		}
		records = append(records, PjskAliasRecord{
			ReviewID: row.ID,
			Entity:   entity,
			Alias:    row.Alias,
		})
	}
	return records, nil
}

func (s *Service) Query(ctx context.Context, aliasType, target string) (*QueryResult, error) {
	if !s.IsReady() {
		return nil, errAliasUnavailable()
	}
	aliasType, err := normalizeAliasType(aliasType)
	if err != nil {
		return nil, err
	}
	entityRef, err := s.resolveEntityByToken(ctx, aliasType, target)
	if err != nil {
		return nil, err
	}
	rows, err := s.pjsk.Alias.Query().
		Where(
			aliasdb.AliasTypeEQ(aliasType),
			aliasdb.AliasTypeIDEQ(entityRef.ID),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}
	aliases := make([]string, 0, len(rows))
	for _, row := range rows {
		value := strings.TrimSpace(row.Alias)
		if value == "" {
			continue
		}
		aliases = append(aliases, value)
	}
	sortAliasTexts(aliases)
	return &QueryResult{
		Entity:  entityRef,
		Aliases: aliases,
	}, nil
}

func (s *Service) ListApprovedMusicAliases(ctx context.Context, musicID int) ([]string, error) {
	if !s.IsReady() || musicID <= 0 {
		return nil, nil
	}
	rows, err := s.pjsk.Alias.Query().
		Where(
			aliasdb.AliasTypeEQ(PjskAliasTypeMusic),
			aliasdb.AliasTypeIDEQ(musicID),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}
	aliases := make([]string, 0, len(rows))
	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		value := strings.TrimSpace(row.Alias)
		if value == "" {
			continue
		}
		key := normalizeCompareText(value)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		aliases = append(aliases, value)
	}
	sortAliasTexts(aliases)
	return aliases, nil
}

func (s *Service) ListApprovedCharacterAliasMap(ctx context.Context) (map[string]int, error) {
	result := make(map[string]int)
	if !s.IsReady() {
		return result, nil
	}

	rows, err := s.pjsk.Alias.Query().
		Where(aliasdb.AliasTypeEQ(PjskAliasTypeCharacter)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	owners := make(map[string]int, len(rows))
	conflicts := make(map[string]struct{})
	for _, row := range rows {
		if row == nil || row.AliasTypeID <= 0 {
			continue
		}
		key := normalizeCompareText(row.Alias)
		if key == "" {
			continue
		}
		if _, conflicted := conflicts[key]; conflicted {
			continue
		}
		if existing, ok := owners[key]; ok && existing != row.AliasTypeID {
			delete(result, key)
			conflicts[key] = struct{}{}
			continue
		}
		owners[key] = row.AliasTypeID
		result[key] = row.AliasTypeID
	}
	return result, nil
}

func (s *Service) Delete(ctx context.Context, aliasType, platform, platformUserID, target string, aliasesToDelete []string) ([]ApprovedAliasRecord, error) {
	if !s.IsReady() {
		return nil, errAliasUnavailable()
	}
	if err := s.requireWritable(); err != nil {
		return nil, err
	}
	aliasType, err := normalizeAliasType(aliasType)
	if err != nil {
		return nil, err
	}
	if _, _, err := s.requireAdmin(ctx, platform, platformUserID); err != nil {
		return nil, err
	}
	entityRef, err := s.resolveEntityByToken(ctx, aliasType, target)
	if err != nil {
		return nil, err
	}
	cleanedAliases, err := normalizeSubmittedAliases(aliasesToDelete)
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

	rows, err := tx.Alias.Query().
		Where(
			aliasdb.AliasTypeEQ(aliasType),
			aliasdb.AliasTypeIDEQ(entityRef.ID),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}

	byAlias := make(map[string]*pjskdb.Alias, len(rows))
	for _, row := range rows {
		key := normalizeCompareText(row.Alias)
		if key == "" {
			continue
		}
		byAlias[key] = row
	}

	result := make([]ApprovedAliasRecord, 0, len(cleanedAliases))
	for _, aliasText := range cleanedAliases {
		row, ok := byAlias[normalizeCompareText(aliasText)]
		if !ok {
			return nil, usererror.New(usererror.CodeNotFound, i18n.M("alias.approved_not_found", i18n.Data{"Kind": aliasKind(aliasType), "Target": entityRef.Name, "Alias": aliasText}))
		}
		if err := tx.Alias.DeleteOneID(row.ID).Exec(ctx); err != nil {
			return nil, err
		}
		result = append(result, ApprovedAliasRecord{
			AliasID: row.ID,
			Entity:  entityRef,
			Alias:   row.Alias,
		})
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	tx = nil
	changes := make([]AliasChange, 0, len(result))
	for _, record := range result {
		changes = append(changes, AliasChange{AliasType: aliasType, AliasTypeID: record.Entity.ID, Alias: record.Alias})
	}
	s.notifyChanged(ctx, changes)
	return result, nil
}
