package alias

import (
	"context"

	pjskdb "haruki-cloud/database/pjsk"
)

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
