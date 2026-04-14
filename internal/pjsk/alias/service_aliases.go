package alias

import (
	"context"
	"fmt"
	"strings"
	"time"

	pjskdb "haruki-cloud/database/pjsk"
	aliasdb "haruki-cloud/database/pjsk/alias"
)

func (s *Service) Submit(ctx context.Context, aliasType, platform, platformUserID, target string, aliasesToSubmit []string) ([]AliasRecord, error) {
	if !s.IsReady() {
		return nil, fmt.Errorf("别名服务未就绪，请稍后再试")
	}
	aliasType, err := normalizeAliasType(aliasType)
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
	for _, aliasText := range cleanedAliases {
		if err := s.ensureAliasAvailable(ctx, aliasType, s.pjsk.Alias, s.pjsk.PendingAlias, aliasText); err != nil {
			return nil, err
		}
	}

	submitter := buildActorLabel(platform, platformUserID)
	now := time.Now()
	tx, err := s.pjsk.Tx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	records := make([]AliasRecord, 0, len(cleanedAliases))
	for _, aliasText := range cleanedAliases {
		row, err := tx.PendingAlias.Create().
			SetAliasType(aliasType).
			SetAliasTypeID(entityRef.ID).
			SetAlias(aliasText).
			SetSubmittedBy(submitter).
			SetSubmittedAt(now).
			Save(ctx)
		if err != nil {
			if pjskdb.IsConstraintError(err) {
				return nil, fmt.Errorf("%s别名 %q 已经在待审核列表中", aliasTypeLabel(aliasType), aliasText)
			}
			return nil, err
		}
		records = append(records, AliasRecord{
			ReviewID: row.ID,
			Entity:   entityRef,
			Alias:    row.Alias,
		})
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	tx = nil
	return records, nil
}

func (s *Service) Query(ctx context.Context, aliasType, target string) (*QueryResult, error) {
	if !s.IsReady() {
		return nil, fmt.Errorf("别名服务未就绪，请稍后再试")
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
			aliasdb.AliasTypeEQ(AliasTypeMusic),
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

func (s *Service) Delete(ctx context.Context, aliasType, platform, platformUserID, target string, aliasesToDelete []string) ([]ApprovedAliasRecord, error) {
	if !s.IsReady() {
		return nil, fmt.Errorf("别名服务未就绪，请稍后再试")
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
			return nil, fmt.Errorf("未找到%s %d %s 下的已审核别名 %q", aliasTypeLabel(aliasType), entityRef.ID, entityRef.Name, aliasText)
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
	return result, nil
}
