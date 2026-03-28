//go:build cgo && pjsk_deck_cgo

package deck

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"haruki-cloud/internal/pjsk/render/deck/deck_cgo"
)

type nativeLocalEngineProvider struct {
	cfg          RecommendConfig
	mu           sync.Mutex
	recommenders map[string]DeckRecommender
}

func newLocalEngineProvider(cfg RecommendConfig) localEngineProvider {
	return &nativeLocalEngineProvider{
		cfg:          cfg,
		recommenders: make(map[string]DeckRecommender),
	}
}

func (p *nativeLocalEngineProvider) Get(region string) (DeckRecommender, error) {
	if p == nil {
		return nil, fmt.Errorf("deck local engine provider is not initialized")
	}
	region = strings.ToLower(strings.TrimSpace(region))
	if region == "" {
		region = "jp"
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if recommender, ok := p.recommenders[region]; ok && recommender != nil {
		return recommender, nil
	}

	recommender, err := newLocalDeckRecommender(p.cfg, region)
	if err != nil {
		return nil, err
	}
	p.recommenders[region] = recommender
	return recommender, nil
}

type LocalDeckRecommender struct {
	pool        *deck_cgo.Pool
	defaultAlgs []string
	timeout     time.Duration
	region      string
}

func newLocalDeckRecommender(cfg RecommendConfig, region string) (*LocalDeckRecommender, error) {
	masterdataDir, err := resolveDeckMasterdataDir(cfg.MasterdataDir, region)
	if err != nil {
		return nil, err
	}
	if masterdataDir == "" {
		return nil, fmt.Errorf("deck local engine requires local masterdata dir")
	}

	poolSize := cfg.LocalPoolSize
	if poolSize <= 0 {
		poolSize = runtime.NumCPU()
		if poolSize > 4 {
			poolSize = 4
		}
	}

	algs := append([]string(nil), cfg.DefaultAlgs...)
	if len(algs) == 0 {
		algs = []string{"dfs", "sa", "ga"}
	}

	if err := prependDeckLibraryDirs(cfg.LocalLibraryDirs); err != nil {
		return nil, fmt.Errorf("deck local engine: set library dirs: %w", err)
	}

	if staticDataDir := resolveDeckStaticDataDir(cfg.StaticDataDir, cfg.MasterdataDir); staticDataDir != "" {
		if err := deck_cgo.SetStaticDataDir(staticDataDir); err != nil {
			return nil, fmt.Errorf("deck local engine: set static data dir: %w", err)
		}
	}

	pool, err := deck_cgo.NewPool(
		masterdataDir,
		nil,
		"",
		nil,
		region,
		poolSize,
	)
	if err != nil {
		return nil, fmt.Errorf("deck local engine: init pool for region %s: %w", region, err)
	}

	return &LocalDeckRecommender{
		pool:        pool,
		defaultAlgs: algs,
		timeout:     cfg.Timeout,
		region:      region,
	}, nil
}

func prependDeckLibraryDirs(configured []string) error {
	var dirs []string
	appendDir := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		dirs = append(dirs, path)
	}

	for _, path := range configured {
		appendDir(path)
	}

	if len(dirs) == 0 {
		return nil
	}

	envKey := "LD_LIBRARY_PATH"
	sep := ":"
	switch runtime.GOOS {
	case "windows":
		envKey = "PATH"
		sep = string(os.PathListSeparator)
	case "darwin":
		envKey = "DYLD_LIBRARY_PATH"
	}

	current := os.Getenv(envKey)
	parts := make([]string, 0, len(dirs)+1)
	seen := make(map[string]struct{})
	for _, dir := range dirs {
		clean := filepath.Clean(dir)
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		parts = append(parts, clean)
	}
	if strings.TrimSpace(current) != "" {
		for _, part := range strings.Split(current, sep) {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			clean := filepath.Clean(part)
			if _, ok := seen[clean]; ok {
				continue
			}
			seen[clean] = struct{}{}
			parts = append(parts, clean)
		}
	}

	return os.Setenv(envKey, strings.Join(parts, sep))
}

