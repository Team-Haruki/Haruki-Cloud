//go:build cgo && pjsk_deck_cgo

// Package deck_cgo provides a Go interface to the sekai-deck-recommend-cpp
// C++ engine via CGo.
//
// The engine is wrapped through a pure-C shim (deck_c_api.h / deck_c_api.cpp)
// to avoid C++ ABI issues. Options and results are exchanged as JSON strings.
//
// Build requirements:
//   - The sekai_deck_recommend_c shared library must be present in
//     internal/pjsk/render/deck/deck_cgo/lib/<GOOS>_<GOARCH>/ before
//     compiling the Go binary.
//   - See BUILD.md in this package for how to produce the library.
//
// Cross-platform note:
//   CGo directives below select the correct library path per OS/arch.
//   The actual symbol list is identical across platforms; only the lib
//   filename and search path differ.

package deck_cgo

/*
#cgo CFLAGS: -I${SRCDIR} -I${SRCDIR}/vendor/sekai-deck-recommend-cpp/src -I${SRCDIR}/vendor/sekai-deck-recommend-cpp/3rdparty/json/single_include
#cgo CXXFLAGS: -I${SRCDIR} -I${SRCDIR}/vendor/sekai-deck-recommend-cpp/src -I${SRCDIR}/vendor/sekai-deck-recommend-cpp/3rdparty/json/single_include -std=c++20

#cgo windows,amd64  LDFLAGS: -L${SRCDIR}/lib/windows_amd64  -lsekai_deck_recommend_c
#cgo linux,amd64    LDFLAGS: -L${SRCDIR}/lib/linux_amd64    -Wl,-rpath,${SRCDIR}/lib/linux_amd64 -lsekai_deck_recommend_c
#cgo linux,arm64    LDFLAGS: -L${SRCDIR}/lib/linux_arm64    -Wl,-rpath,${SRCDIR}/lib/linux_arm64 -lsekai_deck_recommend_c
#cgo darwin,amd64   LDFLAGS: -L${SRCDIR}/lib/darwin_amd64   -Wl,-rpath,${SRCDIR}/lib/darwin_amd64 -lsekai_deck_recommend_c
#cgo darwin,arm64   LDFLAGS: -L${SRCDIR}/lib/darwin_arm64   -Wl,-rpath,${SRCDIR}/lib/darwin_arm64 -lsekai_deck_recommend_c

#include "deck_c_api.h"
#include <stdlib.h>
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"unsafe"
)

// ─── Option / Result types mirroring Python .pyi ─────────────────────────────

// CardConfig mirrors DeckRecommendCardConfig.
type CardConfig struct {
	Disable     bool `json:"disable"`
	LevelMax    bool `json:"level_max"`
	EpisodeRead bool `json:"episode_read"`
	MasterMax   bool `json:"master_max"`
	SkillMax    bool `json:"skill_max"`
	Canvas      bool `json:"canvas"`
}

// SingleCardConfig mirrors DeckRecommendSingleCardConfig.
type SingleCardConfig struct {
	CardID      int  `json:"card_id"`
	Disable     bool `json:"disable"`
	LevelMax    bool `json:"level_max"`
	EpisodeRead bool `json:"episode_read"`
	MasterMax   bool `json:"master_max"`
	SkillMax    bool `json:"skill_max"`
	Canvas      bool `json:"canvas"`
}

// Options mirrors DeckRecommendOptions (only fields used by Haruki-Cloud).
// Unmapped / optional fields can be added as needed.
type Options struct {
	Region    string `json:"region"`
	LiveType  string `json:"live_type"`
	MusicID   int    `json:"music_id"`
	MusicDiff string `json:"music_diff"`
	EventID   *int   `json:"event_id,omitempty"`
	// Optional event overrides (when EventID is nil)
	EventAttr string `json:"event_attr,omitempty"`
	EventUnit string `json:"event_unit,omitempty"`
	EventType string `json:"event_type,omitempty"`

	WorldBloomCharacterID    *int `json:"world_bloom_character_id,omitempty"`
	WorldBloomEventTurn      *int `json:"world_bloom_event_turn,omitempty"`
	ChallengeLiveCharacterID *int `json:"challenge_live_character_id,omitempty"`

	Algorithm string `json:"algorithm"` // "dfs" | "sa" | "ga"
	Target    string `json:"target"`    // "score" | "power" | "skill" | "bonus"
	Limit     int    `json:"limit,omitempty"`
	TimeoutMs *int   `json:"timeout_ms,omitempty"`

	Rarity1Config  *CardConfig        `json:"rarity_1_config,omitempty"`
	Rarity2Config  *CardConfig        `json:"rarity_2_config,omitempty"`
	Rarity3Config  *CardConfig        `json:"rarity_3_config,omitempty"`
	Rarity4Config  *CardConfig        `json:"rarity_4_config,omitempty"`
	RarityBDConfig *CardConfig        `json:"rarity_birthday_config,omitempty"`
	SingleCardCfgs []SingleCardConfig `json:"single_card_configs,omitempty"`

	FixedCards      []int `json:"fixed_cards,omitempty"`
	FixedCharacters []int `json:"fixed_characters,omitempty"`
	TargetBonusList []int `json:"target_bonus_list,omitempty"`

	MultiLiveTeammatePower     *int     `json:"multi_live_teammate_power,omitempty"`
	MultiLiveTeammateScoreUp   *int     `json:"multi_live_teammate_score_up,omitempty"`
	MultiLiveScoreUpLowerBound *float64 `json:"multi_live_score_up_lower_bound,omitempty"`

	SkillOrderChooseStrategy     string `json:"skill_order_choose_strategy,omitempty"`
	SkillReferenceChooseStrategy string `json:"skill_reference_choose_strategy,omitempty"`
	KeepAfterTrainingState       bool   `json:"keep_after_training_state,omitempty"`
	BestSkillAsLeader            *bool  `json:"best_skill_as_leader,omitempty"`
}

// ResultCard mirrors RecommendCard.
type ResultCard struct {
	CardID            int     `json:"card_id"`
	TotalPower        int     `json:"total_power"`
	BasePower         int     `json:"base_power"`
	EventBonusRate    float64 `json:"event_bonus_rate"`
	MasterRank        int     `json:"master_rank"`
	Level             int     `json:"level"`
	SkillLevel        int     `json:"skill_level"`
	SkillScoreUp      int     `json:"skill_score_up"`
	SkillLifeRecovery int     `json:"skill_life_recovery"`
	Episode1Read      bool    `json:"episode1_read"`
	Episode2Read      bool    `json:"episode2_read"`
	AfterTraining     bool    `json:"after_training"`
	DefaultImage      string  `json:"default_image"`
	HasCanvasBonus    bool    `json:"has_canvas_bonus"`
}

// ResultDeck mirrors RecommendDeck.
type ResultDeck struct {
	Score                int          `json:"score"`
	LiveScore            int          `json:"live_score"`
	MysekaiEventPoint    int          `json:"mysekai_event_point"`
	TotalPower           int          `json:"total_power"`
	BasePower            int          `json:"base_power"`
	AreaItemBonusPower   int          `json:"area_item_bonus_power"`
	CharacterBonusPower  int          `json:"character_bonus_power"`
	HonorBonusPower      int          `json:"honor_bonus_power"`
	FixtureBonusPower    int          `json:"fixture_bonus_power"`
	GateBonusPower       int          `json:"gate_bonus_power"`
	EventBonusRate       float64      `json:"event_bonus_rate"`
	SupportDeckBonusRate float64      `json:"support_deck_bonus_rate"`
	MultiLiveScoreUp     float64      `json:"multi_live_score_up"`
	ChallengeScoreDelta  int          `json:"challenge_score_delta"`
	Cards                []ResultCard `json:"cards"`
}

// Result mirrors DeckRecommendResult.
type Result struct {
	Decks []ResultDeck `json:"decks"`
}

// ─── Engine ───────────────────────────────────────────────────────────────────

// Engine is a Go wrapper around the C++ SekaiDeckRecommend engine.
// It is NOT goroutine-safe; protect shared instances with a mutex or use a
// pool if you need concurrent calls.
type Engine struct {
	handle C.DeckEngineHandle
}

// NewEngine creates a new deck recommendation engine instance.
func NewEngine() (*Engine, error) {
	h := C.deck_engine_create()
	if h == nil {
		return nil, fmt.Errorf("deck_cgo: failed to create engine")
	}
	return &Engine{handle: h}, nil
}

// SetStaticDataDir configures the global static data directory used by the
// C++ engine to resolve bundled extra masterdata files.
func SetStaticDataDir(path string) error {
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))

	if rc := C.deck_set_static_data_dir(cPath); rc != 0 {
		return fmt.Errorf("deck_cgo: set static data dir failed")
	}
	return nil
}

// Close destroys the engine and frees all C++ resources.
func (e *Engine) Close() {
	if e.handle != nil {
		C.deck_engine_destroy(e.handle)
		e.handle = nil
	}
}

// lastError returns the last error string from the C++ engine.
func (e *Engine) lastError() string {
	return C.GoString(C.deck_last_error(e.handle))
}

// UpdateMasterdata loads masterdata from a local directory.
func (e *Engine) UpdateMasterdata(baseDir, region string) error {
	cDir := C.CString(baseDir)
	cReg := C.CString(region)
	defer C.free(unsafe.Pointer(cDir))
	defer C.free(unsafe.Pointer(cReg))

	if rc := C.deck_engine_update_masterdata(e.handle, cDir, cReg); rc != 0 {
		return fmt.Errorf("deck_cgo: update_masterdata: %s", e.lastError())
	}
	return nil
}

// UpdateMasterdataFromStrings loads masterdata from in-memory JSON map.
// The keys match filenames without extension (e.g. "cards", "events").
func (e *Engine) UpdateMasterdataFromStrings(data map[string][]byte, region string) error {
	count := len(data)
	if count == 0 {
		return nil
	}

	keys := make([]*C.char, count)
	vals := make([]*C.uint8_t, count)
	lens := make([]C.size_t, count)

	i := 0
	for k, v := range data {
		keys[i] = C.CString(k)
		vals[i] = (*C.uint8_t)(unsafe.Pointer(&v[0]))
		lens[i] = C.size_t(len(v))
		i++
	}
	defer func() {
		for _, k := range keys {
			C.free(unsafe.Pointer(k))
		}
	}()

	cReg := C.CString(region)
	defer C.free(unsafe.Pointer(cReg))

	rc := C.deck_engine_update_masterdata_from_strings(
		e.handle,
		(**C.char)(unsafe.Pointer(&keys[0])),
		(**C.uint8_t)(unsafe.Pointer(&vals[0])),
		(*C.size_t)(unsafe.Pointer(&lens[0])),
		C.int(count),
		cReg,
	)
	if rc != 0 {
		return fmt.Errorf("deck_cgo: update_masterdata_from_strings: %s", e.lastError())
	}
	return nil
}

// UpdateMusicmetas loads music metadata from a local file.
func (e *Engine) UpdateMusicmetas(filePath, region string) error {
	cPath := C.CString(filePath)
	cReg := C.CString(region)
	defer C.free(unsafe.Pointer(cPath))
	defer C.free(unsafe.Pointer(cReg))

	if rc := C.deck_engine_update_musicmetas(e.handle, cPath, cReg); rc != 0 {
		return fmt.Errorf("deck_cgo: update_musicmetas: %s", e.lastError())
	}
	return nil
}

// UpdateMusicmetasFromBytes loads music metadata from an in-memory JSON buffer.
func (e *Engine) UpdateMusicmetasFromBytes(data []byte, region string) error {
	cReg := C.CString(region)
	defer C.free(unsafe.Pointer(cReg))

	rc := C.deck_engine_update_musicmetas_from_string(
		e.handle,
		(*C.uint8_t)(unsafe.Pointer(&data[0])),
		C.size_t(len(data)),
		cReg,
	)
	if rc != 0 {
		return fmt.Errorf("deck_cgo: update_musicmetas_from_string: %s", e.lastError())
	}
	return nil
}

// Recommend runs deck recommendation with the given options and user data.
// userJSON is the raw bytes of user.json (suite dump); may be nil.
func (e *Engine) Recommend(opts Options, userJSON []byte) (*Result, error) {
	optsBytes, err := json.Marshal(opts)
	if err != nil {
		return nil, fmt.Errorf("deck_cgo: marshal options: %w", err)
	}

	var userPtr *C.uint8_t
	var userLen C.size_t
	if len(userJSON) > 0 {
		userPtr = (*C.uint8_t)(unsafe.Pointer(&userJSON[0]))
		userLen = C.size_t(len(userJSON))
	}

	cResult := C.deck_engine_recommend(
		e.handle,
		(*C.uint8_t)(unsafe.Pointer(&optsBytes[0])),
		C.size_t(len(optsBytes)),
		userPtr,
		userLen,
	)
	if cResult == nil {
		return nil, fmt.Errorf("deck_cgo: recommend: %s", e.lastError())
	}
	defer C.deck_free_string(cResult)

	var result Result
	if err := json.Unmarshal([]byte(C.GoString(cResult)), &result); err != nil {
		return nil, fmt.Errorf("deck_cgo: unmarshal result: %w", err)
	}
	return &result, nil
}
