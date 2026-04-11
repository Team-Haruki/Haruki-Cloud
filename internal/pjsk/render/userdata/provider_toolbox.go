package userdata

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	sekaiDB "haruki-cloud/database/sekai"
	"haruki-cloud/internal/pjsk/render/assets"
	renderregion "haruki-cloud/internal/pjsk/render/region"
	accountdata "haruki-cloud/internal/pjsk/userdata"
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
}

func NewToolboxSnapshotProvider(bindings bindingLookup, client privateDataClient, sekai *sekaiDB.Client, assetHelper *assets.AssetHelper) *ToolboxSnapshotProvider {
	return &ToolboxSnapshotProvider{
		bindings: bindings,
		client:   client,
		factory:  NewDefaultSnapshotFactory(sekai, assetHelper),
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
		return nil, fmt.Errorf("userdata: snapshot selector is incomplete")
	}

	region := renderregion.WithDefault(selector.Region)
	binding, err := resolveSnapshotBinding(ctx, p.bindings, platform, imUserID, region, selector.PJSKUserID, opts)
	if err != nil {
		return nil, err
	}

	uid, err := strconv.ParseInt(binding.PJSKUserID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("userdata: invalid bound pjsk user id %q: %w", binding.PJSKUserID, err)
	}

	suiteJSON, err := p.client.GetSuiteData(binding.Server, uid, platform, imUserID)
	if err != nil {
		return nil, err
	}
	if len(suiteJSON) == 0 {
		return nil, fmt.Errorf("userdata: suite snapshot is empty")
	}

	var mysekaiJSON []byte
	if opts.NeedMySekai {
		mysekaiJSON, err = p.client.GetMySekaiData(binding.Server, uid, platform, imUserID)
		if err != nil {
			return nil, err
		}
	}

	metaRegion := region
	if bindingRegion := renderregion.Normalize(binding.Server); !bindingRegion.IsZero() {
		metaRegion = bindingRegion
	}
	var musicMetaJSON []byte
	if p.metas != nil {
		musicMetaJSON = p.metas.Get(metaRegion.String())
	}

	snapshot, err := p.factory.Build(ctx, BuildInput{
		Region:        region,
		Source:        "toolbox_live",
		SuiteJSON:     suiteJSON,
		MySekaiJSON:   mysekaiJSON,
		MusicMetaJSON: musicMetaJSON,
	})
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}