func (l *LocalDeckRecommender) Enabled() bool {
	return l != nil && l.pool != nil
}

func (l *LocalDeckRecommender) Close() {
	if l != nil && l.pool != nil {
		l.pool.Close()
	}
}

func (l *LocalDeckRecommender) ExpandAlgorithms(option map[string]interface{}) []map[string]interface{} {
	if option == nil {
		return nil
	}
	alg, _ := option["algorithm"].(string)
	alg = strings.ToLower(strings.TrimSpace(alg))
	if alg != "all" {
		return []map[string]interface{}{option}
	}
	result := make([]map[string]interface{}, 0, len(l.defaultAlgs))
	for _, a := range l.defaultAlgs {
		copied := make(map[string]interface{}, len(option))
		for k, v := range option {
			copied[k] = v
		}
		copied["algorithm"] = a
		result = append(result, copied)
	}
	return result
}

func (l *LocalDeckRecommender) Recommend(req RecommendRequest) (*RecommendResult, error) {
	if len(req.BatchOption) == 0 {
		return nil, fmt.Errorf("deck local engine requires batch_options")
	}
	if len(req.UserData) == 0 {
		return nil, fmt.Errorf("deck local engine requires user_data bytes")
	}

	type partial struct {
		alg   string
		decks []RecommendDeck
		cost  float64
		err   error
	}

	results := make(chan partial, len(req.BatchOption))
	for _, option := range req.BatchOption {
		opt := option
		go func() {
			alg, _ := opt["algorithm"].(string)
			start := time.Now()

			cgoOpts := mapOptionsToCgo(opt, req.Region)
			var cgoResult *deck_cgo.Result
			var err error

			poolErr := l.pool.Do(func(eng *deck_cgo.Engine) error {
				if len(req.MusicMeta) > 0 {
					if err := eng.UpdateMusicmetasFromBytes(req.MusicMeta, req.Region); err != nil {
						return err
					}
				}
				cgoResult, err = eng.Recommend(cgoOpts, req.UserData)
				return err
			})
			if poolErr != nil {
				results <- partial{alg: alg, err: poolErr}
				return
			}

			results <- partial{
				alg:   alg,
				decks: convertCgoDecks(cgoResult.Decks),
				cost:  time.Since(start).Seconds(),
			}
		}()
	}

	agg := &RecommendResult{
		CostTimes: make(map[string]float64),
		WaitTimes: make(map[string]float64),
	}
	seen := make(map[string]*RecommendDeck)
	var order []string

	for range req.BatchOption {
		p := <-results
		if p.err != nil {
			slog.Warn("deck local engine failed", "region", req.Region, "algorithm", p.alg, "error", p.err)
			continue
		}
		if p.alg != "" {
			agg.CostTimes[p.alg] = p.cost
			agg.WaitTimes[p.alg] = 0
		}
		for _, deck := range p.decks {
			h := deckHash(deck)
			if existing, ok := seen[h]; ok {
				if p.alg != "" {
					existing.Algs = append(existing.Algs, p.alg)
				}
				continue
			}
			deckCopy := deck
			if p.alg != "" {
				deckCopy.Algs = []string{p.alg}
			}
			seen[h] = &deckCopy
			order = append(order, h)
		}
	}

	type pair struct {
		Deck RecommendDeck
		Alg  string
	}
	var pairs []pair
	for _, h := range order {
		deck := seen[h]
		algsMap := make(map[string]struct{})
		for _, a := range deck.Algs {
			algsMap[a] = struct{}{}
		}
		var algs []string
		for alg := range algsMap {
			algs = append(algs, alg)
		}
		sort.Strings(algs)
		pairs = append(pairs, pair{Deck: *deck, Alg: strings.Join(algs, "+")})
	}

	liveType, _ := req.BatchOption[0]["live_type"].(string)
	target, _ := req.BatchOption[0]["target"].(string)
	sort.SliceStable(pairs, func(i, j int) bool {
		d1 := pairs[i].Deck
		d2 := pairs[j].Deck
		if liveType == "mysekai" {
			if d1.MysekaiEventPoint != d2.MysekaiEventPoint {
				return d1.MysekaiEventPoint > d2.MysekaiEventPoint
			}
			return d1.TotalPower > d2.TotalPower
		}
		if target == "power" {
			return d1.TotalPower > d2.TotalPower
		}
		if target == "skill" {
			return d1.MultiLiveScoreUp > d2.MultiLiveScoreUp
		}
		if target == "bonus" {
			if d1.EventBonusRate != d2.EventBonusRate {
				return d1.EventBonusRate < d2.EventBonusRate
			}
			if d1.Score != d2.Score {
				return d1.Score > d2.Score
			}
			return d1.MultiLiveScoreUp > d2.MultiLiveScoreUp
		}
		if d1.Score != d2.Score {
			return d1.Score > d2.Score
		}
		return d1.MultiLiveScoreUp > d2.MultiLiveScoreUp
	})

	limitFloat, _ := req.BatchOption[0]["limit"].(float64)
	limitInt, ok := req.BatchOption[0]["limit"].(int)
	if !ok {
		limitInt = int(limitFloat)
	}
	if limitInt <= 0 {
		limitInt = len(pairs)
	}
	if limitInt > len(pairs) {
		limitInt = len(pairs)
	}

	for i := 0; i < limitInt; i++ {
		agg.Decks = append(agg.Decks, pairs[i].Deck)
		agg.DeckAlgs = append(agg.DeckAlgs, pairs[i].Alg)
	}

	return agg, nil
}

