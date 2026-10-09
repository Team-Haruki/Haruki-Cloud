package alias

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	sekaiDB "haruki-cloud/database/sekai"
	"haruki-cloud/database/sekai/gamecharacter"
	sekaimusic "haruki-cloud/database/sekai/music"
	"haruki-cloud/internal/i18n"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/utils/usererror"
)

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
		result[musicID] = fmt.Sprintf("%s%d", aliasTypeLabel(PjskAliasTypeMusic), musicID)
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
		result[characterID] = fmt.Sprintf("%s%d", aliasTypeLabel(PjskAliasTypeCharacter), characterID)
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
		return fmt.Sprintf("%s%d", aliasTypeLabel(PjskAliasTypeMusic), musicID)
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
		return fmt.Sprintf("%s%d", aliasTypeLabel(PjskAliasTypeCharacter), characterID)
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
	switch renderregion.Normalize(region) {
	case renderregion.JP:
		return 0
	case renderregion.CN:
		return 1
	case renderregion.TW:
		return 2
	case renderregion.KR:
		return 3
	case renderregion.EN:
		return 4
	default:
		return 5
	}
}

// AmbiguousError reports that an alias or name matches several songs or
// characters. It carries the candidate IDs (AmbiguousIDs) and unwraps to the
// typed user reply listing them.
type AmbiguousError struct {
	AliasType string
	IDs       []int
	names     map[int]string
}

func (e *AmbiguousError) Error() string { return e.userError().Error() }

// Unwrap exposes the typed user reply.
func (e *AmbiguousError) Unwrap() error { return e.userError() }

// AmbiguousIDs lists the candidate IDs.
func (e *AmbiguousError) AmbiguousIDs() []int { return e.IDs }

func (e *AmbiguousError) userError() *usererror.Error {
	if e.AliasType == PjskAliasTypeMusic {
		lines := make([]i18n.Message, 0, len(e.IDs))
		for _, id := range e.IDs {
			lines = append(lines, i18n.M("music.ambiguous_candidate", i18n.Data{"ID": id, "Title": e.names[id]}))
		}
		return usererror.New(usererror.CodeAmbiguous, i18n.M("music.ambiguous", i18n.Data{"Candidates": lines}))
	}
	lines := make([]i18n.Message, 0, len(e.IDs))
	for _, id := range e.IDs {
		lines = append(lines, i18n.M("alias.ambiguous_candidate", i18n.Data{"ID": id, "Name": e.names[id]}))
	}
	return usererror.New(usererror.CodeAmbiguous, i18n.M("alias.ambiguous", i18n.Data{"Kind": aliasKind(e.AliasType), "Candidates": lines}))
}

func ambiguousEntityError(aliasType, sourceName string, ids []int, names map[int]string) error {
	return &AmbiguousError{AliasType: aliasType, IDs: ids, names: names}
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
			return nil, usererror.Invalid(i18n.M("alias.duplicate_in_request", i18n.Data{"First": previous, "Second": aliasText}))
		}
		seen[key] = aliasText
		cleaned = append(cleaned, aliasText)
	}
	if len(cleaned) == 0 {
		return nil, usererror.Misuse(i18n.M("alias.aliases_required"))
	}
	return cleaned, nil
}

func normalizeReviewIDs(reviewIDs []int64) ([]int64, error) {
	if len(reviewIDs) == 0 {
		return nil, usererror.Misuse(i18n.M("alias.review_ids_required"))
	}
	result := make([]int64, 0, len(reviewIDs))
	seen := make(map[int64]struct{}, len(reviewIDs))
	for _, reviewID := range reviewIDs {
		if reviewID <= 0 {
			return nil, usererror.Invalid(i18n.M("alias.review_id_positive"))
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
	case PjskAliasTypeMusic, PjskAliasTypeCharacter:
		return aliasType, nil
	default:
		return "", usererror.Wrap(usererror.CodeInternal, i18n.M("alias.type_unsupported"), fmt.Errorf("unsupported alias type %q", aliasType))
	}
}

// aliasKind is the catalog name of an alias type, for error replies.
func aliasKind(aliasType string) i18n.Message {
	if aliasType == PjskAliasTypeCharacter {
		return i18n.M("alias.kind.character")
	}
	return i18n.M("alias.kind.music")
}

// aliasNameKind is the catalog name of the entity name an alias may not
// repeat, for error replies.
func aliasNameKind(aliasType string) i18n.Message {
	if aliasType == PjskAliasTypeCharacter {
		return i18n.M("alias.name_kind.character")
	}
	return i18n.M("alias.name_kind.music")
}

func errAliasUnavailable() error {
	return usererror.Unavailable(i18n.M("alias.feature"), errors.New("alias service is not ready"))
}

func aliasTypeLabel(aliasType string) string {
	switch aliasType {
	case PjskAliasTypeMusic:
		return "歌曲"
	case PjskAliasTypeCharacter:
		return "角色"
	default:
		return "未知类型"
	}
}

func aliasTypeIDLabel(aliasType string) string {
	switch aliasType {
	case PjskAliasTypeMusic:
		return "歌曲ID"
	case PjskAliasTypeCharacter:
		return "角色ID"
	default:
		return "目标ID"
	}
}

func aliasTypeNameLabel(aliasType string) string {
	switch aliasType {
	case PjskAliasTypeMusic:
		return "曲名"
	case PjskAliasTypeCharacter:
		return "角色名"
	default:
		return "名称"
	}
}

func entityTokenPrompt(aliasType string) string {
	switch aliasType {
	case PjskAliasTypeMusic:
		return "歌曲ID、曲名或已审核别名"
	case PjskAliasTypeCharacter:
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

func shouldTryPartialMusicAlias(token string) bool {
	token = normalizeCompareText(token)
	if token == "" {
		return false
	}
	return utf8.RuneCountInString(token) >= 2
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

func formatAliasRecord(record PjskAliasRecord) string {
	return fmt.Sprintf("审核ID: %d | 类型: %s | 目标ID: %d | 名称: %s | 别名: %s", record.ReviewID, aliasTypeLabel(record.Entity.AliasType), record.Entity.ID, record.Entity.Name, record.Alias)
}

func formatRejectedAliasRecord(record PjskAliasRecord, reason string) string {
	return formatAliasRecord(record) + "\n原因: " + reason
}

func formatApprovedAliasRecord(record ApprovedAliasRecord) string {
	return fmt.Sprintf("别名ID: %d | 类型: %s | 目标ID: %d | 名称: %s | 别名: %s", record.AliasID, aliasTypeLabel(record.Entity.AliasType), record.Entity.ID, record.Entity.Name, record.Alias)
}
