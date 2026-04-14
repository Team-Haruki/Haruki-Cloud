package provider

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"haruki-cloud/internal/pjsk/render/masterdata"
)

// ---------------------------------------------------------------------------
// localStore – cached file reader
// ---------------------------------------------------------------------------

type localStore struct {
	dirs  []string
	mu    sync.RWMutex
	cache map[string][]byte
}

func newLocalStore(dirs ...string) *localStore {
	cleaned := make([]string, 0, len(dirs))
	seen := make(map[string]struct{}, len(dirs))
	for _, dir := range dirs {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		dir = filepath.Clean(dir)
		if _, ok := seen[dir]; ok {
			continue
		}
		seen[dir] = struct{}{}
		cleaned = append(cleaned, dir)
	}

	return &localStore{
		dirs:  cleaned,
		cache: make(map[string][]byte),
	}
}

func (s *localStore) Configured() bool {
	return s != nil && len(s.dirs) > 0
}

func (s *localStore) readFile(filename string) ([]byte, error) {
	if s == nil || !s.Configured() {
		return nil, fmt.Errorf("read %s: local store is not configured", filename)
	}
	filename = filepath.Clean(filename)

	s.mu.RLock()
	if data, ok := s.cache[filename]; ok {
		s.mu.RUnlock()
		return data, nil
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()

	if data, ok := s.cache[filename]; ok {
		return data, nil
	}

	var (
		data    []byte
		err     error
		readErr error
	)
	for _, dir := range s.dirs {
		data, err = os.ReadFile(filepath.Join(dir, filename))
		if err == nil {
			s.cache[filename] = data
			return data, nil
		}
		if readErr == nil {
			readErr = err
		}
	}
	if readErr == nil {
		readErr = os.ErrNotExist
	}
	return nil, fmt.Errorf("read %s: %w", filename, readErr)
}

// loadJSON reads a JSON file and unmarshals it into a slice of T.
func loadJSON[T any](store *localStore, filename string) ([]T, error) {
	data, err := store.readFile(filename)
	if err != nil {
		return nil, err
	}
	var items []T
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("unmarshal %s: %w", filename, err)
	}
	return items, nil
}

// ---------------------------------------------------------------------------
// JSON wrapper types – for masterdata types that lack JSON tags or have
// field-name mismatches with the camelCase JSON files.
// ---------------------------------------------------------------------------

type localMusicJSON struct {
	ID                 int      `json:"id"`
	Seq                int      `json:"seq"`
	ReleaseConditionID int      `json:"releaseConditionId"`
	Categories         []string `json:"categories"`
	Title              string   `json:"title"`
	Pronunciation      string   `json:"pronunciation"`
	Lyricist           string   `json:"lyricist"`
	Composer           string   `json:"composer"`
	Arranger           string   `json:"arranger"`
	DancerCount        int      `json:"dancerCount"`
	SelfDancerPosition int      `json:"selfDancerPosition"`
	AssetBundleName    string   `json:"assetbundleName"`
	PublishedAt        int64    `json:"publishedAt"`
	ReleasedAt         int64    `json:"releasedAt"`
	IsFullLength       bool     `json:"isFullLength"`
}

func (j *localMusicJSON) toModel() *masterdata.Music {
	return &masterdata.Music{
		ID:                 j.ID,
		Seq:                j.Seq,
		ReleaseConditionID: j.ReleaseConditionID,
		Categories:         j.Categories,
		Title:              j.Title,
		Pronunciation:      j.Pronunciation,
		Lyricist:           j.Lyricist,
		Composer:           j.Composer,
		Arranger:           j.Arranger,
		DancerCount:        j.DancerCount,
		SelfDancerCount:    j.SelfDancerPosition,
		AssetBundleName:    j.AssetBundleName,
		PublishedAt:        j.PublishedAt,
		DigitizedAt:        j.ReleasedAt,
		IsFullLength:       j.IsFullLength,
	}
}

type localEventJSON struct {
	ID                       int             `json:"id"`
	EventType                string          `json:"eventType"`
	Name                     string          `json:"name"`
	AssetBundleName          string          `json:"assetbundleName"`
	StartAt                  int64           `json:"startAt"`
	AggregateAt              int64           `json:"aggregateAt"`
	ClosedAt                 int64           `json:"closedAt"`
	EventRankingRewardRanges json.RawMessage `json:"eventRankingRewardRanges"`
}

func (j *localEventJSON) toModel() *masterdata.Event {
	return &masterdata.Event{
		ID:              j.ID,
		EventType:       j.EventType,
		Name:            j.Name,
		AssetBundleName: j.AssetBundleName,
		StartAt:         j.StartAt,
		AggregateAt:     j.AggregateAt,
		ClosedAt:        j.ClosedAt,
	}
}