func mapOptionsToCgo(opt map[string]interface{}, region string) deck_cgo.Options {
	get := func(key string) interface{} { return opt[key] }
	str := func(key string) string {
		v, _ := get(key).(string)
		return v
	}
	intPtr := func(key string) *int {
		val := get(key)
		if val == nil {
			return nil
		}
		switch v := val.(type) {
		case int:
			return &v
		case float64:
			n := int(v)
			return &n
		case float32:
			n := int(v)
			return &n
		}
		return nil
	}
	floatPtr := func(key string) *float64 {
		val := get(key)
		if val == nil {
			return nil
		}
		switch v := val.(type) {
		case float64:
			return &v
		case float32:
			n := float64(v)
			return &n
		case int:
			n := float64(v)
			return &n
		}
		return nil
	}
	boolPtr := func(key string) *bool {
		v, ok := get(key).(bool)
		if !ok {
			return nil
		}
		return &v
	}

	limitInt := 0
	if l := intPtr("limit"); l != nil {
		limitInt = *l
	}

	keepAfterTrainingState, _ := get("keep_after_training_state").(bool)
	o := deck_cgo.Options{
		Region:                       strings.ToLower(strings.TrimSpace(region)),
		Algorithm:                    str("algorithm"),
		Target:                       str("target"),
		LiveType:                     str("live_type"),
		MusicDiff:                    str("music_diff"),
		EventAttr:                    str("event_attr"),
		EventUnit:                    str("event_unit"),
		EventType:                    str("event_type"),
		SkillOrderChooseStrategy:     str("skill_order_choose_strategy"),
		SkillReferenceChooseStrategy: str("skill_reference_choose_strategy"),
		EventID:                      intPtr("event_id"),
		WorldBloomCharacterID:        intPtr("world_bloom_character_id"),
		WorldBloomEventTurn:          intPtr("world_bloom_event_turn"),
		ChallengeLiveCharacterID:     intPtr("challenge_live_character_id"),
		TimeoutMs:                    intPtr("timeout_ms"),
		MultiLiveTeammatePower:       intPtr("multi_live_teammate_power"),
		MultiLiveTeammateScoreUp:     intPtr("multi_live_teammate_score_up"),
		MultiLiveScoreUpLowerBound:   floatPtr("multi_live_score_up_lower_bound"),
		KeepAfterTrainingState:       keepAfterTrainingState,
		BestSkillAsLeader:            boolPtr("best_skill_as_leader"),
		Limit:                        limitInt,
	}

	parseCardConfig := func(key string) *deck_cgo.CardConfig {
		val, ok := opt[key].(map[string]interface{})
		if !ok {
			return nil
		}
		b := func(k string) bool {
			v, _ := val[k].(bool)
			return v
		}
		return &deck_cgo.CardConfig{
			Disable:     b("disable"),
			LevelMax:    b("level_max"),
			EpisodeRead: b("episode_read"),
			MasterMax:   b("master_max"),
			SkillMax:    b("skill_max"),
			Canvas:      b("canvas"),
		}
	}

	o.Rarity1Config = parseCardConfig("rarity_1_config")
	o.Rarity2Config = parseCardConfig("rarity_2_config")
	o.Rarity3Config = parseCardConfig("rarity_3_config")
	o.Rarity4Config = parseCardConfig("rarity_4_config")
	o.RarityBDConfig = parseCardConfig("rarity_birthday_config")

	if rawCfgs, ok := opt["single_card_configs"].([]interface{}); ok {
		for _, raw := range rawCfgs {
			m, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			b := func(k string) bool {
				v, _ := m[k].(bool)
				return v
			}
			cardID := 0
			switch id := m["card_id"].(type) {
			case float64:
				cardID = int(id)
			case int:
				cardID = id
			}
			o.SingleCardCfgs = append(o.SingleCardCfgs, deck_cgo.SingleCardConfig{
				CardID:      cardID,
				Disable:     b("disable"),
				LevelMax:    b("level_max"),
				EpisodeRead: b("episode_read"),
				MasterMax:   b("master_max"),
				SkillMax:    b("skill_max"),
				Canvas:      b("canvas"),
			})
		}
	}

	if mid := intPtr("music_id"); mid != nil {
		o.MusicID = *mid
	}

	parseArray := func(key string, target *[]int) {
		val := opt[key]
		switch v := val.(type) {
		case []int:
			*target = append(*target, v...)
		case []interface{}:
			for _, item := range v {
				switch num := item.(type) {
				case int:
					*target = append(*target, num)
				case float64:
					*target = append(*target, int(num))
				}
			}
		}
	}

	parseArray("fixed_cards", &o.FixedCards)
	parseArray("fixed_characters", &o.FixedCharacters)
	parseArray("target_bonus_list", &o.TargetBonusList)

	return o
}

