package music

import "testing"

func TestFindMusicMetaMatchesDifficulty(t *testing.T) {
	payload := []byte(`[
		{"music_id": 123, "difficulty": "expert", "skill_score_solo": [0.1], "fever_score": 0.2},
		{"music_id": 123, "difficulty": "master", "skill_score_solo": [0.3], "fever_score": 0.4}
	]`)

	item := findMusicMeta(payload, 123, "master")
	if item == nil {
		t.Fatal("expected matching music meta")
	}
	if got := item["difficulty"]; got != "master" {
		t.Fatalf("difficulty = %v", got)
	}
	if got := item["fever_score"]; got != 0.4 {
		t.Fatalf("fever_score = %v", got)
	}
}
