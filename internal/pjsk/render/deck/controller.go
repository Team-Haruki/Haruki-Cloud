package deck

import (
	"context"
	"fmt"
	json "haruki-cloud/internal/jsonutil"
	"slices"
	"strconv"
	"strings"
	"time"

	"haruki-cloud/internal/core/upstream"
	"haruki-cloud/internal/observability/commandtrace"
	"haruki-cloud/internal/pjsk/drawing"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	rendersnapshot "haruki-cloud/internal/pjsk/render/snapshot"
	regionsource "haruki-cloud/internal/pjsk/render/source"
)

func NewController(cards CardSource, events EventSource, drawingClient *drawing.HarukiDrawingClient, assetHelper *assets.AssetHelper, snapshot rendersnapshot.Snapshot, defaultRegion renderregion.Value) *Controller {
	return NewControllerWithConfig(cards, events, drawingClient, assetHelper, snapshot, defaultRegion, RecommendConfig{}, nil)
}

func NewControllerWithConfig(cards CardSource, events EventSource, drawingClient *drawing.HarukiDrawingClient, assetHelper *assets.AssetHelper, snapshot rendersnapshot.Snapshot, defaultRegion renderregion.Value, cfg RecommendConfig, metaLoader MusicMetaSource) *Controller {
	if assetHelper == nil {
		assetHelper = assets.NewAssetHelper("", nil)
	}
	resolvedDefaultRegion := renderregion.WithDefault(defaultRegion)
	controller := &Controller{
		cardSources:   regionsource.NewRegistry[CardSource](resolvedDefaultRegion),
		eventSources:  regionsource.NewRegistry[EventSource](resolvedDefaultRegion),
		musicSources:  regionsource.NewRegistry[MusicSource](resolvedDefaultRegion),
		drawing:       drawingClient,
		assets:        assetHelper,
		snapshot:      snapshot,
		defaultRegion: resolvedDefaultRegion,
		recommendCfg: RecommendConfig{
			Enabled:                   cfg.Enabled,
			ServiceBaseURL:            strings.TrimSpace(cfg.ServiceBaseURL),
			Targets:                   slices.Clone(cfg.Targets),
			SharedResources:           cfg.SharedResources,
			MasterdataDir:             cfg.MasterdataDir,
			MasterdataRefreshInterval: cfg.MasterdataRefreshInterval,
			Timeout:                   cfg.Timeout,
			MaxRetries:                cfg.MaxRetries,
			RetryWaitTime:             cfg.RetryWaitTime,
			DefaultAlgs:               slices.Clone(cfg.DefaultAlgs),
		},
		metaLoader: metaLoader,
	}
	controller.RegisterCardSource(cards)
	controller.RegisterEventSource(events)
	if cfg.Enabled && len(upstream.ResolveTargets(controller.recommendCfg.ServiceBaseURL, controller.recommendCfg.Targets, "deck-service")) > 0 {
		controller.engine = newRemoteEngineProvider(controller.recommendCfg)
	}
	return controller
}

func (c *Controller) RegisterCardSource(source CardSource) {
	if c == nil {
		return
	}
	if c.cardSources == nil {
		c.cardSources = regionsource.NewRegistry[CardSource](c.defaultRegion)
	}
	c.cardSources.RegisterSource(source)
}

func (c *Controller) RegisterEventSource(source EventSource) {
	if c == nil {
		return
	}
	if c.eventSources == nil {
		c.eventSources = regionsource.NewRegistry[EventSource](c.defaultRegion)
	}
	c.eventSources.RegisterSource(source)
}

func (c *Controller) RegisterMusicSource(source MusicSource) {
	if c == nil {
		return
	}
	if c.musicSources == nil {
		c.musicSources = regionsource.NewRegistry[MusicSource](c.defaultRegion)
	}
	c.musicSources.RegisterSource(source)
}