func convertCgoDecks(src []deck_cgo.ResultDeck) []RecommendDeck {
	out := make([]RecommendDeck, 0, len(src))
	for _, d := range src {
		cards := make([]RecommendCard, 0, len(d.Cards))
		for _, c := range d.Cards {
			cards = append(cards, RecommendCard{
				CardID:          c.CardID,
				Level:           c.Level,
				MasterRank:      c.MasterRank,
				DefaultImage:    c.DefaultImage,
				SkillLevel:      c.SkillLevel,
				SkillRate:       float64(c.SkillScoreUp),
				EventBonusRate:  c.EventBonusRate,
				IsAfterStory:    c.Episode2Read,
				IsBeforeStory:   c.Episode1Read,
				IsAfterTraining: c.AfterTraining,
				HasCanvasBonus:  c.HasCanvasBonus,
			})
		}
		out = append(out, RecommendDeck{
			Cards:                cards,
			Score:                d.Score,
			LiveScore:            d.LiveScore,
			MysekaiEventPoint:    d.MysekaiEventPoint,
			TotalPower:           d.TotalPower,
			EventBonusRate:       d.EventBonusRate,
			SupportDeckBonusRate: d.SupportDeckBonusRate,
			MultiLiveScoreUp:     d.MultiLiveScoreUp,
			ChallengeScoreDelta:  d.ChallengeScoreDelta,
		})
	}
	return out
}
