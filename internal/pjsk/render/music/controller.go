package music

import (
	"context"
	"fmt"
	"strings"

	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/meta"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/internal/pjsk/render/masterdata"
	"haruki-cloud/internal/pjsk/render/snapshot"
	regionsource "haruki-cloud/internal/pjsk/render/source"
	sekaiapi "haruki-cloud/internal/pjsk/sekai"
)

var hiddenMusicIDs = map[int]struct{}{
	241: {},
	290: {},
}

func (c *Controller) WithContext(ctx context.Context) *Controller {
	if c == nil {
		return nil
	}
	clone := *c
	clone.requestCtx = ctx
	clone.drawing = c.drawing.WithContext(ctx)
	clone.assets = c.assets.WithContext(ctx)
	if customScores, ok := c.customScores.(*sekaiapi.HarukiSekaiAPIClient); ok {
		clone.customScores = customScores.WithContext(ctx)
	}
	clone.sources = regionsource.NewRegistry[DataSource](c.sources.ResolveRegion(renderregion.Unknown))
	for _, source := range c.sources.OrderedSources() {
		if contextual, ok := any(source).(contextualDataSource); ok {
			clone.sources.RegisterSource(contextual.WithContext(ctx))
			continue
		}
		clone.sources.RegisterSource(source)
	}
	return &clone
}

func (c *Controller) WithSnapshot(snapshot snapshot.Snapshot) *Controller {
	if c == nil {
		return nil
	}
	clone := *c
	clone.snapshot = snapshot
	return &clone
}

func (c *Controller) SetCustomMusicScoreClient(client customMusicScoreClient) {
	if c == nil {
		return
	}
	c.customScores = client
}

func NewController(defaultSource DataSource, drawingClient *drawing.HarukiDrawingClient, assetHelper *assets.AssetHelper, snapshot snapshot.Snapshot, metaLoader *meta.Loader) *Controller {
	if assetHelper == nil {
		assetHelper = assets.NewAssetHelper("", nil)
	}
	controller := &Controller{
		sources:               regionsource.NewRegistry[DataSource](renderregion.JP),
		drawing:               drawingClient,
		assets:                assetHelper,
		banCharacterNicknames: cloneNicknames(defaultBanCharacterNicknames),
		snapshot:              snapshot,
		metaLoader:            metaLoader,
	}
	controller.RegisterSource(defaultSource)
	return controller
}

func (c *Controller) RegisterSource(source DataSource) {
	c.sources.RegisterSource(source)
}

func (c *Controller) SetAliasResolver(resolver musicAliasResolver) {
	if c == nil {
		return
	}
	c.aliases = resolver
}

func (c *Controller) contextOrBackground() context.Context {
	if c != nil && c.requestCtx != nil {
		return c.requestCtx
	}
	return context.TODO()
}

func (c *Controller) newSearchService(source DataSource, allowUnreleased ...bool) *SearchService {
	allow := false
	if len(allowUnreleased) > 0 {
		allow = allowUnreleased[0]
	}
	return NewSearchService(source, NewParser(c.banCharacterNicknames)).
		WithAllowUnreleased(allow).
		WithTitleResolver(func(query string) (*masterdata.Music, error) {
			return c.resolveMusicTitleQuery(source, query, allow)
		})
}

func (c *Controller) shouldAllowLookupLeaks(region string, explicit bool) bool {
	if explicit {
		return true
	}
	return c.resolveRegion(region) != renderregion.JP
}

func (c *Controller) resolveMusicTitleQuery(source DataSource, query string, allowUnreleased bool) (*masterdata.Music, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("music query is empty")
	}
	now := currentMusicVisibilityTime()

	if musicInfo, handled, err := resolveExplicitMusicTitleQuery(source, query, now, allowUnreleased); handled {
		return musicInfo, err
	}
	if musicInfo, handled, err := c.resolveAliasMusicTitleQuery(source, query, now, allowUnreleased); handled {
		return musicInfo, err
	}
	return resolveMusicTitleFallbacks(source, query, allowUnreleased)
}

func resolveExplicitMusicTitleQuery(source DataSource, query string, now int64, allowUnreleased bool) (*masterdata.Music, bool, error) {
	musicID, ok := ParseExplicitMusicID(query)
	if !ok {
		return nil, false, nil
	}
	musicInfo, err := source.GetMusicByID(musicID)
	if err != nil {
		return nil, true, err
	}
	musicInfo, err = ensureAccessibleMusic(musicInfo, now, musicID, allowUnreleased)
	return musicInfo, true, err
}

func (c *Controller) resolveAliasMusicTitleQuery(
	source DataSource,
	query string,
	now int64,
	allowUnreleased bool,
) (*masterdata.Music, bool, error) {
	if c == nil || c.aliases == nil {
		return nil, false, nil
	}
	musicID, ok, err := c.aliases.TryResolveMusicTitleOrAliasID(c.contextOrBackground(), query)
	if err != nil {
		musicInfo, normalizedErr := normalizeAmbiguousAliasMusic(source, err, now, allowUnreleased)
		return musicInfo, true, normalizedErr
	}
	if !ok {
		return nil, false, nil
	}
	musicInfo, err := source.GetMusicByID(musicID)
	if err != nil {
		return nil, true, err
	}
	musicInfo, err = ensureAccessibleMusic(musicInfo, now, query, allowUnreleased)
	return musicInfo, true, err
}

