package alias

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"haruki-cloud/api/bot/onebot11"
	pjskdb "haruki-cloud/database/pjsk"
	aliasdb "haruki-cloud/database/pjsk/alias"
	"haruki-cloud/database/pjsk/aliasadmin"
	"haruki-cloud/database/pjsk/pendingalias"
	sekaiDB "haruki-cloud/database/sekai"
	"haruki-cloud/database/sekai/gamecharacter"
	sekaimusic "haruki-cloud/database/sekai/music"
)

const (
	AliasTypeMusic     = "music"
	AliasTypeCharacter = "character"
)

var supportedAliasTypes = []string{AliasTypeMusic, AliasTypeCharacter}

type IdentityResolver interface {
	ResolveOrCreate(ctx context.Context, platform, userID string) (int, error)
}

type Service struct {
	sekai    *sekaiDB.Client
	pjsk     *pjskdb.Client
	identity IdentityResolver
}

type EntityRef struct {
	AliasType string
	ID        int
	Name      string
}

type AliasRecord struct {
	ReviewID int64
	Entity   EntityRef
	Alias    string
}

type ApprovedAliasRecord struct {
	AliasID int64
	Entity  EntityRef
	Alias   string
}

type QueryResult struct {
	Entity  EntityRef
	Aliases []string
}

type entityKey struct {
	aliasType string
	id        int
}

func NewService(sekai *sekaiDB.Client, pjsk *pjskdb.Client, identity IdentityResolver) *Service {
	if sekai == nil || pjsk == nil {
		return nil
	}
	return &Service{
		sekai:    sekai,
		pjsk:     pjsk,
		identity: identity,
	}
}

func (s *Service) IsReady() bool {
	return s != nil && s.sekai != nil && s.pjsk != nil
}

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

