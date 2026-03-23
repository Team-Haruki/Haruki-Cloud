package meta

import "encoding/json"

// InjectOmakase ensures music_metas JSON contains a synthetic "omakase" entry
// (music_id 10000) for each difficulty in [master, expert, hard].
// Values are averaged across all qualifying real tracks.
// If the entry already exists, or the JSON is malformed, the original data is returned unchanged.
func InjectOmakase(data []byte) []byte {
	var metas []map[string]interface{}
	if err := json.Unmarshal(data, &metas); err != nil {
		return data
	}

	const omakaseMusicID = 10000
	difficulties := []string{"master", "expert", "hard"}

	for _, item := range metas {
		if musicID, ok := item["music_id"].(float64); ok && int(musicID) == omakaseMusicID {
			return data
		}
	}

	aggregate := map[string]interface{}{
		"music_time":        0.0,
		"event_rate":        0.0,
		"base_score":        0.0,
		"base_score_auto":   0.0,
		"fever_score":       0.0,
		"fever_end_time":    0.0,
		"tap_count":         0.0,
		"skill_score_solo":  []float64{0, 0, 0, 0, 0, 0},
		"skill_score_auto":  []float64{0, 0, 0, 0, 0, 0},
		"skill_score_multi": []float64{0, 0, 0, 0, 0, 0},
	}
	count := 0.0

	for _, item := range metas {
		diff, _ := item["difficulty"].(string)
		if diff != "master" && diff != "expert" && diff != "hard" {
			continue
		}
		count++
		accumulateFloat(aggregate, item, "music_time")
		accumulateFloat(aggregate, item, "event_rate")
		accumulateFloat(aggregate, item, "base_score")
		accumulateFloat(aggregate, item, "base_score_auto")
		accumulateFloat(aggregate, item, "fever_score")
		accumulateFloat(aggregate, item, "fever_end_time")
		accumulateFloat(aggregate, item, "tap_count")
		accumulateFloatSlice(aggregate, item, "skill_score_solo")
		accumulateFloatSlice(aggregate, item, "skill_score_auto")
		accumulateFloatSlice(aggregate, item, "skill_score_multi")
	}

	if count == 0 {
		return data
	}

	aggregate["music_time"] = aggregate["music_time"].(float64) / count
	aggregate["event_rate"] = float64(int(aggregate["event_rate"].(float64) / count))
	aggregate["base_score"] = aggregate["base_score"].(float64) / count
	aggregate["base_score_auto"] = aggregate["base_score_auto"].(float64) / count
	aggregate["fever_score"] = aggregate["fever_score"].(float64) / count
	aggregate["fever_end_time"] = aggregate["fever_end_time"].(float64) / count
	aggregate["tap_count"] = float64(int(aggregate["tap_count"].(float64) / count))

	for _, key := range []string{"skill_score_solo", "skill_score_auto", "skill_score_multi"} {
		values := aggregate[key].([]float64)
		for i := range values {
			values[i] /= count
		}
	}

	for _, difficulty := range difficulties {
		metas = append(metas, map[string]interface{}{
			"music_id":          float64(omakaseMusicID),
			"difficulty":        difficulty,
			"music_time":        aggregate["music_time"],
			"event_rate":        aggregate["event_rate"],
			"base_score":        aggregate["base_score"],
			"base_score_auto":   aggregate["base_score_auto"],
			"fever_score":       aggregate["fever_score"],
			"fever_end_time":    aggregate["fever_end_time"],
			"tap_count":         aggregate["tap_count"],
			"skill_score_solo":  aggregate["skill_score_solo"],
			"skill_score_auto":  aggregate["skill_score_auto"],
			"skill_score_multi": aggregate["skill_score_multi"],
		})
	}

	encoded, err := json.Marshal(metas)
	if err != nil {
		return data
	}
	return encoded
}

func accumulateFloat(target, source map[string]interface{}, key string) {
	value, ok := source[key].(float64)
	if !ok {
		return
	}
	target[key] = target[key].(float64) + value
}

func accumulateFloatSlice(target, source map[string]interface{}, key string) {
	values, ok := source[key].([]interface{})
	if !ok {
		return
	}
	base := target[key].([]float64)
	for i, raw := range values {
		if i >= len(base) {
			break
		}
		if v, ok := raw.(float64); ok {
			base[i] += v
		}
	}
}
