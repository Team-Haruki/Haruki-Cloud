package snapshot

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	sekaiDB "haruki-cloud/database/sekai"
	"haruki-cloud/internal/pjsk/accountdata"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/assets"
	"haruki-cloud/utils/logger"
)

type bindingLookup interface {
	ResolveUserBinding(ctx context.Context, platform, platformUserID, server string) (int, *accountdata.ResolvedBinding, error)
	List(ctx context.Context, platform, platformUserID string) ([]accountdata.BindingListItem, error)
}

type privateDataClient interface {
	GetSuiteData(server string, userID int64, platform, platformUserID string) ([]byte, error)
	GetMySekaiData(server string, userID int64, platform, platformUserID string) ([]byte, error)
}

type musicMetaSource interface {
	Get(region string) []byte
}

// ToolboxSnapshotProvider is the current request-scoped provider implementation.
// It keeps the existing Toolbox live-data path behind a stable provider
// contract, so future DB-backed snapshot stores can replace it without forcing
// controller and bridge layers to change again.
type ToolboxSnapshotProvider struct {
	bindings bindingLookup
	client   privateDataClient
	factory  HarukiSnapshotFactory
	metas    musicMetaSource
	logger   *logger.Logger
}

func NewToolboxSnapshotProvider(bindings bindingLookup, client privateDataClient, sekai *sekaiDB.Client, assetHelper *assets.AssetHelper) *ToolboxSnapshotProvider {
	return &ToolboxSnapshotProvider{
		bindings: bindings,
		client:   client,
		factory:  NewDefaultSnapshotFactory(sekai, assetHelper),
		logger:   logger.NewLoggerFromGlobal("ToolboxSnapshot"),
	}
}

func (p *ToolboxSnapshotProvider) WithMusicMetaSource(source musicMetaSource) *ToolboxSnapshotProvider {
	if p == nil {
		return nil
	}
	p.metas = source
	return p
}