type localWorldBloomJSON struct {
	ID                    int    `json:"id"`
	EventID               int    `json:"eventId"`
	GameCharacterID       int    `json:"gameCharacterId"`
	WorldBloomChapterType string `json:"worldBloomChapterType"`
	ChapterNo             int    `json:"chapterNo"`
	ChapterStartAt        int64  `json:"chapterStartAt"`
	AggregateAt           int64  `json:"aggregateAt"`
	ChapterEndAt          int64  `json:"chapterEndAt"`
	IsSupplemental        bool   `json:"isSupplemental"`
}

func (j *localWorldBloomJSON) toModel() *masterdata.WorldBloom {
	wb := &masterdata.WorldBloom{
		ID:             j.ID,
		EventID:        j.EventID,
		ChapterNo:      j.ChapterNo,
		ChapterStartAt: j.ChapterStartAt,
		AggregateAt:    j.AggregateAt,
		ChapterEndAt:   j.ChapterEndAt,
		IsSupplemental: j.IsSupplemental,
		ChapterType:    j.WorldBloomChapterType,
	}
	if j.GameCharacterID != 0 {
		id := j.GameCharacterID
		wb.GameCharacterID = &id
	}
	return wb
}

type localVirtualLiveJSON struct {
	ID                   int             `json:"id"`
	Name                 string          `json:"name"`
	StartAt              int64           `json:"startAt"`
	EndAt                int64           `json:"endAt"`
	VirtualLiveSchedules json.RawMessage `json:"virtualLiveSchedules"`
}

type localCostume3dJSON struct {
	ID                    int    `json:"id"`
	CharacterID           int    `json:"characterId"`
	Name                  string `json:"name"`
	AssetBundleName       string `json:"assetbundleName"`
	LegacyAssetBundleName string `json:"_assetbundleName"`
}

func (j *localCostume3dJSON) toModel() *masterdata.Costume3d {
	assetBundleName := strings.TrimSpace(j.AssetBundleName)
	if assetBundleName == "" {
		assetBundleName = strings.TrimSpace(j.LegacyAssetBundleName)
	}
	return &masterdata.Costume3d{
		ID:              j.ID,
		CharacterID:     j.CharacterID,
		AssetBundleName: assetBundleName,
		Description:     j.Name,
	}
}

type localMusicVocalJSON struct {
	ID              int             `json:"id"`
	MusicID         int             `json:"musicId"`
	MusicVocalType  string          `json:"musicVocalType"`
	Caption         string          `json:"caption"`
	Characters      json.RawMessage `json:"characters"`
	AssetBundleName string          `json:"assetbundleName"`
}

func (j *localMusicVocalJSON) toModel() *masterdata.MusicVocal {
	return &masterdata.MusicVocal{
		ID:              j.ID,
		MusicID:         j.MusicID,
		MusicVocalType:  j.MusicVocalType,
		Caption:         j.Caption,
		Characters:      parseMusicVocalCharactersFromRaw(j.Characters, j.ID, j.MusicID),
		AssetBundleName: j.AssetBundleName,
	}
}

type localCharacterRankJSON struct {
	CharacterID     int     `json:"characterId"`
	CharacterRank   int     `json:"characterRank"`
	Power1BonusRate float64 `json:"power1BonusRate"`
}

type localMysekaiGateLevelJSON struct {
	MysekaiGateID  int     `json:"mysekaiGateId"`
	Level          int     `json:"level"`
	PowerBonusRate float64 `json:"powerBonusRate"`
}

// ---------------------------------------------------------------------------
// Link/join table types – entities without masterdata equivalents.
// ---------------------------------------------------------------------------

type localEventCardJSON struct {
	CardID  int `json:"cardId"`
	EventID int `json:"eventId"`
}

type localEventMusicJSON struct {
	EventID int `json:"eventId"`
	MusicID int `json:"musicId"`
	Seq     int `json:"seq"`
}

type localCardSupplyJSON struct {
	ID             int    `json:"id"`
	CardSupplyType string `json:"cardSupplyType"`
}

type localCardCostume3dJSON struct {
	CardID      int `json:"cardId"`
	Costume3dID int `json:"costume3dId"`
}

type localMusicTagJSON struct {
	MusicID  int    `json:"musicId"`
	MusicTag string `json:"musicTag"`
}

type localOutsideCharacterJSON struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type localGameCharacterJSON struct {
	ID               int    `json:"id"`
	FirstName        string `json:"firstName"`
	GivenName        string `json:"givenName"`
	FirstNameEnglish string `json:"firstNameEnglish"`
	GivenNameEnglish string `json:"givenNameEnglish"`
	Unit             string `json:"unit"`
}

type localShopItemJSON struct {
	ID                 int             `json:"id"`
	ShopID             int             `json:"shopId"`
	Seq                int             `json:"seq"`
	ResourceBoxID      int             `json:"resourceBoxId"`
	ReleaseConditionID int             `json:"releaseConditionId"`
	StartAt            int64           `json:"startAt"`
	Costs              json.RawMessage `json:"costs"`
}