func (s *Service) ListPending(ctx context.Context, platform, platformUserID string) ([]AliasRecord, error) {
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

func (s *Service) Approve(ctx context.Context, platform, platformUserID string, reviewIDs []int64) ([]AliasRecord, error) {
	if !s.IsReady() {
		return nil, onebot11.NewReplayError("别名服务未就绪，请稍后再试")
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

func (s *Service) Reject(ctx context.Context, platform, platformUserID string, reviewID int64, reason string) (*AliasRecord, error) {
	if !s.IsReady() {
		return nil, onebot11.NewReplayError("别名服务未就绪，请稍后再试")
	}
	admin, reviewer, err := s.requireAdmin(ctx, platform, platformUserID)
	if err != nil {
		return nil, err
	}
	if reviewID <= 0 {
		return nil, onebot11.NewReplayError("请输入正确的待审核ID")
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

	row, err := tx.PendingAlias.Query().
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

	if _, err := tx.RejectedAlias.Create().
		SetAliasType(row.AliasType).
		SetAliasTypeID(row.AliasTypeID).
		SetAlias(row.Alias).
		SetReviewedBy(reviewer).
		SetReason(reason).
		SetReviewedAt(time.Now()).
		Save(ctx); err != nil {
		return nil, err
	}
	if err := tx.PendingAlias.DeleteOneID(reviewID).Exec(ctx); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	tx = nil

	records, err := s.buildAliasRecordsFromPending(ctx, []*pjskdb.PendingAlias{row})
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("未找到待审核别名ID: %d", reviewID)
	}
	return &records[0], nil
}

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
	case AliasTypeMusic:
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
	case AliasTypeCharacter:
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

func (s *Service) buildAliasRecordsFromPending(ctx context.Context, rows []*pjskdb.PendingAlias) ([]AliasRecord, error) {
	refs, err := s.loadEntityRefs(ctx, pendingRowsToKeys(rows))
	if err != nil {
		return nil, err
	}
	records := make([]AliasRecord, 0, len(rows))
	for _, row := range rows {
		ref := refs[entityMapKey(row.AliasType, row.AliasTypeID)]
		records = append(records, AliasRecord{
			ReviewID: row.ID,
			Entity:   ref,
			Alias:    row.Alias,
		})
	}
	return records, nil
}

func orderedPendingAliases(reviewIDs []int64, byID map[int64]*pjskdb.PendingAlias) []*pjskdb.PendingAlias {
	result := make([]*pjskdb.PendingAlias, 0, len(reviewIDs))
	for _, reviewID := range reviewIDs {
		if row, ok := byID[reviewID]; ok {
			result = append(result, row)
		}
	}
	return result
}

func pendingRowsToKeys(rows []*pjskdb.PendingAlias) []entityKey {
	keys := make([]entityKey, 0, len(rows))
	for _, row := range rows {
		keys = append(keys, entityKey{aliasType: row.AliasType, id: row.AliasTypeID})
	}
	return keys
}

func (s *Service) loadEntityRefs(ctx context.Context, keys []entityKey) (map[string]EntityRef, error) {
	result := make(map[string]EntityRef)
	if len(keys) == 0 {
		return result, nil
	}

	musicIDs := make([]int, 0, len(keys))
	characterIDs := make([]int, 0, len(keys))
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		if key.id <= 0 {
			continue
		}
		mapKey := entityMapKey(key.aliasType, key.id)
		if _, ok := seen[mapKey]; ok {
			continue
		}
		seen[mapKey] = struct{}{}
		switch key.aliasType {
		case AliasTypeMusic:
			musicIDs = append(musicIDs, key.id)
		case AliasTypeCharacter:
			characterIDs = append(characterIDs, key.id)
		}
	}

	musicNames, err := s.loadMusicTitles(ctx, musicIDs)
	if err != nil {
		return nil, err
	}
	for _, musicID := range musicIDs {
		result[entityMapKey(AliasTypeMusic, musicID)] = EntityRef{
			AliasType: AliasTypeMusic,
			ID:        musicID,
			Name:      musicNames[musicID],
		}
	}

	characterNames, err := s.loadCharacterNames(ctx, characterIDs)
	if err != nil {
		return nil, err
	}
	for _, characterID := range characterIDs {
		result[entityMapKey(AliasTypeCharacter, characterID)] = EntityRef{
			AliasType: AliasTypeCharacter,
			ID:        characterID,
			Name:      characterNames[characterID],
		}
	}

	return result, nil
}

func (s *Service) loadMusicTitles(ctx context.Context, musicIDs []int) (map[int]string, error) {
	result := make(map[int]string)
	if len(musicIDs) == 0 {
		return result, nil
	}
	ids64 := make([]int64, 0, len(musicIDs))
	seen := make(map[int]struct{}, len(musicIDs))
	for _, musicID := range musicIDs {
		if musicID <= 0 {
			continue
		}
		if _, ok := seen[musicID]; ok {
			continue
		}
		seen[musicID] = struct{}{}
		ids64 = append(ids64, int64(musicID))
	}
	rows, err := s.sekai.Music.Query().
		Where(sekaimusic.GameIDIn(ids64...)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	grouped := make(map[int][]*sekaiDB.Music)
	for _, row := range rows {
		grouped[int(row.GameID)] = append(grouped[int(row.GameID)], row)
	}
	for _, musicID := range musicIDs {
		if items, ok := grouped[musicID]; ok {
			result[musicID] = preferredMusicTitle(items, musicID)
			continue
		}
		result[musicID] = fmt.Sprintf("%s%d", aliasTypeLabel(AliasTypeMusic), musicID)
	}
	return result, nil
}

func (s *Service) loadCharacterNames(ctx context.Context, characterIDs []int) (map[int]string, error) {
	result := make(map[int]string)
	if len(characterIDs) == 0 {
		return result, nil
	}
	ids64 := make([]int64, 0, len(characterIDs))
	seen := make(map[int]struct{}, len(characterIDs))
	for _, characterID := range characterIDs {
		if characterID <= 0 {
			continue
		}
		if _, ok := seen[characterID]; ok {
			continue
		}
		seen[characterID] = struct{}{}
		ids64 = append(ids64, int64(characterID))
	}
	rows, err := s.sekai.Gamecharacter.Query().
		Where(gamecharacter.GameIDIn(ids64...)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	grouped := make(map[int][]*sekaiDB.Gamecharacter)
	for _, row := range rows {
		grouped[int(row.GameID)] = append(grouped[int(row.GameID)], row)
	}
	for _, characterID := range characterIDs {
		if items, ok := grouped[characterID]; ok {
			result[characterID] = preferredCharacterName(items, characterID)
			continue
		}
		result[characterID] = fmt.Sprintf("%s%d", aliasTypeLabel(AliasTypeCharacter), characterID)
	}
	return result, nil
}

func preferredMusicTitle(rows []*sekaiDB.Music, musicID int) string {
	bestTitle := ""
	bestRank := 999
	for _, row := range rows {
		title := strings.TrimSpace(row.Title)
		if title == "" {
			continue
		}
		rank := serverRegionRank(row.ServerRegion)
		if rank < bestRank {
			bestRank = rank
			bestTitle = title
		}
	}
	if bestTitle == "" {
		return fmt.Sprintf("%s%d", aliasTypeLabel(AliasTypeMusic), musicID)
	}
	return bestTitle
}

func preferredCharacterName(rows []*sekaiDB.Gamecharacter, characterID int) string {
	bestName := ""
	bestRank := 999
	for _, row := range rows {
		for _, candidate := range characterDisplayNames(row) {
			name := strings.TrimSpace(candidate)
			if name == "" {
				continue
			}
			rank := serverRegionRank(row.ServerRegion)
			if rank < bestRank {
				bestRank = rank
				bestName = name
				break
			}
		}
	}
	if bestName == "" {
		return fmt.Sprintf("%s%d", aliasTypeLabel(AliasTypeCharacter), characterID)
	}
	return bestName
}

func characterMatchesName(row *sekaiDB.Gamecharacter, normalizedTarget string) bool {
	if normalizedTarget == "" {
		return false
	}
	for _, candidate := range characterMatchNames(row) {
		if normalizeCompareText(candidate) == normalizedTarget {
			return true
		}
	}
	return false
}

func characterDisplayNames(row *sekaiDB.Gamecharacter) []string {
	values := make([]string, 0, 4)
	appendUniqueString(&values, strings.TrimSpace(row.FirstName+row.GivenName))
	appendUniqueString(&values, strings.TrimSpace(strings.TrimSpace(row.FirstName)+" "+strings.TrimSpace(row.GivenName)))
	appendUniqueString(&values, strings.TrimSpace(row.FirstNameEnglish+row.GivenNameEnglish))
	appendUniqueString(&values, strings.TrimSpace(strings.TrimSpace(row.FirstNameEnglish)+" "+strings.TrimSpace(row.GivenNameEnglish)))
	return values
}

func characterMatchNames(row *sekaiDB.Gamecharacter) []string {
	values := characterDisplayNames(row)
	appendUniqueString(&values, row.FirstName)
	appendUniqueString(&values, row.GivenName)
	appendUniqueString(&values, row.FirstNameEnglish)
	appendUniqueString(&values, row.GivenNameEnglish)
	return values
}

func appendUniqueString(values *[]string, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	for _, existing := range *values {
		if strings.EqualFold(existing, value) {
			return
		}
	}
	*values = append(*values, value)
}

func entityMapKey(aliasType string, id int) string {
	return aliasType + ":" + strconv.Itoa(id)
}

func serverRegionRank(region string) int {
	switch strings.ToLower(strings.TrimSpace(region)) {
	case "jp":
		return 0
	case "cn":
		return 1
	case "tw":
		return 2
	case "kr":
		return 3
	case "en":
		return 4
	default:
		return 5
	}
}

func ambiguousEntityError(aliasType, sourceName string, ids []int, names map[int]string) error {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, fmt.Sprintf("%d/%s", id, names[id]))
	}
	if aliasType == AliasTypeMusic {
		lines := make([]string, 0, len(ids))
		for _, id := range ids {
			lines = append(lines, fmt.Sprintf("music%d/%s", id, names[id]))
		}
		return fmt.Errorf("%s匹配到多个%s，请改用 music<id> 查询：\n%s", sourceName, aliasTypeLabel(aliasType), strings.Join(lines, "\n"))
	}
	return fmt.Errorf("%s匹配到多个%s，请改用%s：\n%s", sourceName, aliasTypeLabel(aliasType), aliasTypeIDLabel(aliasType), strings.Join(parts, "\n"))
}

func normalizeSubmittedAliases(raw []string) ([]string, error) {
	cleaned := make([]string, 0, len(raw))
	seen := make(map[string]string, len(raw))
	for _, item := range raw {
		aliasText := strings.TrimSpace(item)
		if aliasText == "" {
			continue
		}
		key := normalizeCompareText(aliasText)
		if previous, ok := seen[key]; ok {
			return nil, fmt.Errorf("别名 %q 与 %q 重复", previous, aliasText)
		}
		seen[key] = aliasText
		cleaned = append(cleaned, aliasText)
	}
	if len(cleaned) == 0 {
		return nil, fmt.Errorf("请至少提供一个非空别名")
	}
	return cleaned, nil
}

func normalizeReviewIDs(reviewIDs []int64) ([]int64, error) {
	if len(reviewIDs) == 0 {
		return nil, fmt.Errorf("请至少提供一个待审核ID")
	}
	result := make([]int64, 0, len(reviewIDs))
	seen := make(map[int64]struct{}, len(reviewIDs))
	for _, reviewID := range reviewIDs {
		if reviewID <= 0 {
			return nil, fmt.Errorf("待审核ID必须为正整数")
		}
		if _, ok := seen[reviewID]; ok {
			continue
		}
		seen[reviewID] = struct{}{}
		result = append(result, reviewID)
	}
	return result, nil
}

func normalizeAliasType(aliasType string) (string, error) {
	aliasType = strings.ToLower(strings.TrimSpace(aliasType))
	switch aliasType {
	case AliasTypeMusic, AliasTypeCharacter:
		return aliasType, nil
	default:
		return "", fmt.Errorf("不支持的别名类型: %s", aliasType)
	}
}

func aliasTypeLabel(aliasType string) string {
	switch aliasType {
	case AliasTypeMusic:
		return "歌曲"
	case AliasTypeCharacter:
		return "角色"
	default:
		return "未知类型"
	}
}

func aliasTypeIDLabel(aliasType string) string {
	switch aliasType {
	case AliasTypeMusic:
		return "歌曲ID"
	case AliasTypeCharacter:
		return "角色ID"
	default:
		return "目标ID"
	}
}

func aliasTypeNameLabel(aliasType string) string {
	switch aliasType {
	case AliasTypeMusic:
		return "曲名"
	case AliasTypeCharacter:
		return "角色名"
	default:
		return "名称"
	}
}

func entityTokenPrompt(aliasType string) string {
	switch aliasType {
	case AliasTypeMusic:
		return "歌曲ID、曲名或已审核别名"
	case AliasTypeCharacter:
		return "角色ID、角色名或已审核别名"
	default:
		return "ID、名称或已审核别名"
	}
}

func buildActorLabel(platform, platformUserID string) string {
	platform = strings.TrimSpace(platform)
	platformUserID = strings.TrimSpace(platformUserID)
	if platform == "" && platformUserID == "" {
		return "unknown"
	}
	if platform == "" {
		return platformUserID
	}
	if platformUserID == "" {
		return platform
	}
	return platform + ":" + platformUserID
}

func normalizeCompareText(text string) string {
	return strings.ToLower(strings.TrimSpace(text))
}

func sortAliasTexts(values []string) {
	sort.Slice(values, func(i, j int) bool {
		left := normalizeCompareText(values[i])
		right := normalizeCompareText(values[j])
		if left == right {
			return values[i] < values[j]
		}
		return left < right
	})
}

func formatAliasRecord(record AliasRecord) string {
	return fmt.Sprintf("审核ID: %d | 类型: %s | 目标ID: %d | 名称: %s | 别名: %s", record.ReviewID, aliasTypeLabel(record.Entity.AliasType), record.Entity.ID, record.Entity.Name, record.Alias)
}

func formatRejectedAliasRecord(record AliasRecord, reason string) string {
	return formatAliasRecord(record) + "\n原因: " + reason
}

func formatApprovedAliasRecord(record ApprovedAliasRecord) string {
	return fmt.Sprintf("别名ID: %d | 类型: %s | 目标ID: %d | 名称: %s | 别名: %s", record.AliasID, aliasTypeLabel(record.Entity.AliasType), record.Entity.ID, record.Entity.Name, record.Alias)
}