func (p *ToolboxSnapshotProvider) Resolve(ctx context.Context, selector Selector, opts ResolveOptions) (Snapshot, error) {
	if p == nil || p.bindings == nil || p.client == nil || p.factory == nil {
		return nil, ErrProviderUnavailable
	}

	platform := strings.TrimSpace(selector.IMPlatform)
	imUserID := strings.TrimSpace(selector.IMUserID)
	if platform == "" || imUserID == "" {
		return nil, fmt.Errorf("snapshot: snapshot selector is incomplete")
	}

	tResolve := time.Now()
	region := renderregion.WithDefault(selector.Region)
	binding, err := resolveSnapshotBinding(ctx, p.bindings, platform, imUserID, region, selector.PJSKUserID, opts)
	if err != nil {
		p.logger.Warnf("toolbox snapshot binding failed: platform=%s user=%s region=%s pjsk_user=%s need_mysekai=%t err=%v",
			platform, maskBindingDebugID(imUserID), region.String(), maskBindingDebugID(selector.PJSKUserID), opts.NeedMySekai, err)
		return nil, err
	}
	p.logger.Debugf("toolbox snapshot binding selected: platform=%s user=%s region=%s binding=%s",
		platform, maskBindingDebugID(imUserID), region.String(), formatSnapshotBindingDebug(binding))

	uid, err := strconv.ParseInt(binding.PJSKUserID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("snapshot: invalid bound pjsk user id %q: %w", binding.PJSKUserID, err)
	}

	tSuite := time.Now()
	var suiteCacheHit bool
	suiteJSON, err, suiteCacheHit := cachedPrivateData(ctx, privateDataCacheKey{
		Server:         binding.Server,
		DataType:       "suite",
		UserID:         uid,
		Platform:       platform,
		PlatformUserID: imUserID,
	}, func() ([]byte, error) {
		return p.client.GetSuiteData(binding.Server, uid, platform, imUserID)
	})
	suiteElapsed := time.Since(tSuite)
	if err != nil {
		p.logger.Warnf("toolbox suite fetch failed: elapsed=%dms platform=%s user=%s binding=%s err=%v",
			suiteElapsed.Milliseconds(), platform, maskBindingDebugID(imUserID), formatSnapshotBindingDebug(binding), err)
		return nil, err
	}
	if len(suiteJSON) == 0 {
		p.logger.Warnf("toolbox suite fetch returned empty payload: platform=%s user=%s binding=%s",
			platform, maskBindingDebugID(imUserID), formatSnapshotBindingDebug(binding))
		return nil, fmt.Errorf("snapshot: suite snapshot is empty")
	}
	if suiteCacheHit {
		p.logger.Infof("toolbox suite cache hit: elapsed=%dms platform=%s user=%s binding=%s bytes=%d",
			suiteElapsed.Milliseconds(), platform, maskBindingDebugID(imUserID), formatSnapshotBindingDebug(binding), len(suiteJSON))
	} else {
		p.logger.Infof("toolbox suite fetch: elapsed=%dms platform=%s user=%s binding=%s bytes=%d",
			suiteElapsed.Milliseconds(), platform, maskBindingDebugID(imUserID), formatSnapshotBindingDebug(binding), len(suiteJSON))
	}

	var mysekaiJSON []byte
	if opts.NeedMySekai {
		tMysekai := time.Now()
		var mysekaiCacheHit bool
		mysekaiJSON, err, mysekaiCacheHit = cachedPrivateData(ctx, privateDataCacheKey{
			Server:         binding.Server,
			DataType:       "mysekai",
			UserID:         uid,
			Platform:       platform,
			PlatformUserID: imUserID,
		}, func() ([]byte, error) {
			return p.client.GetMySekaiData(binding.Server, uid, platform, imUserID)
		})
		mysekaiElapsed := time.Since(tMysekai)
		if err != nil {
			p.logger.Warnf("toolbox mysekai fetch failed: elapsed=%dms platform=%s user=%s binding=%s err=%v",
				mysekaiElapsed.Milliseconds(), platform, maskBindingDebugID(imUserID), formatSnapshotBindingDebug(binding), err)
			return nil, err
		}
		if mysekaiCacheHit {
			p.logger.Infof("toolbox mysekai cache hit: elapsed=%dms platform=%s user=%s binding=%s bytes=%d",
				mysekaiElapsed.Milliseconds(), platform, maskBindingDebugID(imUserID), formatSnapshotBindingDebug(binding), len(mysekaiJSON))
		} else {
			p.logger.Infof("toolbox mysekai fetch: elapsed=%dms platform=%s user=%s binding=%s bytes=%d",
				mysekaiElapsed.Milliseconds(), platform, maskBindingDebugID(imUserID), formatSnapshotBindingDebug(binding), len(mysekaiJSON))
		}
	}

	metaRegion := region
	if bindingRegion := renderregion.Normalize(binding.Server); !bindingRegion.IsZero() {
		metaRegion = bindingRegion
	}
	snapshotRegion := metaRegion
	var musicMetaJSON []byte
	if p.metas != nil {
		musicMetaJSON = p.metas.Get(metaRegion.String())
	}

	snapshot, err := p.factory.Build(ctx, BuildInput{
		Region:        snapshotRegion,
		Source:        "toolbox_live",
		SuiteJSON:     suiteJSON,
		MySekaiJSON:   mysekaiJSON,
		MusicMetaJSON: musicMetaJSON,
	})
	if err != nil {
		p.logger.Warnf("toolbox snapshot build failed: platform=%s user=%s binding=%s snapshot_region=%s suite_bytes=%d mysekai_bytes=%d err=%v",
			platform, maskBindingDebugID(imUserID), formatSnapshotBindingDebug(binding), snapshotRegion.String(), len(suiteJSON), len(mysekaiJSON), err)
		return nil, err
	}
	p.logger.Infof("toolbox snapshot resolve: elapsed=%dms platform=%s user=%s binding=%s region=%s",
		time.Since(tResolve).Milliseconds(), platform, maskBindingDebugID(imUserID), formatSnapshotBindingDebug(binding), snapshotRegion.String())
	return snapshot, nil
}
