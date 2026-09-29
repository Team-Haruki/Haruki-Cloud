package provider

import "strings"

// soloVirtualLivesFile names the solo-only slice of virtuallives for the raw
// row store. Selecting only these rows keeps the whole virtuallives table
// (large per-live JSON columns) out of the row cache, and SELECT * keeps
// working before the cheer-point columns exist.
const soloVirtualLivesFile = "virtualLives.solo.json"

// applyVLiveSoloFields copies the solo cheer-point fields of a raw
// virtualLives row (game JSON key names) onto live.
func applyVLiveSoloFields(live *VLive, row map[string]any) {
	if live == nil || row == nil {
		return
	}
	if items, ok := row["virtualLiveTotalCheerPointRewards"].([]any); ok {
		live.TotalCheerPointRewards = live.TotalCheerPointRewards[:0]
		for _, raw := range items {
			item, _ := raw.(map[string]any)
			threshold, _ := interfaceToInt(item["threshold"])
			boxID, _ := interfaceToInt(item["resourceBoxId"])
			if threshold > 0 && boxID > 0 {
				live.TotalCheerPointRewards = append(live.TotalCheerPointRewards, VLiveTotalCheerPointReward{Threshold: threshold, ResourceBoxID: boxID})
			}
		}
	}
	if item, ok := row["virtualLiveTotalCheerPointSurplusReward"].(map[string]any); ok {
		basePoint, _ := interfaceToInt(item["basePoint"])
		boxID, _ := interfaceToInt(item["resourceBoxId"])
		if basePoint > 0 && boxID > 0 {
			live.TotalCheerPointSurplusReward = &VLiveSurplusReward{BasePoint: basePoint, ResourceBoxID: boxID}
		}
	}
	if item, ok := row["virtualLiveVirtualItemOverrideCost"].(map[string]any); ok {
		resourceID, _ := interfaceToInt(item["costResourceId"])
		resourceType := strings.TrimSpace(vliveString(item["costResourceType"]))
		if resourceType != "" {
			live.VirtualItemOverrideCost = &VLiveOverrideCost{
				CostResourceType: resourceType,
				CostResourceID:   resourceID,
				AssetBundleName:  strings.TrimSpace(vliveString(item["assetbundleName"])),
			}
		}
	}
}

func vliveGroupFromRow(id int, row map[string]any) *VLiveGroup {
	if id <= 0 || row == nil {
		return nil
	}
	startAt, _ := interfaceToInt(row["startAt"])
	endAt, _ := interfaceToInt(row["endAt"])
	return &VLiveGroup{
		ID:                   id,
		Name:                 strings.TrimSpace(vliveString(row["name"])),
		VirtualLiveGroupType: strings.TrimSpace(vliveString(row["virtualLiveGroupType"])),
		AssetBundleName:      strings.TrimSpace(vliveString(row["assetbundleName"])),
		StartAt:              int64(startAt),
		EndAt:                int64(endAt),
	}
}

func vliveGroupsFromRows(rows map[int]map[string]any) map[int]*VLiveGroup {
	groups := make(map[int]*VLiveGroup, len(rows))
	for id, row := range rows {
		if group := vliveGroupFromRow(id, row); group != nil {
			groups[id] = group
		}
	}
	return groups
}

func hasSoloVLive(lives []*VLive) bool {
	for _, live := range lives {
		if live != nil && live.VirtualLiveType == VLiveTypeSolo {
			return true
		}
	}
	return false
}
