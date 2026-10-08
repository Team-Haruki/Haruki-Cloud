package snapshot

import (
	"bytes"
	"fmt"
	"math/rand/v2"
	"slices"
	"strconv"

	json "haruki-cloud/internal/jsonutil"
)

// syntheticSuitePayload builds a deterministic suite-shaped JSON document of
// roughly targetBytes. It contains no user data: every value comes from a
// seeded generator. Top-level keys are emitted in sorted order, the way the
// Toolbox encodes its stored documents, so upload_time sits between the short
// leading members and the large user* arrays.
func syntheticSuitePayload(targetBytes int, uploadTime int64) []byte {
	rng := rand.New(rand.NewPCG(7, uint64(targetBytes)))
	members := map[string]any{
		"now":         uploadTime * 1000,
		"upload_time": uploadTime,
		"userGamedata": map[string]any{
			"userId": 7000000000000001, "name": "synthetic", "deck": 1, "rank": 250, "coin": 123456, "virtualCoin": 789,
		},
		"userProfile": map[string]any{"profileImageType": "leader", "profileImageId": 1, "word": "synthetic profile"},
		"userDecks":   []any{map[string]any{"deckId": 1, "leader": 1001, "subLeader": 1002, "member1": 1001, "member2": 1002, "member3": 1003, "member4": 1004, "member5": 1005, "name": "deck"}},
	}
	cards := make([]any, 0, 1500)
	for i := range 1500 {
		cards = append(cards, map[string]any{
			"cardId": 1000 + i, "level": 1 + rng.IntN(60), "exp": rng.IntN(100000), "totalExp": rng.IntN(1000000),
			"skillLevel": 1 + rng.IntN(4), "skillExp": rng.IntN(1000), "totalSkillExp": rng.IntN(10000),
			"masterRank": rng.IntN(6), "specialTrainingStatus": []string{"not_doing", "done"}[rng.IntN(2)],
			"defaultImage": []string{"original", "special_training"}[rng.IntN(2)], "duplicateCount": rng.IntN(10),
			"createdAt": 1600000000000 + rng.Int64N(1e11),
			"episodes": []any{
				map[string]any{"cardEpisodeId": i*2 + 1, "scenarioStatus": "already_read", "scenarioStatusReasons": []string{}, "isNotSkipped": false},
				map[string]any{"cardEpisodeId": i*2 + 2, "scenarioStatus": "unreleased", "scenarioStatusReasons": []string{"no_open_condition"}, "isNotSkipped": true},
			},
		})
	}
	members["userCards"] = cards
	musics := make([]any, 0, 600)
	results := make([]any, 0, 3000)
	for i := range 600 {
		statuses := make([]any, 0, 5)
		for _, difficulty := range []string{"easy", "normal", "hard", "expert", "master"} {
			statuses = append(statuses, map[string]any{
				"musicId": i + 1, "musicDifficulty": difficulty, "musicDifficultyStatus": "available",
				"userMusicResults": []any{map[string]any{
					"musicId": i + 1, "musicDifficulty": difficulty, "playType": "single", "playResult": []string{"clear", "full_combo", "full_perfect"}[rng.IntN(3)],
					"highScore": rng.IntN(2000000), "fullComboFlg": rng.IntN(2) == 1, "fullPerfectFlg": rng.IntN(2) == 1,
					"mvpCount": rng.IntN(50), "superStarCount": rng.IntN(50), "createdAt": 1600000000000 + rng.Int64N(1e11), "updatedAt": 1600000000000 + rng.Int64N(1e11),
				}},
			})
			results = append(results, map[string]any{
				"musicId": i + 1, "musicDifficultyType": difficulty, "playType": "single", "playResult": "clear",
				"highScore": rng.IntN(2000000), "fullComboFlg": rng.IntN(2) == 1, "fullPerfectFlg": rng.IntN(2) == 1,
			})
		}
		musics = append(musics, map[string]any{"musicId": i + 1, "userMusicDifficultyStatuses": statuses})
	}
	members["userMusics"] = musics
	members["userMusicResults"] = results

	encode := func() []byte {
		keys := make([]string, 0, len(members))
		for key := range members {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		var out bytes.Buffer
		out.WriteByte('{')
		for i, key := range keys {
			if i > 0 {
				out.WriteByte(',')
			}
			out.WriteString(strconv.Quote(key))
			out.WriteByte(':')
			value, err := json.Marshal(members[key])
			if err != nil {
				panic(err)
			}
			out.Write(value)
		}
		out.WriteByte('}')
		return out.Bytes()
	}
	data := encode()
	// Pad with mission-status style arrays until the target size is reached.
	for filler := 0; len(data) < targetBytes; filler++ {
		rows := make([]any, 0, 4000)
		for i := range 4000 {
			rows = append(rows, map[string]any{
				"missionType": "synthetic_mission", "missionId": filler*4000 + i, "progress": rng.IntN(1000),
				"missionStatus": []string{"achieved", "received", "progress"}[rng.IntN(3)], "updatedAt": 1600000000000 + rng.Int64N(1e11),
			})
		}
		members[fmt.Sprintf("userSyntheticMissionStatuses%02d", filler)] = rows
		data = encode()
	}
	return data
}

// syntheticMySekaiPayload builds a mysekai-shaped document whose large
// updatedResources member sorts before upload_time.
func syntheticMySekaiPayload(targetBytes int, uploadTime int64) []byte {
	rng := rand.New(rand.NewPCG(11, uint64(targetBytes)))
	var out bytes.Buffer
	out.WriteString(`{"updatedResources":{"userMysekaiHarvestMaps":[`)
	for i := 0; out.Len() < targetBytes; i++ {
		if i > 0 {
			out.WriteByte(',')
		}
		fmt.Fprintf(&out, `{"mysekaiSiteId":%d,"userMysekaiSiteHarvestFixtures":[{"mysekaiSiteHarvestFixtureId":%d,"userMysekaiSiteHarvestFixtureStatus":"spawned","positionX":%d,"positionZ":%d,"hp":%d,"lastHarvestedAt":%d}],"userMysekaiSiteHarvestResourceDrops":[{"resourceType":"mysekai_material","resourceId":%d,"positionX":%d,"positionZ":%d,"hp":%d,"seq":%d,"mysekaiSiteHarvestResourceDropStatus":"before_drop","quantity":%d}]}`,
			i%8+1, rng.IntN(100000), rng.IntN(200)-100, rng.IntN(200)-100, rng.IntN(100), 1600000000000+rng.Int64N(1e11),
			rng.IntN(100), rng.IntN(200)-100, rng.IntN(200)-100, rng.IntN(100), i, 1+rng.IntN(5))
	}
	fmt.Fprintf(&out, `]},"upload_time":%d}`, uploadTime)
	return out.Bytes()
}
