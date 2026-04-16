package snapshot

import (
	"context"
	"fmt"

	sekaiDB "haruki-cloud/database/sekai"
	"haruki-cloud/internal/pjsk/render/assets"
	renderregion "haruki-cloud/internal/pjsk/region"
)

// NewFromBytes constructs a Service from raw JSON bytes obtained from live API calls,
// rather than from local files. This is the live-data path used in production.
//
//   - suiteJSON:      required; raw bytes from GetSuiteData() (equivalent to user.json)
//   - mysekaiJSON:    optional (nil = skip); raw bytes from GetMySekaiData() (equivalent to mysekai.json)
//   - musicMetaBytes: optional (nil = skip); raw bytes from the music-meta cache
//
// sekaiClient and assetHelper are used to resolve the leader card image path for rendering.
func NewFromBytes(
	sekaiClient *sekaiDB.Client,
	assetHelper *assets.AssetHelper,
	region renderregion.Value,
	suiteJSON []byte,
	mysekaiJSON []byte,
	musicMetaBytes []byte,
) (*Service, error) {
	return NewFromBytesWithContext(context.Background(), sekaiClient, assetHelper, region, suiteJSON, mysekaiJSON, musicMetaBytes)
}

func NewFromBytesWithContext(
	ctx context.Context,
	sekaiClient *sekaiDB.Client,
	assetHelper *assets.AssetHelper,
	region renderregion.Value,
	suiteJSON []byte,
	mysekaiJSON []byte,
	musicMetaBytes []byte,
) (*Service, error) {
	snapshot, err := NewDefaultSnapshotFactory(sekaiClient, assetHelper).Build(ctx, BuildInput{
		Region:        region,
		Source:        "toolbox_live",
		SuiteJSON:     suiteJSON,
		MySekaiJSON:   mysekaiJSON,
		MusicMetaJSON: musicMetaBytes,
	})
	if err != nil {
		return nil, err
	}
	service, ok := snapshot.(*Service)
	if !ok {
		return nil, fmt.Errorf("userdata: snapshot factory returned unexpected type %T", snapshot)
	}
	return service, nil
}