// WithSnapshot returns a shallow copy of this Controller that uses the given
// snapshot instead of the one configured at construction time. This is used by
// the bridge layer to inject a live Toolbox snapshot on a per-request basis.
func (c *Controller) WithSnapshot(s rendersnapshot.Snapshot) *Controller {
	if c == nil {
		return nil
	}
	clone := *c
	clone.snapshot = s
	return &clone
}

func (c *Controller) WithContext(ctx context.Context) *Controller {
	if c == nil {
		return nil
	}
	ctx = normalizeRecommendContext(ctx)
	clone := *c
	clone.ctx = ctx
	clone.drawing = c.drawing.WithContext(ctx)
	clone.assets = c.assets.WithContext(ctx)
	clone.cardSources = c.cardSourcesWithContext(ctx)
	clone.eventSources = c.eventSourcesWithContext(ctx)
	clone.musicSources = c.musicSourcesWithContext(ctx)
	return &clone
}

func (c *Controller) cardSourcesWithContext(ctx context.Context) *regionsource.Registry[CardSource] {
	if c.cardSources != nil {
		result := regionsource.NewRegistry[CardSource](c.cardSources.ResolveRegion(renderregion.Unknown))
		for _, source := range c.cardSources.OrderedSources() {
			if contextual, ok := any(source).(contextualCardSource); ok {
				result.RegisterSource(contextual.WithContext(ctx))
				continue
			}
			result.RegisterSource(source)
		}
		return result
	}
	return nil
}

func (c *Controller) eventSourcesWithContext(ctx context.Context) *regionsource.Registry[EventSource] {
	if c.eventSources != nil {
		result := regionsource.NewRegistry[EventSource](c.eventSources.ResolveRegion(renderregion.Unknown))
		for _, source := range c.eventSources.OrderedSources() {
			if contextual, ok := any(source).(contextualEventSource); ok {
				result.RegisterSource(contextual.WithContext(ctx))
				continue
			}
			result.RegisterSource(source)
		}
		return result
	}
	return nil
}

func (c *Controller) musicSourcesWithContext(ctx context.Context) *regionsource.Registry[MusicSource] {
	if c.musicSources != nil {
		result := regionsource.NewRegistry[MusicSource](c.musicSources.ResolveRegion(renderregion.Unknown))
		for _, source := range c.musicSources.OrderedSources() {
			if contextual, ok := any(source).(contextualMusicSource); ok {
				result.RegisterSource(contextual.WithContext(ctx))
				continue
			}
			result.RegisterSource(source)
		}
		return result
	}
	return nil
}

func (c *Controller) contextOrBackground() context.Context {
	if c == nil {
		return context.Background()
	}
	return normalizeRecommendContext(c.ctx)
}

func (c *Controller) BuildRecommendRequest(req drawing.DeckRequest) (*drawing.DeckRequest, error) {
	if strings.TrimSpace(req.Region) == "" {
		return nil, fmt.Errorf("deck request missing region")
	}
	if len(req.DeckData) == 0 {
		return nil, fmt.Errorf("deck request deck_data is empty")
	}
	return &req, nil
}

func (c *Controller) RenderRecommend(req drawing.DeckRequest) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	finishBuild := commandtrace.MeasureOperation(c.contextOrBackground(), "payload.build")
	payload, err := c.BuildRecommendRequest(req)
	finishBuild()
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateDeckRecommendation(payload)
}

func (c *Controller) BuildAutoRecommendRequest(query AutoQuery) (*drawing.DeckRequest, error) {
	if c == nil {
		return nil, fmt.Errorf("deck controller is not initialized")
	}
	if c.cardSources == nil {
		return nil, fmt.Errorf("deck card source is not configured")
	}
	if c.engine == nil {
		return nil, fmt.Errorf("deck recommend service is not configured")
	}
	ctx := c.contextOrBackground()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	active := c
	finishSnapshot := commandtrace.MeasureOperation(ctx, "deck.snapshot_resolve")
	snapshot, err := c.resolveAutoRecommendSnapshot(query)
	finishSnapshot()
	if err != nil {
		return nil, err
	}
	if snapshot != c.snapshot {
		active = c.WithSnapshot(snapshot)
	}
	return active.buildAutoRecommendWithEngine(ctx, query)
}

