package chardata

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	sekai "haruki-cloud/database/sekai"
	"haruki-cloud/database/sekai/gamecharacter"
	"haruki-cloud/database/sekai/gamecharacterunit"
)

// CharacterNickname maps a nickname string to a character game ID.
type CharacterNickname struct {
	Nickname    string
	CharacterID int
}

// Loader 从 Haruki-Cloud 数据库加载角色昵称，供 Extractor 使用。
// 仅访问 game_characters 和 game_character_units 表。
type Loader struct {
	client *sekai.Client
	region string
	logger *slog.Logger

	mu        sync.RWMutex
	nicknames map[string]int // nickname -> character game_id
	loaded    bool
}

func NewLoader(client *sekai.Client, region string, logger *slog.Logger) *Loader {
	if region == "" {
		region = "jp"
	}
	return &Loader{
		client:    client,
		region:    strings.ToLower(strings.TrimSpace(region)),
		logger:    logger,
		nicknames: make(map[string]int),
	}
}

// Load 从数据库加载角色昵称映射表。
// 昵称映射规则与 Haruki-Service-API 原版保持一致：
//   - firstName (小写)
//   - unit 名 (小写)
//   - "firstName + unit短名" 组合
func (l *Loader) Load(ctx context.Context) error {
	if l.client == nil {
		return fmt.Errorf("chardata: no database client configured")
	}

	characters, err := l.client.Gamecharacter.
		Query().
		Where(gamecharacter.ServerRegionEQ(l.region)).
		All(ctx)
	if err != nil {
		return fmt.Errorf("chardata: query game_characters failed: %w", err)
	}

	units, err := l.client.Gamecharacterunit.
		Query().
		Where(gamecharacterunit.ServerRegionEQ(l.region)).
		All(ctx)
	if err != nil {
		return fmt.Errorf("chardata: query game_character_units failed: %w", err)
	}

	// Build unit map: characterGameID -> unit name
	charUnit := make(map[int64]string)
	for _, u := range units {
		charUnit[u.GameCharacterID] = u.Unit
	}

	nicknames := make(map[string]int, len(characters)*3)
	for _, c := range characters {
		id := int(c.GameID)
		firstName := strings.ToLower(strings.TrimSpace(c.FirstName))
		givenName := strings.ToLower(strings.TrimSpace(c.GivenName))
		fullName := firstName + givenName

		if firstName != "" {
			nicknames[firstName] = id
		}
		if givenName != "" {
			nicknames[givenName] = id
		}
		if fullName != "" && fullName != firstName && fullName != givenName {
			nicknames[fullName] = id
		}
		// Also add unit-based nickname if available
		if unit, ok := charUnit[c.GameID]; ok && unit != "" {
			unitKey := strings.ToLower(strings.TrimSpace(unit))
			if unitKey != "" {
				// Only set if not conflicting with another character already in map
				if _, exists := nicknames[unitKey]; !exists {
					nicknames[unitKey] = id
				}
			}
		}
	}

	l.mu.Lock()
	l.nicknames = nicknames
	l.loaded = true
	l.mu.Unlock()

	if l.logger != nil {
		l.logger.Info("chardata: loaded character nicknames", "count", len(characters), "nickname_entries", len(nicknames))
	}
	return nil
}

// Nicknames returns a snapshot of the current nickname→characterID map.
func (l *Loader) Nicknames() map[string]int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if !l.loaded {
		return nil
	}
	// Return a copy to avoid external mutation
	result := make(map[string]int, len(l.nicknames))
	for k, v := range l.nicknames {
		result[k] = v
	}
	return result
}

// StartBackgroundRefresh starts a goroutine that periodically reloads nicknames
// from the database. Call this after the first Load() succeeds.
func (l *Loader) StartBackgroundRefresh(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := l.Load(ctx); err != nil {
					if l.logger != nil {
						l.logger.Warn("chardata: background refresh failed", "error", err)
					}
				}
			}
		}
	}()
}
