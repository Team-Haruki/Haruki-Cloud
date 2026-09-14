package honor

import (
	"context"
	"fmt"

	"haruki-cloud/internal/pjsk/drawing"
	"haruki-cloud/internal/pjsk/render/assets"
)

func NewBuilder(source DataSource, assetHelper *assets.AssetHelper) *Builder {
	return &Builder{
		source: source,
		assets: assetHelper,
	}
}

// WithAssetReader routes the builder's existence probes through reader, bound
// to ctx. A nil reader keeps the local AssetHelper probing.
func (b *Builder) WithAssetReader(ctx context.Context, reader *assets.AssetReader) *Builder {
	if b == nil {
		return nil
	}
	b.ctx = ctx
	b.reader = reader
	return b
}

func (b *Builder) BuildHonorRequest(query Query) (*drawing.HonorRequest, error) {
	req := &drawing.HonorRequest{
		IsMainHonor: query.IsMain,
	}

	_, errNormal := b.source.GetHonorByID(query.HonorID)
	bondsHonor, errBonds := b.source.GetBondsHonorByID(query.HonorID)

	isNormal := errNormal == nil
	isBonds := errBonds == nil
	if !isNormal && !isBonds {
		return nil, fmt.Errorf("honor %d not found in any masterdata table", query.HonorID)
	}

	if isNormal {
		if err := b.buildNormalHonorRequest(req, query.HonorID, query.HonorLevel, query.FcOrApLevelOverride, query.Region); err != nil {
			return nil, err
		}
	} else if isBonds {
		if err := b.buildBondsHonorRequest(req, bondsHonor, query.HonorLevel, query.BondsHonorViewType, query.BondsHonorWordID, query.UseUnitVirtualSinger, query.Region); err != nil {
			return nil, err
		}
	}

	return req, nil
}