func (c *Controller) RenderAutoRecommend(query AutoQuery) ([]byte, error) {
	if c == nil || c.drawing == nil {
		return nil, fmt.Errorf("drawing client is not configured")
	}
	payload, err := c.BuildAutoRecommendRequest(query)
	if err != nil {
		return nil, err
	}
	return c.drawing.GenerateDeckRecommendation(payload)
}

func (c *Controller) normalizeAutoQuery(query AutoQuery) (renderregion.Value, string, error) {
	region := renderregion.Normalize(query.Region)
	if region.IsZero() {
		if resolvedRegion, _, err := c.resolveCardSource(renderregion.Unknown); err == nil {
			region = resolvedRegion
		}
	}
	if region.IsZero() {
		region = c.defaultRegion
	}

	recType := strings.ToLower(strings.TrimSpace(query.RecommendType))
	if recType == "" {
		recType = "event"
	}
	switch recType {
	case "event", "challenge", "no_event", "bonus", "mysekai":
		return renderregion.WithDefault(region), recType, nil
	default:
		return renderregion.Unknown, "", fmt.Errorf("unsupported recommend_type: %s", recType)
	}
}

func (c *Controller) recommendTimeoutMs() int {
	if c == nil || c.recommendCfg.Timeout <= 0 {
		return 60000
	}
	return int(c.recommendCfg.Timeout / time.Millisecond)
}

func (c *Controller) resolveAutoRecommendSnapshot(query AutoQuery) (rendersnapshot.Snapshot, error) {
	if c == nil {
		return nil, fmt.Errorf("deck controller is not initialized")
	}
	if shouldUseSyntheticAutoRecommendSnapshot(query) {
		return c.buildSyntheticAutoRecommendSnapshot(query)
	}
	if c.snapshot != nil {
		if err := c.snapshot.Require(); err != nil {
			return nil, err
		}
		return c.snapshot, nil
	}
	if query.UseCurrentDeck {
		return nil, fmt.Errorf("user data is required for deck auto recommend")
	}
	return nil, fmt.Errorf("user data is required for deck auto recommend")
}

func shouldUseSyntheticAutoRecommendSnapshot(query AutoQuery) bool {
	return !query.UseCurrentDeck && (query.MaxProfile || query.SubMaxProfile)
}

func (c *Controller) buildSyntheticAutoRecommendSnapshot(query AutoQuery) (rendersnapshot.Snapshot, error) {
	region, _, err := c.normalizeAutoQuery(query)
	if err != nil {
		return nil, err
	}

	raw := syntheticAutoRecommendRawUserData(query)
	if err := c.applyProfilePreset(region, raw, query); err != nil {
		return nil, err
	}
	data, err := encodeSyntheticAutoRecommendRawUserData(raw)
	if err != nil {
		return nil, fmt.Errorf("encode synthetic user data: %w", err)
	}
	snapshot, err := rendersnapshot.NewFromBytes(nil, c.assets, region, data, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("build synthetic user snapshot: %w", err)
	}
	if err := snapshot.Require(); err != nil {
		return nil, err
	}
	return snapshot, nil
}

func syntheticAutoRecommendRawUserData(query AutoQuery) *rendersnapshot.RawUserData {
	userID := int64(1)
	name := "MaxProfile"
	if profile := query.Profile; profile != nil {
		if parsed, err := strconv.ParseInt(strings.TrimSpace(profile.ID), 10, 64); err == nil && parsed > 0 {
			userID = parsed
		}
		if nickname := strings.TrimSpace(profile.Nickname); nickname != "" {
			name = nickname
		}
	}
	return &rendersnapshot.RawUserData{
		Now: time.Now().UnixMilli(),
		UserGamedata: rendersnapshot.RawUserGamedata{
			UserID: userID,
			Name:   name,
			Deck:   1,
		},
		UserProfile: rendersnapshot.RawUserProfile{
			ProfileImageType: "default",
		},
		UserDecks: []rendersnapshot.RawUserDeck{{
			DeckID: 1,
		}},
		UserCharacters: buildSyntheticUserCharacters(),
		UserAreas:      []rendersnapshot.RawUserArea{},
	}
}

