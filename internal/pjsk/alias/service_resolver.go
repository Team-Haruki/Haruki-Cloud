package alias

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	aliasdb "haruki-cloud/database/pjsk/alias"
	sekaiDB "haruki-cloud/database/sekai"
	"haruki-cloud/database/sekai/gamecharacter"
	sekaimusic "haruki-cloud/database/sekai/music"
	"haruki-cloud/internal/pjsk/onebot11"
)

// TryResolveMusicID attempts to resolve a token to a music ID.
func (s *Service) TryResolveMusicID(ctx context.Context, token string) (int, bool, error) {
	if !s.IsReady() {
		return 0, false, nil
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return 0, false, nil
	}

	if ref, ok, err := s.tryResolveMusicByID(ctx, token); err != nil {
		return 0, false, err
	} else if ok {
		return ref.ID, true, nil
	}
	if ref, ok, err := s.tryResolveMusicByTitle(ctx, token); err != nil {
		return 0, false, err
	} else if ok {
		return ref.ID, true, nil
	}
	if ref, ok, err := s.tryResolveMusicByApprovedAlias(ctx, token); err != nil {
		return 0, false, err
	} else if ok {
		return ref.ID, true, nil
	}
	return 0, false, nil
}

// TryResolveMusicTitleOrAliasID attempts to resolve a token to a music ID by title or alias.
func (s *Service) TryResolveMusicTitleOrAliasID(ctx context.Context, token string) (int, bool, error) {
	if !s.IsReady() {
		return 0, false, nil
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return 0, false, nil
	}

	if ref, ok, err := s.tryResolveMusicByTitle(ctx, token); err != nil {
		return 0, false, err
	} else if ok {
		return ref.ID, true, nil
	}
	if ref, ok, err := s.tryResolveMusicByApprovedAlias(ctx, token); err != nil {
		return 0, false, err
	} else if ok {
		return ref.ID, true, nil
	}
	return 0, false, nil
}

// TryResolveCharacterID attempts to resolve a token to a character ID.
func (s *Service) TryResolveCharacterID(ctx context.Context, token string) (int, bool, error) {
	if !s.IsReady() {
		return 0, false, nil
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return 0, false, nil
	}

	if ref, ok, err := s.tryResolveCharacterByID(ctx, token); err != nil {
		return 0, false, err
	} else if ok {
		return ref.ID, true, nil
	}
	if ref, ok, err := s.tryResolveCharacterByName(ctx, token); err != nil {
		return 0, false, err
	} else if ok {
		return ref.ID, true, nil
	}
	if ref, ok, err := s.tryResolveCharacterByApprovedAlias(ctx, token); err != nil {
		return 0, false, err
	} else if ok {
		return ref.ID, true, nil
	}
	return 0, false, nil
}

func (s *Service) resolveEntityByToken(ctx context.Context, aliasType, token string) (EntityRef, error) {
	switch aliasType {
	case AliasTypeMusic:
		return s.resolveMusicByToken(ctx, token)
	case AliasTypeCharacter:
		return s.resolveCharacterByToken(ctx, token)
	default:
		return EntityRef{}, onebot11.NewReplayError("不支持的别名类型: %s", aliasType)
	}
}

func (s *Service) resolveMusicByToken(ctx context.Context, token string) (EntityRef, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return EntityRef{}, fmt.Errorf("请输入%s", entityTokenPrompt(AliasTypeMusic))
	}
	if ref, ok, err := s.tryResolveMusicByID(ctx, token); err != nil || ok {
		return ref, err
	}
	if ref, ok, err := s.tryResolveMusicByTitle(ctx, token); err != nil || ok {
		return ref, err
	}
	if ref, ok, err := s.tryResolveMusicByApprovedAlias(ctx, token); err != nil || ok {
		return ref, err
	}
	return EntityRef{}, fmt.Errorf("未找到对应%s，请检查%s", aliasTypeLabel(AliasTypeMusic), entityTokenPrompt(AliasTypeMusic))
}

