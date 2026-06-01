package provider

import (
	"fmt"
	json "github.com/bytedance/sonic"

	sekaiDB "haruki-cloud/database/sekai"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

func convertCloudHonor(entity *sekaiDB.Honor) (*masterdata.Honor, error) {
	model := &masterdata.Honor{
		ID:              int(entity.GameID),
		GroupID:         int(entity.GroupID),
		HonorRarity:     entity.HonorRarity,
		Name:            entity.Name,
		Description:     "",
		AssetBundleName: entity.AssetbundleName,
	}
	if len(entity.Levels) > 0 {
		if err := json.Unmarshal(entity.Levels, &model.Levels); err != nil {
			return nil, fmt.Errorf("unmarshal honor levels: %w", err)
		}
	}
	return model, nil
}
