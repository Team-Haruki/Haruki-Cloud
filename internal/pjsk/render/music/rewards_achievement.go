package music

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"haruki-cloud/internal/pjsk/render/snapshot"
)

func decodeUserMusicAchievements(raw []byte) ([]userMusicAchievement, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, nil
	}

	var direct []userMusicAchievement
	if err := json.Unmarshal(trimmed, &direct); err == nil {
		return direct, nil
	}

	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()

	var payload any
	if err := decoder.Decode(&payload); err != nil {
		return nil, err
	}

	items := collectUserMusicAchievements(payload)
	if len(items) == 0 {
		return nil, fmt.Errorf("unsupported achievements payload shape")
	}
	return items, nil
}

var snapshotAchievementKeys = []string{
	"userMusicAchievements",
	"compactUserMusicAchievements",
}

func resolveSnapshotAchievementsJSON(snapshot snapshot.Snapshot) ([]byte, error) {
	for _, key := range snapshotAchievementKeys {
		value, err := snapshot.RawValue(key)
		if err == nil {
			return value, nil
		}
	}
	return extractNestedAchievementsJSON(snapshot)
}

func extractNestedAchievementsJSON(snapshot snapshot.Snapshot) ([]byte, error) {
	if snapshot == nil {
		return nil, fmt.Errorf("user snapshot is required for music rewards detail")
	}
	raw, err := snapshot.RawBytes()
	if err != nil {
		return nil, err
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()

	var payload any
	if err := decoder.Decode(&payload); err != nil {
		return nil, err
	}

	for _, key := range snapshotAchievementKeys {
		want := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(key), "_", ""), "-", ""))
		if value, ok := findNestedJSONValue(payload, want); ok {
			data, err := json.Marshal(value)
			if err != nil {
				return nil, err
			}
			return data, nil
		}
	}
	return nil, fmt.Errorf("raw user snapshot keys %q are unavailable", strings.Join(snapshotAchievementKeys, ", "))
}

func collectUserMusicAchievements(value any) []userMusicAchievement {
	switch typed := value.(type) {
	case []any:
		out := make([]userMusicAchievement, 0, len(typed))
		for _, item := range typed {
			out = append(out, collectUserMusicAchievements(item)...)
		}
		return out
	case map[string]any:
		if item, ok := parseAchievementItemMap(typed); ok {
			return []userMusicAchievement{item}
		}
		if items, ok := parseAchievementColumnsMap(typed); ok {
			return items
		}
		if items, ok := parseAchievementGroupedMap(typed); ok {
			return items
		}

		out := make([]userMusicAchievement, 0)
		for _, item := range typed {
			out = append(out, collectUserMusicAchievements(item)...)
		}
		return out
	default:
		return nil
	}
}

func findNestedJSONValue(value any, want string) (any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(key), "_", ""), "-", ""))
			if normalized == want {
				return item, true
			}
		}
		for _, item := range typed {
			if found, ok := findNestedJSONValue(item, want); ok {
				return found, true
			}
		}
	case []any:
		for _, item := range typed {
			if found, ok := findNestedJSONValue(item, want); ok {
				return found, true
			}
		}
	}
	return nil, false
}

func parseAchievementItemMap(value map[string]any) (userMusicAchievement, bool) {
	musicID, okMusic := findAchievementInt(value, "musicid")
	achievementID, okAchievement := findAchievementInt(value, "musicachievementid")
	if !okMusic || !okAchievement || musicID <= 0 || achievementID <= 0 {
		return userMusicAchievement{}, false
	}
	return userMusicAchievement{
		MusicID:            musicID,
		MusicAchievementID: achievementID,
	}, true
}

func parseAchievementColumnsMap(value map[string]any) ([]userMusicAchievement, bool) {
	musicIDsRaw, okMusic := findAchievementValue(value, "musicid")
	achievementIDsRaw, okAchievement := findAchievementValue(value, "musicachievementid")
	if !okMusic || !okAchievement {
		return nil, false
	}

	musicIDs, okMusic := toAchievementIntSlice(musicIDsRaw)
	achievementIDs, okAchievement := toAchievementIntSlice(achievementIDsRaw)
	if !okMusic || !okAchievement || len(musicIDs) == 0 || len(musicIDs) != len(achievementIDs) {
		return nil, false
	}

	items := make([]userMusicAchievement, 0, len(musicIDs))
	for idx := range musicIDs {
		if musicIDs[idx] <= 0 || achievementIDs[idx] <= 0 {
			continue
		}
		items = append(items, userMusicAchievement{
			MusicID:            musicIDs[idx],
			MusicAchievementID: achievementIDs[idx],
		})
	}
	return items, len(items) > 0
}

func parseAchievementGroupedMap(value map[string]any) ([]userMusicAchievement, bool) {
	items := make([]userMusicAchievement, 0)
	for key, raw := range value {
		musicID, err := strconv.Atoi(strings.TrimSpace(key))
		if err != nil || musicID <= 0 {
			continue
		}

		achievementIDs, ok := toAchievementIntSlice(raw)
		if !ok {
			continue
		}
		for _, achievementID := range achievementIDs {
			if achievementID <= 0 {
				continue
			}
			items = append(items, userMusicAchievement{
				MusicID:            musicID,
				MusicAchievementID: achievementID,
			})
		}
	}
	return items, len(items) > 0
}

func findAchievementValue(value map[string]any, want string) (any, bool) {
	for key, item := range value {
		normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(key), "_", ""), "-", ""))
		if normalized == want {
			return item, true
		}
	}
	return nil, false
}

func findAchievementInt(value map[string]any, want string) (int, bool) {
	raw, ok := findAchievementValue(value, want)
	if !ok {
		return 0, false
	}
	return toAchievementInt(raw)
}

func toAchievementIntSlice(value any) ([]int, bool) {
	switch typed := value.(type) {
	case []any:
		out := make([]int, 0, len(typed))
		for _, item := range typed {
			intValue, ok := toAchievementInt(item)
			if !ok {
				return nil, false
			}
			out = append(out, intValue)
		}
		return out, true
	default:
		intValue, ok := toAchievementInt(value)
		if !ok {
			return nil, false
		}
		return []int{intValue}, true
	}
}

func toAchievementInt(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			return 0, false
		}
		return int(parsed), true
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}