func normalizeAmbiguousAliasMusic(source DataSource, aliasErr error, now int64, allowUnreleased bool) (*masterdata.Music, error) {
	ids := ExtractAmbiguousMusicIDs(aliasErr)
	if len(ids) == 0 {
		return nil, aliasErr
	}
	musicInfo, err := selectUniqueMusicMatch("曲名/别名", collectVisibleMusicMatchesByID(source, ids, now, allowUnreleased))
	if musicInfo == nil && err == nil {
		return nil, aliasErr
	}
	return musicInfo, err
}

func resolveMusicTitleFallbacks(source DataSource, query string, allowUnreleased bool) (*masterdata.Music, error) {
	musicInfo, err := resolveUniqueMusicQuery(source, query, allowUnreleased)
	if err == nil || isMusicAmbiguousError(err) {
		return musicInfo, err
	}

	fuzzyMusic, fuzzyErr := resolveFuzzyMusicQuery(source, query, allowUnreleased)
	if fuzzyErr == nil || isMusicAmbiguousError(fuzzyErr) {
		return fuzzyMusic, fuzzyErr
	}
	return nil, err
}

func (c *Controller) resolveMusicListKeywordFilter(source DataSource, keyword string, allowUnreleased bool) (*int, string, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return nil, "", nil
	}
	now := currentMusicVisibilityTime()

	if musicID, handled, err := resolveExplicitMusicListFilter(source, keyword, now, allowUnreleased); handled {
		return musicID, "", err
	}
	if musicID := resolveImplicitMusicListFilter(source, keyword, now, allowUnreleased); musicID != nil {
		return musicID, "", nil
	}
	if musicID, handled, err := c.resolveAliasMusicListFilter(source, keyword, now, allowUnreleased); handled {
		return musicID, "", err
	}
	return nil, strings.ToLower(keyword), nil
}

func resolveExplicitMusicListFilter(source DataSource, keyword string, now int64, allowUnreleased bool) (*int, bool, error) {
	musicID, ok := ParseExplicitMusicID(keyword)
	if !ok {
		return nil, false, nil
	}
	if source == nil {
		return nil, true, fmt.Errorf("music data source is not configured")
	}
	musicInfo, err := source.GetMusicByID(musicID)
	if err != nil {
		return nil, true, err
	}
	if _, err := ensureAccessibleMusic(musicInfo, now, musicID, allowUnreleased); err != nil {
		return nil, true, err
	}
	return &musicID, true, nil
}

func resolveImplicitMusicListFilter(source DataSource, keyword string, now int64, allowUnreleased bool) *int {
	musicID, ok := ParseImplicitMusicID(keyword)
	if !ok || source == nil {
		return nil
	}
	musicInfo, err := source.GetMusicByID(musicID)
	if err != nil || !isMusicAccessibleAt(musicInfo, now, allowUnreleased) {
		return nil
	}
	return &musicID
}

func (c *Controller) resolveAliasMusicListFilter(
	source DataSource,
	keyword string,
	now int64,
	allowUnreleased bool,
) (*int, bool, error) {
	if c == nil || c.aliases == nil {
		return nil, false, nil
	}
	musicID, ok, err := c.aliases.TryResolveMusicTitleOrAliasID(c.contextOrBackground(), keyword)
	if err != nil || !ok {
		return nil, err != nil, err
	}
	if source == nil {
		return &musicID, true, nil
	}
	musicInfo, err := source.GetMusicByID(musicID)
	if err != nil {
		return nil, true, err
	}
	if _, err := ensureAccessibleMusic(musicInfo, now, keyword, allowUnreleased); err != nil {
		return nil, true, err
	}
	return &musicID, true, nil
}

func (c *Controller) resolveBuilder(region string) (renderregion.Value, DataSource, *Builder, error) {
	resolved := c.sources.ResolveRegion(renderregion.Normalize(region))
	source, ok := c.sources.SourceForRegion(resolved)
	if !ok {
		return resolved, nil, nil, fmt.Errorf("no music data source for region %s", resolved)
	}
	return resolved, source, NewBuilder(source, c.fallbackSource(resolved), c.assets), nil
}

func (c *Controller) fallbackSource(region renderregion.Value) DataSource {
	if region == renderregion.JP {
		return nil
	}
	if source, ok := c.sources.SourceForRegion(renderregion.JP); ok {
		return source
	}
	return nil
}

func (c *Controller) resolveRegion(region string) renderregion.Value {
	if c == nil || c.sources == nil {
		return renderregion.WithDefault(renderregion.Normalize(region))
	}
	return c.sources.ResolveRegion(renderregion.Normalize(region))
}
