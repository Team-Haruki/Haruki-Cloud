package musicalias

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	pjskdb "haruki-cloud/database/pjsk"
	"haruki-cloud/database/pjsk/alias"
	"haruki-cloud/database/pjsk/aliasadmin"
	"haruki-cloud/database/pjsk/pendingalias"
	sekaiDB "haruki-cloud/database/sekai"
	sekaimusic "haruki-cloud/database/sekai/music"
)

const (
	AliasTypeMusic = "music"
)

type IdentityResolver interface {
	ResolveOrCreate(ctx context.Context, platform, userID string) (int, error)
}

type Service struct {
	sekai    *sekaiDB.Client
	pjsk     *pjskdb.Client
	identity IdentityResolver
}

type MusicRef struct {
	ID    int
	Title string
}

type AliasRecord struct {
	ReviewID int64
	Music    MusicRef
	Alias    string
}

type ApprovedAliasRecord struct {
	AliasID int64
	Music   MusicRef
	Alias   string
}

type QueryResult struct {
	Music   MusicRef
	Aliases []string
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

func (s *Service) Submit(ctx context.Context, platform, platformUserID, target string, aliasesToSubmit []string) ([]AliasRecord, error) {
	if !s.IsReady() {
		return nil, fmt.Errorf("歌曲别名服务未就绪，请稍后再试")
	}
	musicRef, err := s.resolveMusicByToken(ctx, target)
	if err != nil {
		return nil, err
	}
	cleanedAliases, err := normalizeSubmittedAliases(aliasesToSubmit)
	if err != nil {
		return nil, err
	}
	for _, aliasText := range cleanedAliases {
		if err := s.ensureAliasAvailable(ctx, s.pjsk.Alias, s.pjsk.PendingAlias, aliasText); err != nil {
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
			SetAliasType(AliasTypeMusic).
			SetAliasTypeID(musicRef.ID).
			SetAlias(aliasText).
			SetSubmittedBy(submitter).
			SetSubmittedAt(now).
			Save(ctx)
		if err != nil {
			if pjskdb.IsConstraintError(err) {
				return nil, fmt.Errorf("别名 %q 已经在待审核列表中", aliasText)
			}
			return nil, err
		}
		records = append(records, AliasRecord{
			ReviewID: row.ID,
			Music:    musicRef,
			Alias:    row.Alias,
		})
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	tx = nil
	return records, nil
}

func (s *Service) Query(ctx context.Context, target string) (*QueryResult, error) {
	if !s.IsReady() {
		return nil, fmt.Errorf("歌曲别名服务未就绪，请稍后再试")
	}
	musicRef, err := s.resolveMusicByToken(ctx, target)
	if err != nil {
		return nil, err
	}
	rows, err := s.pjsk.Alias.Query().
		Where(
			alias.AliasTypeEQ(AliasTypeMusic),
			alias.AliasTypeIDEQ(musicRef.ID),
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
		Music:   musicRef,
		Aliases: aliases,
	}, nil
}

func (s *Service) Delete(ctx context.Context, platform, platformUserID, target string, aliasesToDelete []string) ([]ApprovedAliasRecord, error) {
	if !s.IsReady() {
		return nil, fmt.Errorf("歌曲别名服务未就绪，请稍后再试")
	}
	if _, _, err := s.requireAdmin(ctx, platform, platformUserID); err != nil {
		return nil, err
	}
	musicRef, err := s.resolveMusicByToken(ctx, target)
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
			alias.AliasTypeEQ(AliasTypeMusic),
			alias.AliasTypeIDEQ(musicRef.ID),
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
			return nil, fmt.Errorf("未找到歌曲 %d %s 下的已审核别名 %q", musicRef.ID, musicRef.Title, aliasText)
		}
		if err := tx.Alias.DeleteOneID(row.ID).Exec(ctx); err != nil {
			return nil, err
		}
		result = append(result, ApprovedAliasRecord{
			AliasID: row.ID,
			Music:   musicRef,
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
		return nil, fmt.Errorf("歌曲别名服务未就绪，请稍后再试")
	}
	if _, _, err := s.requireAdmin(ctx, platform, platformUserID); err != nil {
		return nil, err
	}
	rows, err := s.pjsk.PendingAlias.Query().
		Where(pendingalias.AliasTypeEQ(AliasTypeMusic)).
		Order(pendingalias.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return s.buildAliasRecordsFromPending(ctx, rows)
}

func (s *Service) Approve(ctx context.Context, platform, platformUserID string, reviewIDs []int64) ([]AliasRecord, error) {
	if !s.IsReady() {
		return nil, fmt.Errorf("歌曲别名服务未就绪，请稍后再试")
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
			pendingalias.AliasTypeEQ(AliasTypeMusic),
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
		return nil, fmt.Errorf("未找到待审核别名ID: %s", strings.Join(missing, " "))
	}

	reserved := make(map[string]int64, len(uniqueIDs))
	for _, reviewID := range uniqueIDs {
		row := byID[reviewID]
		if err := s.ensureTitleAvailable(ctx, row.Alias); err != nil {
			return nil, err
		}
		exists, err := approvedAliasExists(ctx, tx.Alias, row.Alias)
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, fmt.Errorf("别名 %q 已经存在于已审核列表中", row.Alias)
		}
		key := normalizeCompareText(row.Alias)
		if prevID, ok := reserved[key]; ok {
			return nil, fmt.Errorf("待通过的审核ID %d 与 %d 使用了重复别名 %q", prevID, reviewID, row.Alias)
		}
		reserved[key] = reviewID
	}

	for _, reviewID := range uniqueIDs {
		row := byID[reviewID]
		if _, err := tx.Alias.Create().
			SetAliasType(AliasTypeMusic).
			SetAliasTypeID(row.AliasTypeID).
			SetAlias(row.Alias).
			Save(ctx); err != nil {
			if pjskdb.IsConstraintError(err) {
				return nil, fmt.Errorf("别名 %q 已经存在于已审核列表中", row.Alias)
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
		return nil, fmt.Errorf("歌曲别名服务未就绪，请稍后再试")
	}
	admin, reviewer, err := s.requireAdmin(ctx, platform, platformUserID)
	if err != nil {
		return nil, err
	}
	if reviewID <= 0 {
		return nil, fmt.Errorf("请输入正确的待审核ID")
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, fmt.Errorf("请输入拒绝原因")
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
			pendingalias.AliasTypeEQ(AliasTypeMusic),
			pendingalias.IDEQ(reviewID),
		).
		Only(ctx)
	if err != nil {
		if pjskdb.IsNotFound(err) {
			return nil, fmt.Errorf("未找到待审核别名ID: %d", reviewID)
		}
		return nil, err
	}

	if _, err := tx.RejectedAlias.Create().
		SetAliasType(AliasTypeMusic).
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

	titles, err := s.loadMusicTitles(ctx, []int{row.AliasTypeID})
	if err != nil {
		return nil, err
	}
	return &AliasRecord{
		ReviewID: row.ID,
		Music: MusicRef{
			ID:    row.AliasTypeID,
			Title: titles[row.AliasTypeID],
		},
		Alias: row.Alias,
	}, nil
}

func (s *Service) ensureAliasAvailable(ctx context.Context, approved *pjskdb.AliasClient, pending *pjskdb.PendingAliasClient, aliasText string) error {
	if err := s.ensureTitleAvailable(ctx, aliasText); err != nil {
		return err
	}
	exists, err := approvedAliasExists(ctx, approved, aliasText)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("别名 %q 已经存在于已审核列表中", aliasText)
	}
	pendingExists, err := pendingAliasExists(ctx, pending, aliasText)
	if err != nil {
		return err
	}
	if pendingExists {
		return fmt.Errorf("别名 %q 已经在待审核列表中", aliasText)
	}
	return nil
}

func (s *Service) ensureTitleAvailable(ctx context.Context, aliasText string) error {
	conflicts, err := s.sekai.Music.Query().
		Where(sekaimusic.TitleEqualFold(aliasText)).
		Count(ctx)
	if err != nil {
		return err
	}
	if conflicts > 0 {
		return fmt.Errorf("别名 %q 与已有曲名重复", aliasText)
	}
	return nil
}

func approvedAliasExists(ctx context.Context, client *pjskdb.AliasClient, aliasText string) (bool, error) {
	return client.Query().
		Where(
			alias.AliasTypeEQ(AliasTypeMusic),
			alias.AliasEqualFold(aliasText),
		).
		Exist(ctx)
}

func pendingAliasExists(ctx context.Context, client *pjskdb.PendingAliasClient, aliasText string) (bool, error) {
	return client.Query().
		Where(
			pendingalias.AliasTypeEQ(AliasTypeMusic),
			pendingalias.AliasEqualFold(aliasText),
		).
		Exist(ctx)
}

func (s *Service) requireAdmin(ctx context.Context, platform, platformUserID string) (*pjskdb.AliasAdmin, string, error) {
	if s == nil || s.identity == nil {
		return nil, "", fmt.Errorf("歌曲别名审核服务未就绪，请稍后再试")
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
			return nil, "", fmt.Errorf("你不是歌曲别名审核管理员")
		}
		return nil, "", err
	}
	return row, buildActorLabel(platform, platformUserID), nil
}

func (s *Service) resolveMusicByToken(ctx context.Context, token string) (MusicRef, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return MusicRef{}, fmt.Errorf("请输入歌曲ID、曲名或已审核别名")
	}
	if musicRef, ok, err := s.tryResolveMusicByID(ctx, token); err != nil || ok {
		return musicRef, err
	}
	if musicRef, ok, err := s.tryResolveMusicByTitle(ctx, token); err != nil || ok {
		return musicRef, err
	}
	if musicRef, ok, err := s.tryResolveMusicByApprovedAlias(ctx, token); err != nil || ok {
		return musicRef, err
	}
	return MusicRef{}, fmt.Errorf("未找到对应歌曲，请检查歌曲ID、曲名或已审核别名")
}

func (s *Service) tryResolveMusicByID(ctx context.Context, token string) (MusicRef, bool, error) {
	id, err := strconv.Atoi(token)
	if err != nil || id <= 0 {
		return MusicRef{}, false, nil
	}
	rows, err := s.sekai.Music.Query().
		Where(sekaimusic.GameIDEQ(int64(id))).
		All(ctx)
	if err != nil {
		return MusicRef{}, true, err
	}
	if len(rows) == 0 {
		return MusicRef{}, true, fmt.Errorf("未找到歌曲ID: %d", id)
	}
	return MusicRef{ID: id, Title: preferredMusicTitle(rows, id)}, true, nil
}

func (s *Service) tryResolveMusicByTitle(ctx context.Context, token string) (MusicRef, bool, error) {
	rows, err := s.sekai.Music.Query().
		Where(sekaimusic.TitleEqualFold(token)).
		All(ctx)
	if err != nil {
		return MusicRef{}, true, err
	}
	if len(rows) == 0 {
		return MusicRef{}, false, nil
	}
	musicRef, err := uniqueMusicFromRows(rows, "曲名")
	return musicRef, true, err
}

func (s *Service) tryResolveMusicByApprovedAlias(ctx context.Context, token string) (MusicRef, bool, error) {
	rows, err := s.pjsk.Alias.Query().
		Where(
			alias.AliasTypeEQ(AliasTypeMusic),
			alias.AliasEqualFold(token),
		).
		All(ctx)
	if err != nil {
		return MusicRef{}, true, err
	}
	if len(rows) == 0 {
		return MusicRef{}, false, nil
	}
	musicIDs := make([]int, 0, len(rows))
	seen := make(map[int]struct{}, len(rows))
	for _, row := range rows {
		if _, ok := seen[row.AliasTypeID]; ok {
			continue
		}
		seen[row.AliasTypeID] = struct{}{}
		musicIDs = append(musicIDs, row.AliasTypeID)
	}
	sort.Ints(musicIDs)
	if len(musicIDs) > 1 {
		titles, err := s.loadMusicTitles(ctx, musicIDs)
		if err != nil {
			return MusicRef{}, true, err
		}
		return MusicRef{}, true, ambiguousMusicError("别名", musicIDs, titles)
	}
	titles, err := s.loadMusicTitles(ctx, musicIDs)
	if err != nil {
		return MusicRef{}, true, err
	}
	return MusicRef{
		ID:    musicIDs[0],
		Title: titles[musicIDs[0]],
	}, true, nil
}

func uniqueMusicFromRows(rows []*sekaiDB.Music, sourceName string) (MusicRef, error) {
	grouped := make(map[int][]*sekaiDB.Music)
	for _, row := range rows {
		grouped[int(row.GameID)] = append(grouped[int(row.GameID)], row)
	}
	if len(grouped) == 0 {
		return MusicRef{}, fmt.Errorf("未找到对应歌曲")
	}
	musicIDs := make([]int, 0, len(grouped))
	titles := make(map[int]string, len(grouped))
	for musicID, items := range grouped {
		musicIDs = append(musicIDs, musicID)
		titles[musicID] = preferredMusicTitle(items, musicID)
	}
	sort.Ints(musicIDs)
	if len(musicIDs) > 1 {
		return MusicRef{}, ambiguousMusicError(sourceName, musicIDs, titles)
	}
	return MusicRef{
		ID:    musicIDs[0],
		Title: titles[musicIDs[0]],
	}, nil
}

func (s *Service) buildAliasRecordsFromPending(ctx context.Context, rows []*pjskdb.PendingAlias) ([]AliasRecord, error) {
	musicIDs := make([]int, 0, len(rows))
	for _, row := range rows {
		musicIDs = append(musicIDs, row.AliasTypeID)
	}
	titles, err := s.loadMusicTitles(ctx, musicIDs)
	if err != nil {
		return nil, err
	}
	records := make([]AliasRecord, 0, len(rows))
	for _, row := range rows {
		records = append(records, AliasRecord{
			ReviewID: row.ID,
			Music: MusicRef{
				ID:    row.AliasTypeID,
				Title: titles[row.AliasTypeID],
			},
			Alias: row.Alias,
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
		result[musicID] = fmt.Sprintf("歌曲%d", musicID)
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
		return fmt.Sprintf("歌曲%d", musicID)
	}
	return bestTitle
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

func ambiguousMusicError(sourceName string, musicIDs []int, titles map[int]string) error {
	parts := make([]string, 0, len(musicIDs))
	for _, musicID := range musicIDs {
		parts = append(parts, fmt.Sprintf("%d/%s", musicID, titles[musicID]))
	}
	return fmt.Errorf("%s匹配到多首歌曲，请改用歌曲ID：\n%s", sourceName, strings.Join(parts, "\n"))
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
	return fmt.Sprintf("审核ID: %d | 歌曲ID: %d | 曲名: %s | 别名: %s", record.ReviewID, record.Music.ID, record.Music.Title, record.Alias)
}

func formatRejectedAliasRecord(record AliasRecord, reason string) string {
	return formatAliasRecord(record) + "\n原因: " + reason
}

func formatApprovedAliasRecord(record ApprovedAliasRecord) string {
	return fmt.Sprintf("别名ID: %d | 歌曲ID: %d | 曲名: %s | 别名: %s", record.AliasID, record.Music.ID, record.Music.Title, record.Alias)
}
