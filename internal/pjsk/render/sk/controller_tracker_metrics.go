package sk

import (
	"sort"
	"time"

	"haruki-cloud/internal/pjsk/drawing"
)

const trackerRealtimeTailMaxLagSeconds = int64(30 * 24 * time.Hour / time.Second)
const trackerRecoveryIdleSeconds = int64(5 * time.Minute / time.Second)

func applyRankInfoMetrics(info *drawing.RankInfo, samples []trackerScoreSample) {
	applyRankInfoMetricsAt(info, samples, time.Now().UTC())
}

func applyRankInfoMetricsAt(info *drawing.RankInfo, samples []trackerScoreSample, now time.Time) {
	if info == nil || len(samples) == 0 {
		return
	}

	normalized := normalizedTrackerScoreSamples(samples)
	if len(normalized) < 2 {
		return
	}

	sort.Slice(normalized, func(i, j int) bool {
		return normalizeTrackerUnixSeconds(normalized[i].timestamp) < normalizeTrackerUnixSeconds(normalized[j].timestamp)
	})

	if recordStartAt := recoveryRecordStartAt(normalized); recordStartAt != nil {
		info.RecordStartAt = recordStartAt
	}

	applyTrackerDeltaMetrics(info, normalized)
	last := normalized[len(normalized)-1]
	endSec := effectiveTrackerWindowEndUnixSeconds(last.timestamp, now)
	applyTrackerHourMetrics(info, normalized, last, endSec)
	applyTrackerTwentyMinuteMetrics(info, normalized, last, endSec)
}

func normalizedTrackerScoreSamples(samples []trackerScoreSample) []trackerScoreSample {
	normalized := make([]trackerScoreSample, 0, len(samples))
	for _, sample := range samples {
		if sample.timestamp > 0 {
			normalized = append(normalized, sample)
		}
	}
	return normalized
}

func applyTrackerDeltaMetrics(info *drawing.RankInfo, samples []trackerScoreSample) {
	deltas := make([]int, 0, len(samples)-1)
	for i := 1; i < len(samples); i++ {
		diff := samples[i].score - samples[i-1].score
		if diff > 0 {
			deltas = append(deltas, diff)
		}
	}
	if len(deltas) == 0 {
		return
	}
	info.LatestPt = drawing.IntPtr(deltas[len(deltas)-1])
	avgWindow := deltas
	if len(avgWindow) > 10 {
		avgWindow = avgWindow[len(avgWindow)-10:]
	}
	sum := 0
	for _, value := range avgWindow {
		sum += value
	}
	roundCount := len(avgWindow)
	info.AverageRound = drawing.IntPtr(roundCount)
	info.AveragePt = drawing.IntPtr(sum / roundCount)
}

func applyTrackerHourMetrics(info *drawing.RankInfo, samples []trackerScoreSample, last trackerScoreSample, endSec int64) {
	hourStart := endSec - 60*60
	hourBaseIdx := findWindowBaselineIndex(samples, hourStart)
	if hourBaseIdx < 0 {
		return
	}
	hourBase := samples[hourBaseIdx]
	hourBaseSec := normalizeTrackerUnixSeconds(hourBase.timestamp)
	if endSec > hourBaseSec {
		hourGain := max(last.score-hourBase.score, 0)
		hourElapsed := endSec - hourBaseSec
		speed := int((int64(hourGain) * 3600) / hourElapsed)
		info.Speed = drawing.IntPtr(speed)
	}
	hourRound := countPositiveDeltas(samples[hourBaseIdx:])
	info.HourRound = drawing.IntPtr(hourRound)
}

func applyTrackerTwentyMinuteMetrics(info *drawing.RankInfo, samples []trackerScoreSample, last trackerScoreSample, endSec int64) {
	windowStart := endSec - 20*60
	windowBaseIdx := findWindowBaselineIndex(samples, windowStart)
	if windowBaseIdx < 0 {
		return
	}
	windowGain := max(last.score-samples[windowBaseIdx].score, 0)
	info.Min20Time3Speed = drawing.IntPtr(windowGain * 3)
}

func normalizeTrackerUnixSeconds(ts int64) int64 {
	if ts > 1_000_000_000_000 {
		return ts / 1000
	}
	return ts
}

func effectiveTrackerWindowEndUnixSeconds(lastTimestamp int64, now time.Time) int64 {
	lastSec := normalizeTrackerUnixSeconds(lastTimestamp)
	if lastSec <= 0 {
		return lastSec
	}

	nowSec := now.UTC().Unix()
	if nowSec <= lastSec {
		return lastSec
	}
	if nowSec-lastSec > trackerRealtimeTailMaxLagSeconds {
		return lastSec
	}
	return nowSec
}

func appendIdleTrackerRankTraceAt(ranks []drawing.RankInfo, now time.Time) []drawing.RankInfo {
	if len(ranks) == 0 {
		return ranks
	}

	last := ranks[len(ranks)-1]
	lastSec := normalizeTrackerUnixSeconds(last.Time)
	endSec := effectiveTrackerWindowEndUnixSeconds(last.Time, now)
	if endSec <= lastSec {
		return ranks
	}

	idle := last
	idle.Time = time.Unix(endSec, 0).UTC().UnixMilli()
	return append(ranks, idle)
}

func recoveryRecordStartAt(samples []trackerScoreSample) *int64 {
	if len(samples) == 0 {
		return nil
	}
	recovery := normalizeTrackerUnixSeconds(samples[0].timestamp)
	flatStart := recovery
	inFlat := false
	for i := 1; i < len(samples); i++ {
		prev := samples[i-1]
		curr := samples[i]
		currSec := normalizeTrackerUnixSeconds(curr.timestamp)
		switch {
		case curr.score == prev.score:
			inFlat = true
		case curr.score > prev.score:
			if inFlat && currSec-flatStart >= trackerRecoveryIdleSeconds {
				recovery = currSec
			}
			flatStart = currSec
			inFlat = false
		case curr.score < prev.score:
			flatStart = currSec
			inFlat = false
		}
	}
	value := formatTrackerTimestamp(recovery)
	return &value
}

func findWindowBaselineIndex(samples []trackerScoreSample, windowStart int64) int {
	if len(samples) == 0 {
		return -1
	}
	baseline := -1
	for i := range samples {
		sec := normalizeTrackerUnixSeconds(samples[i].timestamp)
		if sec <= windowStart {
			baseline = i
			continue
		}
		break
	}
	if baseline >= 0 {
		return baseline
	}
	return 0
}

func countPositiveDeltas(samples []trackerScoreSample) int {
	if len(samples) < 2 {
		return 0
	}
	count := 0
	for i := 1; i < len(samples); i++ {
		if samples[i].score-samples[i-1].score > 0 {
			count++
		}
	}
	return count
}