func (s *Service) resolveCharacterByToken(ctx context.Context, token string) (EntityRef, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return EntityRef{}, fmt.Errorf("请输入%s", entityTokenPrompt(AliasTypeCharacter))
	}
	if ref, ok, err := s.tryResolveCharacterByID(ctx, token); err != nil || ok {
		return ref, err
	}
	if ref, ok, err := s.tryResolveCharacterByName(ctx, token); err != nil || ok {
		return ref, err
	}
	if ref, ok, err := s.tryResolveCharacterByApprovedAlias(ctx, token); err != nil || ok {
		return ref, err
	}
	return EntityRef{}, fmt.Errorf("未找到对应%s，请检查%s", aliasTypeLabel(AliasTypeCharacter), entityTokenPrompt(AliasTypeCharacter))
}

func (s *Service) tryResolveMusicByID(ctx context.Context, token string) (EntityRef, bool, error) {
	id, err := strconv.Atoi(token)
	if err != nil || id <= 0 {
		return EntityRef{}, false, nil
	}
	rows, err := s.sekai.Music.Query().
		Where(sekaimusic.GameIDEQ(int64(id))).
		All(ctx)
	if err != nil {
		return EntityRef{}, true, err
	}
	if len(rows) == 0 {
		return EntityRef{}, true, fmt.Errorf("未找到%s: %d", aliasTypeIDLabel(AliasTypeMusic), id)
	}
	return EntityRef{AliasType: AliasTypeMusic, ID: id, Name: preferredMusicTitle(rows, id)}, true, nil
}

func (s *Service) tryResolveMusicByTitle(ctx context.Context, token string) (EntityRef, bool, error) {
	rows, err := s.sekai.Music.Query().
		Where(sekaimusic.TitleEqualFold(token)).
		All(ctx)
	if err != nil {
		return EntityRef{}, true, err
	}
	if len(rows) == 0 {
		return EntityRef{}, false, nil
	}
	ref, err := uniqueMusicFromRows(rows, aliasTypeNameLabel(AliasTypeMusic))
	return ref, true, err
}

func (s *Service) tryResolveMusicByApprovedAlias(ctx context.Context, token string) (EntityRef, bool, error) {
	rows, err := s.pjsk.Alias.Query().
		Where(
			aliasdb.AliasTypeEQ(AliasTypeMusic),
			aliasdb.AliasEqualFold(token),
		).
		All(ctx)
	if err != nil {
		return EntityRef{}, true, err
	}
	if len(rows) == 0 {
		return EntityRef{}, false, nil
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
			return EntityRef{}, true, err
		}
		return EntityRef{}, true, ambiguousEntityError(AliasTypeMusic, "别名", musicIDs, titles)
	}
	titles, err := s.loadMusicTitles(ctx, musicIDs)
	if err != nil {
		return EntityRef{}, true, err
	}
	return EntityRef{
		AliasType: AliasTypeMusic,
		ID:        musicIDs[0],
		Name:      titles[musicIDs[0]],
	}, true, nil
}

func (s *Service) tryResolveCharacterByID(ctx context.Context, token string) (EntityRef, bool, error) {
	id, err := strconv.Atoi(token)
	if err != nil || id <= 0 {
		return EntityRef{}, false, nil
	}
	rows, err := s.sekai.Gamecharacter.Query().
		Where(gamecharacter.GameIDEQ(int64(id))).
		All(ctx)
	if err != nil {
		return EntityRef{}, true, err
	}
	if len(rows) == 0 {
		return EntityRef{}, true, onebot11.NewReplayError("未找到%s: %d", aliasTypeIDLabel(AliasTypeCharacter), id)
	}
	return EntityRef{AliasType: AliasTypeCharacter, ID: id, Name: preferredCharacterName(rows, id)}, true, nil
}

