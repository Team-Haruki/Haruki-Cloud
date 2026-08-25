package provider

import (
	json "haruki-cloud/internal/jsonutil"

	"haruki-cloud/internal/pjsk/render/masterdata"
)

type eventRankingRewardRange struct {
	FromRank                  int                        `json:"fromRank"`
	ToRank                    int                        `json:"toRank"`
	EventRankingRewardDetails []eventRankingRewardDetail `json:"eventRankingRewardDetails"`
}

type eventRankingRewardDetail struct {
	ResourceType string `json:"resourceType"`
	ResourceID   int    `json:"resourceId"`
}

func parseEventRankingHonorRewards(raw []byte) []masterdata.EventRankingHonorReward {
	if len(raw) == 0 {
		return nil
	}

	var ranges []eventRankingRewardRange
	if err := json.Unmarshal(raw, &ranges); err != nil {
		return nil
	}

	rewards := make([]masterdata.EventRankingHonorReward, 0)
	for _, item := range ranges {
		for _, detail := range item.EventRankingRewardDetails {
			if detail.ResourceType != "honor" || detail.ResourceID <= 0 {
				continue
			}
			rewards = append(rewards, masterdata.EventRankingHonorReward{
				FromRank: item.FromRank,
				ToRank:   item.ToRank,
				HonorID:  detail.ResourceID,
			})
		}
	}
	return rewards
}