func buildSyntheticUserCharacters() []rendersnapshot.RawUserCharacter {
	characters := make([]rendersnapshot.RawUserCharacter, 0, challengeCharacterCount)
	for characterID := 1; characterID <= challengeCharacterCount; characterID++ {
		characters = append(characters, rendersnapshot.RawUserCharacter{
			CharacterID:   characterID,
			CharacterRank: 120,
		})
	}
	return characters
}

func encodeSyntheticAutoRecommendRawUserData(raw *rendersnapshot.RawUserData) ([]byte, error) {
	if raw == nil {
		return nil, fmt.Errorf("raw user snapshot is unavailable")
	}

	// The deck service treats several suite keys as required even when they are
	// logically empty, so synthetic theoretical profiles must preserve them as
	// empty arrays instead of omitting them via `omitempty`.
	payload := map[string]any{
		"now":                                   raw.Now,
		"userGamedata":                          raw.UserGamedata,
		"userProfile":                           raw.UserProfile,
		"userDecks":                             sliceOrEmpty(raw.UserDecks),
		"userCards":                             sliceOrEmpty(raw.UserCards),
		"userBonds":                             sliceOrEmpty(raw.UserBonds),
		"userMusicResults":                      sliceOrEmpty(raw.UserMusicStats),
		"userMusics":                            sliceOrEmpty(raw.UserMusics),
		"userChallengeLiveSoloDecks":            sliceOrEmpty(raw.UserChallengeLiveSoloDecks),
		"userChallengeLiveSoloResults":          sliceOrEmpty(raw.UserChallengeLiveSoloResults),
		"userChallengeLiveSoloStages":           sliceOrEmpty(raw.UserChallengeLiveSoloStages),
		"userChallengeLiveSoloHighScoreRewards": sliceOrEmpty(raw.UserChallengeLiveSoloHighScoreRewards),
		"userCharacters":                        sliceOrEmpty(raw.UserCharacters),
		"userCharacterMissionV2s":               sliceOrEmpty(raw.UserCharacterMissionV2s),
		"userCharacterLiveUsageCounts":          sliceOrEmpty(raw.UserCharacterLiveUsageCounts),
		"userCharacterMissionV2Statuses":        sliceOrEmpty(raw.UserCharacterMissionV2Statuses),
		"userAreas":                             sliceOrEmpty(raw.UserAreas),
		"userMaterials":                         sliceOrEmpty(raw.UserMaterials),
		"userMysekaiCanvases":                   sliceOrEmpty(raw.UserMysekaiCanvases),
		"userMysekaiGates":                      sliceOrEmpty(raw.UserMysekaiGates),
		"userMysekaiFixtureGameCharacterPerformanceBonuses": sliceOrEmpty(raw.UserMysekaiFixtureGameCharacterPerformanceBonuses),
		"userMusicDifficultyClearCounts":                    sliceOrEmpty(raw.UserMusicClear),
		"userHonors":                                        sliceOrEmpty(raw.UserHonors),
		"userProfileHonors":                                 sliceOrEmpty(raw.UserProfileHonors),
		"userPlayerFrames":                                  sliceOrEmpty(raw.UserFrames),
		"userEvents":                                        sliceOrEmpty(raw.UserEvents),
		"userEventResults":                                  sliceOrEmpty(raw.UserEventResults),
		"userWorldBlooms":                                   sliceOrEmpty(raw.UserWorldBlooms),
	}

	return json.Marshal(payload)
}

func sliceOrEmpty[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
}
