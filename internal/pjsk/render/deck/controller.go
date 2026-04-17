package deck

import (
	"slices"
	"fmt"
	"strings"
	"time"

	"haruki-cloud/internal/pjsk/render/assets"
	renderregion "haruki-cloud/internal/pjsk/region"
	regionsource "haruki-cloud/internal/pjsk/render/source"
	"haruki-cloud/internal/pjsk/render/snapshot"
	"haruki-cloud/internal/pjsk/drawing"
)

func NewController(cards CardSource, events EventSource, drawingClient *drawing.HarukiDrawingClient, assetHelper *assets.AssetHelper, snapshot snapshot.Snapshot, defaultRegion renderregion.Value) *Controller {
	return NewControllerWithConfig(cards, events, drawingClient, assetHelper, snapshot, defaultRegion, RecommendConfig{}, nil)
}

func NewControllerWithConfig(cards CardSource, events EventSource, drawingClient *drawing.HarukiDrawingClient, assetHelper *assets.AssetHelper, snapshot snapshot.Snapshot, defaultRegion renderregion.Value, cfg RecommendConfig, metaLoader MusicMetaSource) *Controller {
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
			Enabled:        cfg.Enabled,
			ServiceBaseURL: strings.TrimSpace(cfg.ServiceBaseURL),
			MasterdataDir:  cfg.MasterdataDir,
			Timeout:        cfg.Timeout,
			MaxRetries:     cfg.MaxRetries,
			RetryWaitTime:  cfg.RetryWaitTime,
			DefaultAlgs:    slices.Clone(cfg.DefaultAlgs),
		},
		metaLoader: metaLoader,
	}
	controller.RegisterCardSource(cards)
	controller.RegisterEventSource(events)
	if cfg.Enabled && controller.recommendCfg.ServiceBaseURL != "" {
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
func (c *Controller) WithSnapshot(s snapshot.Snapshot) *Controller {
	if c == nil {
		return nil
	}
	clone := *c
	clone.snapshot = s
	return &clone
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
	payload, err := c.BuildRecommendRequest(req)
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
	if c.snapshot == nil {
		return nil, fmt.Errorf("user data is required for deck auto recommend")
	}
	if err := c.snapshot.Require(); err != nil {
		return nil, err
	}
	if c.engine == nil {
		return nil, fmt.Errorf("deck recommend service is not configured")
	}
	return c.buildAutoRecommendWithEngine(query)
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
