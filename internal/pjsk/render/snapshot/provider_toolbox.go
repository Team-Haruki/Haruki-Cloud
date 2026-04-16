package snapshot

import (
	"context"
	"fmt"
	"strconv"
	"strings"

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
	factory  SnapshotFactory
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

	suiteJSON, err := p.client.GetSuiteData(binding.Server, uid, platform, imUserID)
	if err != nil {
		p.logger.Warnf("toolbox suite fetch failed: platform=%s user=%s binding=%s err=%v",
			platform, maskBindingDebugID(imUserID), formatSnapshotBindingDebug(binding), err)
		return nil, err
	}
	if len(suiteJSON) == 0 {
		p.logger.Warnf("toolbox suite fetch returned empty payload: platform=%s user=%s binding=%s",
			platform, maskBindingDebugID(imUserID), formatSnapshotBindingDebug(binding))
		return nil, fmt.Errorf("snapshot: suite snapshot is empty")
	}
	p.logger.Debugf("toolbox suite fetch succeeded: platform=%s user=%s binding=%s suite_bytes=%d",
		platform, maskBindingDebugID(imUserID), formatSnapshotBindingDebug(binding), len(suiteJSON))

	var mysekaiJSON []byte
	if opts.NeedMySekai {
		mysekaiJSON, err = p.client.GetMySekaiData(binding.Server, uid, platform, imUserID)
		if err != nil {
			p.logger.Warnf("toolbox mysekai fetch failed: platform=%s user=%s binding=%s err=%v",
				platform, maskBindingDebugID(imUserID), formatSnapshotBindingDebug(binding), err)
			return nil, err
		}
		p.logger.Debugf("toolbox mysekai fetch succeeded: platform=%s user=%s binding=%s mysekai_bytes=%d",
			platform, maskBindingDebugID(imUserID), formatSnapshotBindingDebug(binding), len(mysekaiJSON))
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
	p.logger.Debugf("toolbox snapshot build succeeded: platform=%s user=%s binding=%s snapshot_region=%s",
		platform, maskBindingDebugID(imUserID), formatSnapshotBindingDebug(binding), snapshotRegion.String())
	return snapshot, nil
}