func (s *Service) tryResolveCharacterByName(ctx context.Context, token string) (EntityRef, bool, error) {
	rows, err := s.sekai.Gamecharacter.Query().
		Where(gamecharacter.GameIDGT(0)).
		All(ctx)
	if err != nil {
		return EntityRef{}, true, err
	}
	target := normalizeCompareText(token)
	grouped := make(map[int][]*sekaiDB.Gamecharacter)
	for _, row := range rows {
		if row.GameID == 0 {
			continue
		}
		if !characterMatchesName(row, target) {
			continue
		}
		grouped[int(row.GameID)] = append(grouped[int(row.GameID)], row)
	}
	if len(grouped) == 0 {
		return EntityRef{}, false, nil
	}
	characterIDs := make([]int, 0, len(grouped))
	names := make(map[int]string, len(grouped))
	for characterID, items := range grouped {
		characterIDs = append(characterIDs, characterID)
		names[characterID] = preferredCharacterName(items, characterID)
	}
	sort.Ints(characterIDs)
	if len(characterIDs) > 1 {
		return EntityRef{}, true, ambiguousEntityError(AliasTypeCharacter, aliasTypeNameLabel(AliasTypeCharacter), characterIDs, names)
	}
	return EntityRef{
		AliasType: AliasTypeCharacter,
		ID:        characterIDs[0],
		Name:      names[characterIDs[0]],
	}, true, nil
}

func (s *Service) tryResolveCharacterByApprovedAlias(ctx context.Context, token string) (EntityRef, bool, error) {
	rows, err := s.pjsk.Alias.Query().
		Where(
			aliasdb.AliasTypeEQ(AliasTypeCharacter),
			aliasdb.AliasEqualFold(token),
		).
		All(ctx)
	if err != nil {
		return EntityRef{}, true, err
	}
	if len(rows) == 0 {
		return EntityRef{}, false, nil
	}
	characterIDs := make([]int, 0, len(rows))
	seen := make(map[int]struct{}, len(rows))
	for _, row := range rows {
		if _, ok := seen[row.AliasTypeID]; ok {
			continue
		}
		seen[row.AliasTypeID] = struct{}{}
		characterIDs = append(characterIDs, row.AliasTypeID)
	}
	sort.Ints(characterIDs)
	if len(characterIDs) > 1 {
		names, err := s.loadCharacterNames(ctx, characterIDs)
		if err != nil {
			return EntityRef{}, true, err
		}
		return EntityRef{}, true, ambiguousEntityError(AliasTypeCharacter, "别名", characterIDs, names)
	}
	names, err := s.loadCharacterNames(ctx, characterIDs)
	if err != nil {
		return EntityRef{}, true, err
	}
	return EntityRef{
		AliasType: AliasTypeCharacter,
		ID:        characterIDs[0],
		Name:      names[characterIDs[0]],
	}, true, nil
}

func uniqueMusicFromRows(rows []*sekaiDB.Music, sourceName string) (EntityRef, error) {
	grouped := make(map[int][]*sekaiDB.Music)
	for _, row := range rows {
		grouped[int(row.GameID)] = append(grouped[int(row.GameID)], row)
	}
	if len(grouped) == 0 {
		return EntityRef{}, fmt.Errorf("未找到对应%s", aliasTypeLabel(AliasTypeMusic))
	}
	musicIDs := make([]int, 0, len(grouped))
	titles := make(map[int]string, len(grouped))
	for musicID, items := range grouped {
		musicIDs = append(musicIDs, musicID)
		titles[musicID] = preferredMusicTitle(items, musicID)
	}
	sort.Ints(musicIDs)
	if len(musicIDs) > 1 {
		return EntityRef{}, ambiguousEntityError(AliasTypeMusic, sourceName, musicIDs, titles)
	}
	return EntityRef{
		AliasType: AliasTypeMusic,
		ID:        musicIDs[0],
		Name:      titles[musicIDs[0]],
	}, nil
}
